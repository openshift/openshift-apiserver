package runasgroup

import (
	"testing"

	securityv1 "github.com/openshift/api/security/v1"
)

// TestAllStrategyImplementations verifies that all three runAsGroup strategies
// properly implement the RunAsGroupSecurityContextConstraintsStrategy interface
// and work correctly with the new RunAsGroupIDRange pointer types
func TestAllStrategyImplementations(t *testing.T) {
	tests := []struct {
		name         string
		strategyType string
		opts         *securityv1.RunAsGroupStrategyOptions
		testGID      *int64
		shouldPass   bool
	}{
		{
			name:         "MustRunAs with single GID - valid",
			strategyType: "MustRunAs",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAs,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(1000)},
				},
			},
			testGID:    int64Ptr(1000),
			shouldPass: true,
		},
		{
			name:         "MustRunAs with single GID - invalid",
			strategyType: "MustRunAs",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAs,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(1000)},
				},
			},
			testGID:    int64Ptr(2000),
			shouldPass: false,
		},
		{
			name:         "MustRunAsRange with single range - valid min",
			strategyType: "MustRunAsRange",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAsRange,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(2000)},
				},
			},
			testGID:    int64Ptr(1000),
			shouldPass: true,
		},
		{
			name:         "MustRunAsRange with single range - valid mid",
			strategyType: "MustRunAsRange",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAsRange,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(2000)},
				},
			},
			testGID:    int64Ptr(1500),
			shouldPass: true,
		},
		{
			name:         "MustRunAsRange with single range - valid max",
			strategyType: "MustRunAsRange",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAsRange,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(2000)},
				},
			},
			testGID:    int64Ptr(2000),
			shouldPass: true,
		},
		{
			name:         "MustRunAsRange with single range - invalid below",
			strategyType: "MustRunAsRange",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAsRange,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(2000)},
				},
			},
			testGID:    int64Ptr(999),
			shouldPass: false,
		},
		{
			name:         "MustRunAsRange with single range - invalid above",
			strategyType: "MustRunAsRange",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAsRange,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(2000)},
				},
			},
			testGID:    int64Ptr(2001),
			shouldPass: false,
		},
		{
			name:         "MustRunAsRange with multiple ranges - valid in first",
			strategyType: "MustRunAsRange",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAsRange,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(2000)},
					{Min: int64Ptr(5000), Max: int64Ptr(6000)},
				},
			},
			testGID:    int64Ptr(1500),
			shouldPass: true,
		},
		{
			name:         "MustRunAsRange with multiple ranges - valid in second",
			strategyType: "MustRunAsRange",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAsRange,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(2000)},
					{Min: int64Ptr(5000), Max: int64Ptr(6000)},
				},
			},
			testGID:    int64Ptr(5500),
			shouldPass: true,
		},
		{
			name:         "MustRunAsRange with multiple ranges - invalid between ranges",
			strategyType: "MustRunAsRange",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyMustRunAsRange,
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1000), Max: int64Ptr(2000)},
					{Min: int64Ptr(5000), Max: int64Ptr(6000)},
				},
			},
			testGID:    int64Ptr(3000),
			shouldPass: false,
		},
		{
			name:         "RunAsAny - accepts any GID",
			strategyType: "RunAsAny",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyRunAsAny,
			},
			testGID:    int64Ptr(9999),
			shouldPass: true,
		},
		{
			name:         "RunAsAny - accepts zero",
			strategyType: "RunAsAny",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyRunAsAny,
			},
			testGID:    int64Ptr(0),
			shouldPass: true,
		},
		{
			name:         "RunAsAny - accepts nil",
			strategyType: "RunAsAny",
			opts: &securityv1.RunAsGroupStrategyOptions{
				Type: securityv1.RunAsGroupStrategyRunAsAny,
			},
			testGID:    nil,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var strategy RunAsGroupSecurityContextConstraintsStrategy
			var err error

			// Create the appropriate strategy
			switch tt.strategyType {
			case "MustRunAs":
				strategy, err = NewMustRunAs(tt.opts)
			case "MustRunAsRange":
				strategy, err = NewMustRunAsRange(tt.opts)
			case "RunAsAny":
				strategy, err = NewRunAsAny(tt.opts)
			default:
				t.Fatalf("Unknown strategy type: %s", tt.strategyType)
			}

			if err != nil {
				t.Fatalf("Failed to create strategy: %v", err)
			}

			// Test Generate function
			generated, err := strategy.Generate(nil, nil)
			if err != nil {
				t.Errorf("Generate failed: %v", err)
			}
			// For RunAsAny, generated should be nil
			if tt.strategyType == "RunAsAny" && generated != nil {
				t.Errorf("RunAsAny should generate nil, got %v", *generated)
			}
			// For MustRunAs and MustRunAsRange, generated should be non-nil
			if tt.strategyType != "RunAsAny" && generated == nil {
				t.Errorf("%s should generate a non-nil GID", tt.strategyType)
			}

			// Test Validate function
			errs := strategy.Validate(nil, nil, nil, tt.testGID)
			hasErrors := len(errs) > 0

			if tt.shouldPass && hasErrors {
				t.Errorf("Validation should pass but got errors: %v", errs)
			}
			if !tt.shouldPass && !hasErrors {
				t.Errorf("Validation should fail but got no errors")
			}
		})
	}
}

