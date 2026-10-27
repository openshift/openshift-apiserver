package runasgroup

import (
	"k8s.io/apimachinery/pkg/util/validation/field"
	api "k8s.io/kubernetes/pkg/apis/core"
)

// RunAsGroupSecurityContextConstraintsStrategy defines the interface for all gid constraint strategies.
type RunAsGroupSecurityContextConstraintsStrategy interface {
	// Generate creates the gid based on policy rules.
	Generate(pod *api.Pod, container *api.Container) (*int64, error)
	// Validate ensures that the specified values fall within the range of the strategy.
	Validate(fldPath *field.Path, pod *api.Pod, container *api.Container, runAsGroup *int64) field.ErrorList
}
