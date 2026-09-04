package runasgroup

import (
	"fmt"
	"strings"
	"testing"

	securityv1 "github.com/openshift/api/security/v1"
)

func TestMustRunAsRangeOptions(t *testing.T) {
	tests := map[string]struct {
		opts *securityv1.RunAsGroupStrategyOptions
		pass bool
	}{
		"nil opts": {
			opts: nil,
			pass: false,
		},
		"empty opts": {
			opts: &securityv1.RunAsGroupStrategyOptions{},
			pass: false,
		},
		"no ranges": {
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{},
			},
			pass: false,
		},
		"valid single range": {
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1), Max: int64Ptr(10)},
				},
			},
			pass: true,
		},
		"valid single range with min == max": {
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(5), Max: int64Ptr(5)},
				},
			},
			pass: true,
		},
		"valid multiple ranges": {
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1), Max: int64Ptr(10)},
					{Min: int64Ptr(100), Max: int64Ptr(200)},
				},
			},
			pass: true,
		},
		"valid range starting at zero": {
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(0), Max: int64Ptr(100)},
				},
			},
			pass: true,
		},
	}
	for name, tc := range tests {
		_, err := NewMustRunAsRange(tc.opts)
		if err != nil && tc.pass {
			t.Errorf("%s expected to pass but received error %v", name, err)
		}
		if err == nil && !tc.pass {
			t.Errorf("%s expected to fail but did not receive an error", name)
		}
	}
}

func TestMustRunAsRangeGenerate(t *testing.T) {
	var gidMin int64 = 10
	var gidMax int64 = 20
	opts := &securityv1.RunAsGroupStrategyOptions{
		Ranges: []securityv1.RunAsGroupIDRange{
			{Min: int64Ptr(gidMin), Max: int64Ptr(gidMax)},
		},
	}
	mustRunAsRange, err := NewMustRunAsRange(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewMustRunAsRange %v", err)
	}
	generated, err := mustRunAsRange.Generate(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error generating gid %v", err)
	}
	if generated == nil {
		t.Fatal("expected generated gid but got nil")
	}
	if *generated != gidMin {
		t.Errorf("generated gid %d does not equal expected min gid %d", *generated, gidMin)
	}
}

func TestMustRunAsRangeGenerateMultipleRanges(t *testing.T) {
	opts := &securityv1.RunAsGroupStrategyOptions{
		Ranges: []securityv1.RunAsGroupIDRange{
			{Min: int64Ptr(10), Max: int64Ptr(20)},
			{Min: int64Ptr(100), Max: int64Ptr(200)},
		},
	}
	mustRunAsRange, err := NewMustRunAsRange(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewMustRunAsRange %v", err)
	}
	generated, err := mustRunAsRange.Generate(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error generating gid %v", err)
	}
	if generated == nil {
		t.Fatal("expected generated gid but got nil")
	}
	// Should return the min of the first range
	if *generated != 10 {
		t.Errorf("generated gid %d does not equal expected min of first range 10", *generated)
	}
}

func TestMustRunAsRangeValidate(t *testing.T) {
	var gidMin int64 = 10
	var gidMax int64 = 20
	opts := &securityv1.RunAsGroupStrategyOptions{
		Ranges: []securityv1.RunAsGroupIDRange{
			{Min: int64Ptr(gidMin), Max: int64Ptr(gidMax)},
		},
	}
	mustRunAsRange, err := NewMustRunAsRange(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewMustRunAsRange %v", err)
	}

	// Test nil runAsGroup
	errs := mustRunAsRange.Validate(nil, nil, nil, nil)
	expectedMessage := "runAsGroup: Required value"
	if len(errs) == 0 {
		t.Errorf("expected errors from nil runAsGroup but got none")
	} else if !strings.Contains(errs[0].Error(), expectedMessage) {
		t.Errorf("expected error to contain %q but it did not: %v", expectedMessage, errs)
	}

	// Test GID below range
	var lowGID int64 = 5
	errs = mustRunAsRange.Validate(nil, nil, nil, &lowGID)
	expectedMessage = fmt.Sprintf("runAsGroup: Invalid value: %d: must be in the ranges: [%d, %d]", lowGID, gidMin, gidMax)
	if len(errs) == 0 {
		t.Errorf("expected errors from low gid but got none")
	} else if !strings.Contains(errs[0].Error(), expectedMessage) {
		t.Errorf("expected error to contain %q but it did not: %v", expectedMessage, errs)
	}

	// Test GID above range
	var highGID int64 = 25
	errs = mustRunAsRange.Validate(nil, nil, nil, &highGID)
	expectedMessage = fmt.Sprintf("runAsGroup: Invalid value: %d: must be in the ranges: [%d, %d]", highGID, gidMin, gidMax)
	if len(errs) == 0 {
		t.Errorf("expected errors from high gid but got none")
	} else if !strings.Contains(errs[0].Error(), expectedMessage) {
		t.Errorf("expected error to contain %q but it did not: %v", expectedMessage, errs)
	}

	// Test GID at minimum boundary
	errs = mustRunAsRange.Validate(nil, nil, nil, &gidMin)
	if len(errs) != 0 {
		t.Errorf("expected no errors from min boundary gid but got %v", errs)
	}

	// Test GID at maximum boundary
	errs = mustRunAsRange.Validate(nil, nil, nil, &gidMax)
	if len(errs) != 0 {
		t.Errorf("expected no errors from max boundary gid but got %v", errs)
	}

	// Test GID in middle of range
	var midGID int64 = 15
	errs = mustRunAsRange.Validate(nil, nil, nil, &midGID)
	if len(errs) != 0 {
		t.Errorf("expected no errors from mid-range gid but got %v", errs)
	}
}

