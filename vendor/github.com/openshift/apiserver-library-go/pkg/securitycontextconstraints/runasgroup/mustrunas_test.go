package runasgroup

import (
	"fmt"
	"strings"
	"testing"

	securityv1 "github.com/openshift/api/security/v1"
)

func TestMustRunAsOptions(t *testing.T) {
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
		"range with different min and max": {
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1), Max: int64Ptr(10)},
				},
			},
			pass: false,
		},
		"valid single GID (min == max)": {
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(5), Max: int64Ptr(5)},
				},
			},
			pass: true,
		},
		"valid single GID zero": {
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(0), Max: int64Ptr(0)},
				},
			},
			pass: true,
		},
	}
	for name, tc := range tests {
		_, err := NewMustRunAs(tc.opts)
		if err != nil && tc.pass {
			t.Errorf("%s expected to pass but received error %v", name, err)
		}
		if err == nil && !tc.pass {
			t.Errorf("%s expected to fail but did not receive an error", name)
		}
	}
}

func TestMustRunAsGenerate(t *testing.T) {
	var gid int64 = 100
	opts := &securityv1.RunAsGroupStrategyOptions{
		Ranges: []securityv1.RunAsGroupIDRange{
			{Min: int64Ptr(gid), Max: int64Ptr(gid)},
		},
	}
	mustRunAs, err := NewMustRunAs(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewMustRunAs %v", err)
	}
	generated, err := mustRunAs.Generate(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error generating gid %v", err)
	}
	if generated == nil {
		t.Fatal("expected generated gid but got nil")
	}
	if *generated != gid {
		t.Errorf("generated gid %d does not equal configured gid %d", *generated, gid)
	}
}

func TestMustRunAsValidate(t *testing.T) {
	var gid int64 = 100
	var badGID int64 = 200
	opts := &securityv1.RunAsGroupStrategyOptions{
		Ranges: []securityv1.RunAsGroupIDRange{
			{Min: int64Ptr(gid), Max: int64Ptr(gid)},
		},
	}
	mustRunAs, err := NewMustRunAs(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewMustRunAs %v", err)
	}

	// Test nil runAsGroup
	errs := mustRunAs.Validate(nil, nil, nil, nil)
	expectedMessage := "runAsGroup: Required value"
	if len(errs) == 0 {
		t.Errorf("expected errors from nil runAsGroup but got none")
	} else if !strings.Contains(errs[0].Error(), expectedMessage) {
		t.Errorf("expected error to contain %q but it did not: %v", expectedMessage, errs)
	}

	// Test mismatched GID
	errs = mustRunAs.Validate(nil, nil, nil, &badGID)
	expectedMessage = fmt.Sprintf("runAsGroup: Invalid value: %d: must be in the ranges: [%d, %d]", badGID, gid, gid)
	if len(errs) == 0 {
		t.Errorf("expected errors from mismatch gid but got none")
	} else if !strings.Contains(errs[0].Error(), expectedMessage) {
		t.Errorf("expected error to contain %q but it did not: %v", expectedMessage, errs)
	}

	// Test matching GID
	errs = mustRunAs.Validate(nil, nil, nil, &gid)
	if len(errs) != 0 {
		t.Errorf("expected no errors from matching gid but got %v", errs)
	}
}

func TestMustRunAsValidateZeroGID(t *testing.T) {
	var gid int64 = 0
	opts := &securityv1.RunAsGroupStrategyOptions{
		Ranges: []securityv1.RunAsGroupIDRange{
			{Min: int64Ptr(gid), Max: int64Ptr(gid)},
		},
	}
	mustRunAs, err := NewMustRunAs(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewMustRunAs with zero GID %v", err)
	}

	// Test that zero GID is accepted when required
	errs := mustRunAs.Validate(nil, nil, nil, &gid)
	if len(errs) != 0 {
		t.Errorf("expected no errors from matching zero gid but got %v", errs)
	}

	// Test that non-zero GID is rejected
	var nonZeroGID int64 = 1
	errs = mustRunAs.Validate(nil, nil, nil, &nonZeroGID)
	if len(errs) == 0 {
		t.Errorf("expected errors from non-zero gid when zero is required but got none")
	}
}
