package v1

import (
	v1 "github.com/openshift/api/security/v1"
	"github.com/openshift/apiserver-library-go/pkg/securitycontextconstraints/sccdefaults"
	"k8s.io/apimachinery/pkg/runtime"
)

func AddDefaultingFuncs(scheme *runtime.Scheme) error {
	RegisterDefaults(scheme)
	scheme.AddTypeDefaultingFunc(&v1.SecurityContextConstraints{}, func(obj interface{}) {
		scc := obj.(*v1.SecurityContextConstraints)
		sccdefaults.SetDefaults_SCC(scc)

		// Default RunAsGroup to MustRunAs with ranges if not set
		if len(scc.RunAsGroup.Type) == 0 {
			min := int64(1000)
			max := int64(65534)
			scc.RunAsGroup.Type = v1.RunAsGroupStrategyMustRunAs
			scc.RunAsGroup.Ranges = []v1.RunAsGroupIDRange{
				{
					Min: &min,
					Max: &max,
				},
			}
		}
	})

	return nil
}
