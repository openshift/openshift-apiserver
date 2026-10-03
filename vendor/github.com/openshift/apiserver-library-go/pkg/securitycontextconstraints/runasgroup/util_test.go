package runasgroup

import (
	"strings"
	"testing"

	securityv1 "github.com/openshift/api/security/v1"
)

func TestValidateIDRange(t *testing.T) {
	tests := []struct {
		name        string
		rng         securityv1.RunAsGroupIDRange
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid range with min < max",
			rng:         securityv1.RunAsGroupIDRange{Min: int64Ptr(10), Max: int64Ptr(20)},
			expectError: false,
		},
		{
			name:        "valid range with min == max",
			rng:         securityv1.RunAsGroupIDRange{Min: int64Ptr(100), Max: int64Ptr(100)},
			expectError: false,
		},
		{
			name:        "invalid range with min > max",
			rng:         securityv1.RunAsGroupIDRange{Min: int64Ptr(200), Max: int64Ptr(100)},
			expectError: true,
			errorMsg:    "min (200) must be less than or equal to max (100)",
		},
		{
			name:        "valid range with zero values",
			rng:         securityv1.RunAsGroupIDRange{Min: int64Ptr(0), Max: int64Ptr(0)},
			expectError: false,
		},
		{
			name:        "valid range starting at zero",
			rng:         securityv1.RunAsGroupIDRange{Min: int64Ptr(0), Max: int64Ptr(1000)},
			expectError: false,
		},
		{
			name:        "valid range with negative values (allowed)",
			rng:         securityv1.RunAsGroupIDRange{Min: int64Ptr(-10), Max: int64Ptr(10)},
			expectError: false,
		},
		{
			name:        "invalid range with negative max less than negative min",
			rng:         securityv1.RunAsGroupIDRange{Min: int64Ptr(-5), Max: int64Ptr(-10)},
			expectError: true,
			errorMsg:    "min (-5) must be less than or equal to max (-10)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIDRange(tt.rng)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain %q but got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestMustRunAsWithInvalidRange(t *testing.T) {
	tests := []struct {
		name        string
		opts        *securityv1.RunAsGroupStrategyOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "invalid range with min > max",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(200), Max: int64Ptr(100)},
				},
			},
			expectError: true,
			errorMsg:    "invalid range",
		},
		{
			name: "valid range with min == max",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(100), Max: int64Ptr(100)},
				},
			},
			expectError: false,
		},
		{
			name: "invalid range with negative max less than min",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(10), Max: int64Ptr(-10)},
				},
			},
			expectError: true,
			errorMsg:    "invalid range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMustRunAs(tt.opts)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain %q but got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestMustRunAsRangeWithInvalidRanges(t *testing.T) {
	tests := []struct {
		name        string
		opts        *securityv1.RunAsGroupStrategyOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "single invalid range with min > max",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(200), Max: int64Ptr(100)},
				},
			},
			expectError: true,
			errorMsg:    "invalid range at index 0",
		},
		{
			name: "multiple ranges with one invalid",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(10), Max: int64Ptr(20)},
					{Min: int64Ptr(100), Max: int64Ptr(50)}, // Invalid
				},
			},
			expectError: true,
			errorMsg:    "invalid range at index 1",
		},
		{
			name: "multiple valid ranges",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(10), Max: int64Ptr(20)},
					{Min: int64Ptr(100), Max: int64Ptr(200)},
				},
			},
			expectError: false,
		},
		{
			name: "all ranges invalid",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(200), Max: int64Ptr(100)},
					{Min: int64Ptr(1000), Max: int64Ptr(500)},
				},
			},
			expectError: true,
			errorMsg:    "invalid range at index 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMustRunAsRange(tt.opts)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain %q but got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}