func TestMustRunAsRangeValidateMultipleRanges(t *testing.T) {
	opts := &securityv1.RunAsGroupStrategyOptions{
		Ranges: []securityv1.RunAsGroupIDRange{
			{Min: int64Ptr(10), Max: int64Ptr(20)},
			{Min: int64Ptr(100), Max: int64Ptr(200)},
			{Min: int64Ptr(1000), Max: int64Ptr(2000)},
		},
	}
	mustRunAsRange, err := NewMustRunAsRange(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewMustRunAsRange %v", err)
	}

	tests := []struct {
		name        string
		gid         int64
		shouldPass  bool
		description string
	}{
		{"first range min", 10, true, "minimum of first range"},
		{"first range mid", 15, true, "middle of first range"},
		{"first range max", 20, true, "maximum of first range"},
		{"second range min", 100, true, "minimum of second range"},
		{"second range mid", 150, true, "middle of second range"},
		{"second range max", 200, true, "maximum of second range"},
		{"third range min", 1000, true, "minimum of third range"},
		{"third range mid", 1500, true, "middle of third range"},
		{"third range max", 2000, true, "maximum of third range"},
		{"before first range", 5, false, "before first range"},
		{"between first and second", 50, false, "between first and second range"},
		{"between second and third", 500, false, "between second and third range"},
		{"after third range", 3000, false, "after third range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := mustRunAsRange.Validate(nil, nil, nil, &tt.gid)
			if tt.shouldPass && len(errs) != 0 {
				t.Errorf("expected no errors for %s (gid %d) but got %v", tt.description, tt.gid, errs)
			}
			if !tt.shouldPass && len(errs) == 0 {
				t.Errorf("expected errors for %s (gid %d) but got none", tt.description, tt.gid)
			}
		})
	}
}

func TestMustRunAsRangeValidateErrorMessage(t *testing.T) {
	opts := &securityv1.RunAsGroupStrategyOptions{
		Ranges: []securityv1.RunAsGroupIDRange{
			{Min: int64Ptr(10), Max: int64Ptr(20)},
			{Min: int64Ptr(100), Max: int64Ptr(200)},
		},
	}
	mustRunAsRange, err := NewMustRunAsRange(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewMustRunAsRange %v", err)
	}

	var invalidGID int64 = 50
	errs := mustRunAsRange.Validate(nil, nil, nil, &invalidGID)
	if len(errs) == 0 {
		t.Fatal("expected error but got none")
	}

	errorMsg := errs[0].Error()
	// Error message should contain both ranges
	if !strings.Contains(errorMsg, "[10, 20]") {
		t.Errorf("expected error message to contain first range '[10, 20]' but got: %s", errorMsg)
	}
	if !strings.Contains(errorMsg, "[100, 200]") {
		t.Errorf("expected error message to contain second range '[100, 200]' but got: %s", errorMsg)
	}
	if !strings.Contains(errorMsg, "must be in the ranges:") {
		t.Errorf("expected error message to contain 'must be in the ranges:' but got: %s", errorMsg)
	}
}

func TestMustRunAsRangeSingleGIDRange(t *testing.T) {
	// Test that a range with min == max works correctly
	var gid int64 = 100
	opts := &securityv1.RunAsGroupStrategyOptions{
		Ranges: []securityv1.RunAsGroupIDRange{
			{Min: int64Ptr(gid), Max: int64Ptr(gid)},
		},
	}
	mustRunAsRange, err := NewMustRunAsRange(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewMustRunAsRange with single GID %v", err)
	}

	// Should accept the exact GID
	errs := mustRunAsRange.Validate(nil, nil, nil, &gid)
	if len(errs) != 0 {
		t.Errorf("expected no errors for exact gid %d but got %v", gid, errs)
	}

	// Should reject any other GID
	var otherGID int64 = 99
	errs = mustRunAsRange.Validate(nil, nil, nil, &otherGID)
	if len(errs) == 0 {
		t.Errorf("expected errors for gid %d when only %d is allowed but got none", otherGID, gid)
	}

	otherGID = 101
	errs = mustRunAsRange.Validate(nil, nil, nil, &otherGID)
	if len(errs) == 0 {
		t.Errorf("expected errors for gid %d when only %d is allowed but got none", otherGID, gid)
	}
}
