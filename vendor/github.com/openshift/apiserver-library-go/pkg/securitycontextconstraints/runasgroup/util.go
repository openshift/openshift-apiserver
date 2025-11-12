package runasgroup

import (
	"fmt"

	securityv1 "github.com/openshift/api/security/v1"
)

// validateIDRange validates that a RunAsGroupIDRange has valid min/max values.
// It ensures that Min and Max are set and Min <= Max to prevent invalid range configurations.
func validateIDRange(rng securityv1.RunAsGroupIDRange) error {
	if rng.Min == nil {
		return fmt.Errorf("min must be specified")
	}
	if rng.Max == nil {
		return fmt.Errorf("max must be specified")
	}
	if *rng.Min > *rng.Max {
		return fmt.Errorf("min (%d) must be less than or equal to max (%d)", *rng.Min, *rng.Max)
	}
	// Note: Negative GID values are allowed as they may be used in some systems
	// If validation for non-negative values is needed, it should be added here
	return nil
}