// TestPointerHandling verifies that all strategies properly handle pointer types
// from RunAsGroupIDRange
func TestPointerHandling(t *testing.T) {
	t.Run("MustRunAs handles pointer Min/Max correctly", func(t *testing.T) {
		opts := &securityv1.RunAsGroupStrategyOptions{
			Type: securityv1.RunAsGroupStrategyMustRunAs,
			Ranges: []securityv1.RunAsGroupIDRange{
				{Min: int64Ptr(100), Max: int64Ptr(100)},
			},
		}
		strategy, err := NewMustRunAs(opts)
		if err != nil {
			t.Fatalf("NewMustRunAs failed: %v", err)
		}

		generated, err := strategy.Generate(nil, nil)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if generated == nil {
			t.Fatal("Generated GID should not be nil")
		}
		if *generated != 100 {
			t.Errorf("Generated GID = %d, want 100", *generated)
		}
	})

	t.Run("MustRunAsRange handles pointer Min/Max correctly", func(t *testing.T) {
		opts := &securityv1.RunAsGroupStrategyOptions{
			Type: securityv1.RunAsGroupStrategyMustRunAsRange,
			Ranges: []securityv1.RunAsGroupIDRange{
				{Min: int64Ptr(200), Max: int64Ptr(300)},
			},
		}
		strategy, err := NewMustRunAsRange(opts)
		if err != nil {
			t.Fatalf("NewMustRunAsRange failed: %v", err)
		}

		generated, err := strategy.Generate(nil, nil)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if generated == nil {
			t.Fatal("Generated GID should not be nil")
		}
		if *generated != 200 {
			t.Errorf("Generated GID = %d, want 200 (min of range)", *generated)
		}

		// Test validation with GID in range
		gid := int64Ptr(250)
		errs := strategy.Validate(nil, nil, nil, gid)
		if len(errs) > 0 {
			t.Errorf("Validation failed for GID in range: %v", errs)
		}
	})

	t.Run("validateIDRange handles nil pointers", func(t *testing.T) {
		// Test with nil Min
		rng := securityv1.RunAsGroupIDRange{Min: nil, Max: int64Ptr(100)}
		err := validateIDRange(rng)
		if err == nil {
			t.Error("validateIDRange should fail with nil Min")
		}

		// Test with nil Max
		rng = securityv1.RunAsGroupIDRange{Min: int64Ptr(100), Max: nil}
		err = validateIDRange(rng)
		if err == nil {
			t.Error("validateIDRange should fail with nil Max")
		}

		// Test with both nil
		rng = securityv1.RunAsGroupIDRange{Min: nil, Max: nil}
		err = validateIDRange(rng)
		if err == nil {
			t.Error("validateIDRange should fail with both nil")
		}
	})
}

// TestEdgeCases verifies edge cases and boundary conditions
func TestEdgeCases(t *testing.T) {
	t.Run("Zero GID handling", func(t *testing.T) {
		opts := &securityv1.RunAsGroupStrategyOptions{
			Type: securityv1.RunAsGroupStrategyMustRunAs,
			Ranges: []securityv1.RunAsGroupIDRange{
				{Min: int64Ptr(0), Max: int64Ptr(0)},
			},
		}
		strategy, err := NewMustRunAs(opts)
		if err != nil {
			t.Fatalf("NewMustRunAs failed with zero GID: %v", err)
		}

		generated, err := strategy.Generate(nil, nil)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if *generated != 0 {
			t.Errorf("Generated GID = %d, want 0", *generated)
		}

		gid := int64Ptr(0)
		errs := strategy.Validate(nil, nil, nil, gid)
		if len(errs) > 0 {
			t.Errorf("Validation failed for zero GID: %v", errs)
		}
	})

	t.Run("Large GID values", func(t *testing.T) {
		var largeGID int64 = 2147483647 // Max int32
		opts := &securityv1.RunAsGroupStrategyOptions{
			Type: securityv1.RunAsGroupStrategyMustRunAsRange,
			Ranges: []securityv1.RunAsGroupIDRange{
				{Min: int64Ptr(largeGID), Max: int64Ptr(largeGID)},
			},
		}
		strategy, err := NewMustRunAsRange(opts)
		if err != nil {
			t.Fatalf("NewMustRunAsRange failed with large GID: %v", err)
		}

		gid := int64Ptr(largeGID)
		errs := strategy.Validate(nil, nil, nil, gid)
		if len(errs) > 0 {
			t.Errorf("Validation failed for large GID: %v", errs)
		}
	})

	t.Run("Negative GID values (allowed)", func(t *testing.T) {
		var negGID int64 = -1
		opts := &securityv1.RunAsGroupStrategyOptions{
			Type: securityv1.RunAsGroupStrategyMustRunAsRange,
			Ranges: []securityv1.RunAsGroupIDRange{
				{Min: int64Ptr(-10), Max: int64Ptr(10)},
			},
		}
		strategy, err := NewMustRunAsRange(opts)
		if err != nil {
			t.Fatalf("NewMustRunAsRange failed with negative GIDs: %v", err)
		}

		gid := int64Ptr(negGID)
		errs := strategy.Validate(nil, nil, nil, gid)
		if len(errs) > 0 {
			t.Errorf("Validation failed for negative GID in valid range: %v", errs)
		}
	})
}
