package openshiftadmission

import (
	"fmt"
	"slices"

	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/resourcequota"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/kubernetes/plugin/pkg/admission/gc"

	"github.com/openshift/apiserver-library-go/pkg/admission/imagepolicy"
	quotaclusterresourcequota "github.com/openshift/apiserver-library-go/pkg/admission/quota/clusterresourcequota"
	buildsecretinjector "github.com/openshift/openshift-apiserver/pkg/build/apiserver/admission/secretinjector"
	buildstrategyrestrictions "github.com/openshift/openshift-apiserver/pkg/build/apiserver/admission/strategyrestrictions"
	imageadmission "github.com/openshift/openshift-apiserver/pkg/image/apiserver/admission/limitrange"
	projectrequestlimit "github.com/openshift/openshift-apiserver/pkg/project/apiserver/admission/requestlimit"
	requiredrouteannotations "github.com/openshift/openshift-apiserver/pkg/route/apiserver/admission/requiredrouteannotations"
	"k8s.io/apiserver/pkg/server/options"
)

// TODO register this per apiserver or at least per process
var OriginAdmissionPlugins = admission.NewPlugins()

func init() {
	RegisterAllAdmissionPlugins(OriginAdmissionPlugins)
}

// RegisterAllAdmissionPlugins registers all admission plugins
func RegisterAllAdmissionPlugins(plugins *admission.Plugins) {
	// Kube (k8s.io/kubernetes) admission plugins that we rely on. These should move to generic (k8s.io/apiserver).
	gc.Register(plugins)
	resourcequota.Register(plugins)

	genericapiserver.RegisterAllAdmissionPlugins(plugins)
	RegisterOpenshiftAdmissionPlugins(plugins)
}

func RegisterOpenshiftAdmissionPlugins(plugins *admission.Plugins) {
	projectrequestlimit.Register(plugins)
	buildsecretinjector.Register(plugins)
	buildstrategyrestrictions.Register(plugins)
	imageadmission.Register(plugins)
	imagepolicy.Register(plugins)
	quotaclusterresourcequota.Register(plugins)
	requiredrouteannotations.Register(plugins)
}

var (
	// OpenShiftAdmissionPlugins gives the in-order default admission chain for openshift resources.
	OpenShiftAdmissionPlugins = func() []string {
		// The effective admission chain.
		downstreamPlugins := []string{
			// these are from the kube chain
			"NamespaceLifecycle",
			"OwnerReferencesPermissionEnforcement",

			// all custom admission goes here to simulate being part of a webhook
			"project.openshift.io/ProjectRequestLimit",
			"build.openshift.io/BuildConfigSecretInjector",
			"build.openshift.io/BuildByStrategy",
			"image.openshift.io/ImageLimitRange",
			"image.openshift.io/ImagePolicy",
			"quota.openshift.io/ClusterResourceQuota",
			"route.openshift.io/RequiredRouteAnnotations",

			// the rest of the kube chain goes here
			"MutatingAdmissionPolicy",
			"MutatingAdmissionWebhook",
			"ValidatingAdmissionPolicy",
			"ValidatingAdmissionWebhook",
			"ResourceQuota",
		}

		// Upstream plugins that are omitted intentionally. If a given plugin is enabled by
		// default for generic API servers, it must either be enabled by default downstream
		// or included in this list with an explanation.
		omittedPlugins := []string{}

		// Plugins that are enabled downstream but are not included in the upstream
		// defaults. If an admission plugin is _removed_ from the upstream defaults, this
		// should generate an error during the corresponding k8s.io/apiserver dependency
		// bump, and the removed plugin must either be removed from `downstreamPlugins` (to
		// remain in sync with upstream) or added to `downstreamOnlyPlugins` (to
		// intentionally diverge from the upstream).
		downstreamOnlyPlugins := []string{
			"project.openshift.io/ProjectRequestLimit",
			"build.openshift.io/BuildConfigSecretInjector",
			"build.openshift.io/BuildByStrategy",
			"image.openshift.io/ImageLimitRange",
			"image.openshift.io/ImagePolicy",
			"quota.openshift.io/ClusterResourceQuota",
			"route.openshift.io/RequiredRouteAnnotations",

			"OwnerReferencesPermissionEnforcement",
			"ResourceQuota",
		}

		upstreamPlugins := options.NewAdmissionOptions().RecommendedPluginOrder
		for _, upstreamPluginName := range upstreamPlugins {
			if !slices.Contains(downstreamPlugins, upstreamPluginName) && !slices.Contains(omittedPlugins, upstreamPluginName) {
				// If you are reading this because you are changing the version of
				// the k8s.io/apiserver dependency, upstream may have introduced a
				// new default-enabled admission plugin. If there is a good reason
				// against enabling it in openshift-apiserver, its name must be
				// included in omittedPlugins, otherwise, it should in the
				// appropriate position in downstreamPlugins.
				panic(fmt.Sprintf("k8s.io/apiserver default admission plugins includes %q which is in neither downstreamPlugins nor omittedPlugins: %v", upstreamPluginName, upstreamPlugins))
			}
		}

		for _, downstreamPluginName := range downstreamPlugins {
			if !slices.Contains(upstreamPlugins, downstreamPluginName) && !slices.Contains(downstreamOnlyPlugins, downstreamPluginName) {
				// If you are reading this because you are changing the version of
				// the k8s.io/apiserver dependency, upstream may have removed a
				// default-enabled admission plugin. If there is a good reason to
				// keep it enabled in openshift-apiserver, its name must be included
				// in downstreamOnlyPlugins, otherwise, it should be removed from
				// downstreamPlugins.
				panic(fmt.Sprintf("downstreamPlugins includes %q which is in neither downstreamOnlyPlugins nor the k8s.io/apiserver default admission plugins: %v", downstreamPluginName, upstreamPlugins))
			}
		}

		return downstreamPlugins
	}()
)
