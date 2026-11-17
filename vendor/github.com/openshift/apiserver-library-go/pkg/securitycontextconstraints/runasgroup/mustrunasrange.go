package runasgroup

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	api "k8s.io/kubernetes/pkg/apis/core"

	securityv1 "github.com/openshift/api/security/v1"
)

// mustRunAsRange implements the RunAsGroupSecurityContextConstraintsStrategy interface
type mustRunAsRange struct {
	opts *securityv1.RunAsGroupStrategyOptions
}

var _ RunAsGroupSecurityContextConstraintsStrategy = &mustRunAsRange{}

// NewMustRunAsRange provides a strategy that requires the container to run as a specific GID in a range.
func NewMustRunAsRange(options *securityv1.RunAsGroupStrategyOptions) (RunAsGroupSecurityContextConstraintsStrategy, error) {
	if options == nil {
		return nil, fmt.Errorf("MustRunAsRange requires run as group options")
	}
	if len(options.Ranges) == 0 {
		return nil, fmt.Errorf("MustRunAsRange requires at least one range")
	}
	// Validate all ranges are valid (min <= max for each range)
	for i, rng := range options.Ranges {
		if err := validateIDRange(rng); err != nil {
			return nil, fmt.Errorf("MustRunAsRange has invalid range at index %d: %v", i, err)
		}
	}
	return &mustRunAsRange{
		opts: options,
	}, nil
}

// Generate creates the gid based on policy rules.  MustRunAsRange returns the minimum GID of the first range.
func (s *mustRunAsRange) Generate(pod *api.Pod, container *api.Container) (*int64, error) {
	return s.opts.Ranges[0].Min, nil
}

// Validate ensures that the specified values fall within the range of the strategy.
func (s *mustRunAsRange) Validate(fldPath *field.Path, _ *api.Pod, _ *api.Container, runAsGroup *int64) field.ErrorList {
	allErrs := field.ErrorList{}

	if runAsGroup == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("runAsGroup"), ""))
		return allErrs
	}

	// Check if the GID falls within any of the allowed ranges
	for _, rng := range s.opts.Ranges {
		if *runAsGroup >= *rng.Min && *runAsGroup <= *rng.Max {
			return allErrs
		}
	}

	// Build error message with all allowed ranges
	rangesStr := ""
	for i, rng := range s.opts.Ranges {
		if i > 0 {
			rangesStr += ", "
		}
		rangesStr += fmt.Sprintf("[%d, %d]", *rng.Min, *rng.Max)
	}
	detail := fmt.Sprintf("must be in the ranges: %s", rangesStr)
	allErrs = append(allErrs, field.Invalid(fldPath.Child("runAsGroup"), *runAsGroup, detail))

	return allErrs
}
