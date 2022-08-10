package bootstrappolicy

import (
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/apis/apps"
	kauthenticationapi "k8s.io/kubernetes/pkg/apis/authentication"
	kauthorizationapi "k8s.io/kubernetes/pkg/apis/authorization"
	"k8s.io/kubernetes/pkg/apis/autoscaling"
	"k8s.io/kubernetes/pkg/apis/batch"
	"k8s.io/kubernetes/pkg/apis/certificates"
	"k8s.io/kubernetes/pkg/apis/coordination"
	kapi "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/apis/policy"
	"k8s.io/kubernetes/pkg/apis/rbac"
	rbacv1helpers "k8s.io/kubernetes/pkg/apis/rbac/v1"
	"k8s.io/kubernetes/pkg/apis/storage"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"

	oapps "github.com/openshift/api/apps"
	"github.com/openshift/api/authorization"
	"github.com/openshift/api/build"
	"github.com/openshift/api/config"
	"github.com/openshift/api/image"
	"github.com/openshift/api/network"
	"github.com/openshift/api/oauth"
	"github.com/openshift/api/project"
	"github.com/openshift/api/quota"
	"github.com/openshift/api/route"
	"github.com/openshift/api/security"
	"github.com/openshift/api/template"
	"github.com/openshift/api/user"
)

var (
	readWrite = []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}
	read      = []string{"get", "list", "watch"}

	kapiGroup                  = kapi.GroupName
	admissionRegistrationGroup = "admissionregistration.k8s.io"
	appsGroup                  = apps.GroupName
	autoscalingGroup           = autoscaling.GroupName
	apiExtensionsGroup         = "apiextensions.k8s.io"
	eventsGroup                = "events.k8s.io"
	apiRegistrationGroup       = "apiregistration.k8s.io"
	batchGroup                 = batch.GroupName
	certificatesGroup          = certificates.GroupName
	coordinationGroup          = coordination.GroupName
	extensionsGroup            = extensions.GroupName
	networkingGroup            = "networking.k8s.io"
	nodeGroup                  = "node.k8s.io"
	policyGroup                = policy.GroupName
	rbacGroup                  = rbac.GroupName
	storageGroup               = storage.GroupName
	schedulingGroup            = "scheduling.k8s.io"
	kAuthzGroup                = kauthorizationapi.GroupName
	kAuthnGroup                = kauthenticationapi.GroupName
	discoveryGroup             = "discovery.k8s.io"

	deployGroup         = oapps.GroupName
	authzGroup          = authorization.GroupName
	buildGroup          = build.GroupName
	configGroup         = config.GroupName
	imageGroup          = image.GroupName
	networkGroup        = network.GroupName
	oauthGroup          = oauth.GroupName
	projectGroup        = project.GroupName
	quotaGroup          = quota.GroupName
	routeGroup          = route.GroupName
	securityGroup       = security.GroupName
	templateGroup       = template.GroupName
	userGroup           = user.GroupName
	legacyAuthzGroup    = ""
	legacyBuildGroup    = ""
	legacyDeployGroup   = ""
	legacyImageGroup    = ""
	legacyProjectGroup  = ""
	legacyQuotaGroup    = ""
	legacyRouteGroup    = ""
	legacyTemplateGroup = ""
	legacyUserGroup     = ""
	legacyOauthGroup    = ""
	legacyNetworkGroup  = ""
	legacySecurityGroup = ""

	userResource        = "users"
	groupResource       = "groups"
	systemUserResource  = "systemusers"
	systemGroupResource = "systemgroups"

	// discoveryRule is a rule that allows a client to discover the API resources available on this server
	discoveryRule = rbacv1.PolicyRule{
		Verbs: []string{"get"},
		NonResourceURLs: []string{
			// Server version checking
			"/version", "/version/*",

			// API discovery/negotiation
			"/api", "/api/*",
			"/apis", "/apis/*",
			"/oapi", "/oapi/*",
			"/openapi/v2",
			"/swaggerapi", "/swaggerapi/*", "/swagger.json", "/swagger-2.0.0.pb-v1",
			"/osapi", "/osapi/", // these cannot be removed until we can drop support for pre 3.1 clients
			"/.well-known", "/.well-known/oauth-authorization-server",

			// we intentionally allow all to here
			"/",
		},
	}

	// serviceBrokerRoot is the API root of the template service broker.
	serviceBrokerRoot = "/brokers/template.openshift.io"

	// openShiftDescription is a common, optional annotation that stores the description for a resource.
	openShiftDescription = "openshift.io/description"
)

func newOriginRoleBinding(bindingName, roleName, namespace string) *rbacv1helpers.RoleBindingBuilder {
	builder := rbacv1helpers.NewRoleBinding(roleName, namespace)
	builder.RoleBinding.Name = bindingName
	return builder
}

// TODO we need to remove the global mutable state from all roles / bindings so we know this function is safe to call
func addDefaultMetadata(obj runtime.Object) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		// if this happens, then some static code is broken
		panic(err)
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	for k, v := range bootstrappolicy.Annotation {
		annotations[k] = v
	}
	metadata.SetAnnotations(annotations)
}
