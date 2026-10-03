package runasgroup

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	api "k8s.io/kubernetes/pkg/apis/core"

	securityv1 "github.com/openshift/api/security/v1"
)

// mustRunAs implements the RunAsGroupSecurityContextConstraintsStrategy interface
type mustRunAs struct {
	opts *securityv1.RunAsGroupStrategyOptions
}

var _ RunAsGroupSecurityContextConstraintsStrategy = &mustRunAs{}

// NewMustRunAs provides a strategy that requires the container to run as a specific GID.
func NewMustRunAs(options *securityv1.RunAsGroupStrategyOptions) (RunAsGroupSecurityContextConstraintsStrategy, error) {
	if options == nil {
		return nil, fmt.Errorf("MustRunAs requires run as group options")
	}
	if len(options.Ranges) == 0 {
		return nil, fmt.Errorf("MustRunAs requires at least one range")
	}
	// Validate the range is valid (min <= max)
	if err := validateIDRange(options.Ranges[0]); err != nil {
		return nil, fmt.Errorf("MustRunAs has invalid range: %v", err)
	}
	if *options.Ranges[0].Min != *options.Ranges[0].Max {
		return nil, fmt.Errorf("MustRunAs requires the first range to have the same min and max GID")
	}
	return &mustRunAs{
		opts: options,
	}, nil
}

// Generate creates the gid based on policy rules.  MustRunAs returns the GID it is initialized with.
func (s *mustRunAs) Generate(pod *api.Pod, container *api.Container) (*int64, error) {
	return s.opts.Ranges[0].Min, nil
}

// Validate ensures that the specified values fall within the range of the strategy.
func (s *mustRunAs) Validate(fldPath *field.Path, _ *api.Pod, _ *api.Container, runAsGroup *int64) field.ErrorList {
	allErrs := field.ErrorList{}

	if runAsGroup == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("runAsGroup"), ""))
		return allErrs
	}

	requiredGID := *s.opts.Ranges[0].Min
	if requiredGID != *runAsGroup {
		detail := fmt.Sprintf("must be in the ranges: [%d, %d]", requiredGID, requiredGID)
		allErrs = append(allErrs, field.Invalid(fldPath.Child("runAsGroup"), *runAsGroup, detail))
		return allErrs
	}

	return allErrs
}
