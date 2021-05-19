package bootstrappolicy

import (
	"strings"

	"k8s.io/klog/v2"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1helpers "k8s.io/kubernetes/pkg/apis/rbac/v1"
)

const saRolePrefix = "system:openshift:controller:"

const (
	InfraServiceAccountPullSecretsControllerServiceAccountName  = "serviceaccount-pull-secrets-controller"
	InfraServiceServingCertServiceAccountName                   = "service-serving-cert-controller"
	InfraImageTriggerControllerServiceAccountName               = "image-trigger-controller"
	InfraImageImportControllerServiceAccountName                = "image-import-controller"
	InfraClusterQuotaReconciliationControllerServiceAccountName = "cluster-quota-reconciliation-controller"
	InfraUnidlingControllerServiceAccountName                   = "unidling-controller"
	InfraServiceIngressIPControllerServiceAccountName           = "service-ingress-ip-controller"
	InfraPersistentVolumeRecyclerControllerServiceAccountName   = "pv-recycler-controller"
	InfraResourceQuotaControllerServiceAccountName              = "resourcequota-controller"
	InfraDefaultRoleBindingsControllerServiceAccountName        = "default-rolebindings-controller"

	// template service broker is an open service broker-compliant API
	// implementation which serves up OpenShift templates.  It uses the
	// TemplateInstance backend for most of the heavy lifting.
	InfraTemplateServiceBrokerServiceAccountName = "template-service-broker"

	// This is a special constant which maps to the service account name used by the underlying
	// Kubernetes code, so that we can build out the extra policy required to scale OpenShift resources.
	InfraHorizontalPodAutoscalerControllerServiceAccountName = "horizontal-pod-autoscaler"
)

var (
	// controllerRoles is a slice of roles used for controllers
	controllerRoles = []rbacv1.ClusterRole{}
	// controllerRoleBindings is a slice of roles used for controllers
	controllerRoleBindings = []rbacv1.ClusterRoleBinding{}
)

func bindControllerRole(saName string, roleName string) {
	roleBinding := rbacv1helpers.NewClusterBinding(roleName).SAs(DefaultOpenShiftInfraNamespace, saName).BindingOrDie()
	addDefaultMetadata(&roleBinding)
	controllerRoleBindings = append(controllerRoleBindings, roleBinding)
}

func addControllerRole(role rbacv1.ClusterRole) {
	if !strings.HasPrefix(role.Name, saRolePrefix) {
		klog.Fatalf(`role %q must start with %q`, role.Name, saRolePrefix)
	}
	addControllerRoleToSA(DefaultOpenShiftInfraNamespace, role.Name[len(saRolePrefix):], role)
}

func addControllerRoleToSA(saNamespace, saName string, role rbacv1.ClusterRole) {
	if !strings.HasPrefix(role.Name, saRolePrefix) {
		klog.Fatalf(`role %q must start with %q`, role.Name, saRolePrefix)
	}

	for _, existingRole := range controllerRoles {
		if role.Name == existingRole.Name {
			klog.Fatalf("role %q was already registered", role.Name)
		}
	}

	addDefaultMetadata(&role)
	controllerRoles = append(controllerRoles, role)

	roleBinding := rbacv1helpers.NewClusterBinding(role.Name).SAs(saNamespace, saName).BindingOrDie()
	addDefaultMetadata(&roleBinding)
	controllerRoleBindings = append(controllerRoleBindings, roleBinding)
}

func eventsRule() rbacv1.PolicyRule {
	return rbacv1helpers.NewRule("create", "update", "patch").Groups(kapiGroup).Resources("events").RuleOrDie()
}

func init() {

	// serviceaccount-pull-secrets-controller
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraServiceAccountPullSecretsControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("get", "list", "watch", "create", "update").Groups(kapiGroup).Resources("serviceaccounts").RuleOrDie(),
			rbacv1helpers.NewRule("get", "list", "watch", "create", "update", "patch", "delete").Groups(kapiGroup).Resources("secrets").RuleOrDie(),
			rbacv1helpers.NewRule("get", "list", "watch").Groups(kapiGroup).Resources("services").RuleOrDie(),
			eventsRule(),
		},
	})

	// imagetrigger-controller
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraImageTriggerControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("list", "watch").Groups(imageGroup, legacyImageGroup).Resources("imagestreams").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update").Groups(extensionsGroup).Resources("daemonsets").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update").Groups(extensionsGroup, appsGroup).Resources("deployments").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update").Groups(appsGroup).Resources("statefulsets").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update").Groups(batchGroup).Resources("cronjobs").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update").Groups(deployGroup, legacyDeployGroup).Resources("deploymentconfigs").RuleOrDie(),
			rbacv1helpers.NewRule("create").Groups(buildGroup, legacyBuildGroup).Resources("buildconfigs/instantiate").RuleOrDie(),
			// trigger controller must be able to modify these build types
			// TODO: move to a new custom binding that can be removed separately from end user access?
			rbacv1helpers.NewRule("create").Groups(buildGroup, legacyBuildGroup).Resources(
				SourceBuildResource,
				DockerBuildResource,
				CustomBuildResource,
				OptimizedDockerBuildResource,
				JenkinsPipelineBuildResource,
			).RuleOrDie(),

			eventsRule(),
		},
	})

	// service-serving-cert-controller
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraServiceServingCertServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("list", "watch", "update").Groups(kapiGroup).Resources("services").RuleOrDie(),
			rbacv1helpers.NewRule("get", "list", "watch", "create", "update", "delete").Groups(kapiGroup).Resources("secrets").RuleOrDie(),
			eventsRule(),
		},
	})

	// image-import-controller
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraImageImportControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("get", "list", "watch", "create", "update").Groups(imageGroup, legacyImageGroup).Resources("imagestreams").RuleOrDie(),
			rbacv1helpers.NewRule("get", "list", "watch", "create", "update", "patch", "delete").Groups(imageGroup, legacyImageGroup).Resources("images").RuleOrDie(),
			rbacv1helpers.NewRule("create").Groups(imageGroup, legacyImageGroup).Resources("imagestreamimports").RuleOrDie(),
			eventsRule(),
		},
	})

	// cluster-quota-reconciliation
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraClusterQuotaReconciliationControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("get", "list").Groups(kapiGroup).Resources("configmaps").RuleOrDie(),
			rbacv1helpers.NewRule("get", "list").Groups(kapiGroup).Resources("secrets").RuleOrDie(),
			rbacv1helpers.NewRule("update").Groups(quotaGroup, legacyQuotaGroup).Resources("clusterresourcequotas/status").RuleOrDie(),
			eventsRule(),
		},
	})

	// unidling-controller
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraUnidlingControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("get", "update").Groups(kapiGroup).Resources("replicationcontrollers/scale", "endpoints", "services").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update", "patch").Groups(kapiGroup).Resources("replicationcontrollers").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update", "patch").Groups(deployGroup, legacyDeployGroup).Resources("deploymentconfigs").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update").Groups(extensionsGroup, appsGroup).Resources("replicasets/scale", "deployments/scale").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update").Groups(deployGroup, legacyDeployGroup).Resources("deploymentconfigs/scale").RuleOrDie(),
			rbacv1helpers.NewRule("watch", "list").Groups(kapiGroup).Resources("events").RuleOrDie(),
			eventsRule(),
		},
	})

	// ingress-ip-controller
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraServiceIngressIPControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("list", "watch", "update").Groups(kapiGroup).Resources("services").RuleOrDie(),
			rbacv1helpers.NewRule("update").Groups(kapiGroup).Resources("services/status").RuleOrDie(),
			eventsRule(),
		},
	})

	// pv-recycler-controller
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraPersistentVolumeRecyclerControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("get", "update", "create", "delete", "list", "watch").Groups(kapiGroup).Resources("persistentvolumes").RuleOrDie(),
			rbacv1helpers.NewRule("update").Groups(kapiGroup).Resources("persistentvolumes/status").RuleOrDie(),
			rbacv1helpers.NewRule("get", "update", "list", "watch").Groups(kapiGroup).Resources("persistentvolumeclaims").RuleOrDie(),
			rbacv1helpers.NewRule("update").Groups(kapiGroup).Resources("persistentvolumeclaims/status").RuleOrDie(),
			rbacv1helpers.NewRule("get", "create", "delete", "list", "watch").Groups(kapiGroup).Resources("pods").RuleOrDie(),
			eventsRule(),
		},
	})

	// resourcequota-controller
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraResourceQuotaControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("update").Groups(kapiGroup).Resources("resourcequotas/status").RuleOrDie(),
			rbacv1helpers.NewRule("list").Groups(kapiGroup).Resources("resourcequotas").RuleOrDie(),
			rbacv1helpers.NewRule("list").Groups(kapiGroup).Resources("services").RuleOrDie(),
			rbacv1helpers.NewRule("list").Groups(kapiGroup).Resources("configmaps").RuleOrDie(),
			rbacv1helpers.NewRule("list").Groups(kapiGroup).Resources("secrets").RuleOrDie(),
			rbacv1helpers.NewRule("list").Groups(kapiGroup).Resources("replicationcontrollers").RuleOrDie(),
			eventsRule(),
		},
	})

	// horizontal-pod-autoscaler-controller (the OpenShift resources only)
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraHorizontalPodAutoscalerControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("get", "update").Groups(deployGroup, legacyDeployGroup).Resources("deploymentconfigs/scale").RuleOrDie(),
		},
	})

	bindControllerRole(InfraHorizontalPodAutoscalerControllerServiceAccountName, "system:controller:horizontal-pod-autoscaler")

	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraTemplateServiceBrokerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("create").Groups(kAuthzGroup).Resources("subjectaccessreviews").RuleOrDie(),
			rbacv1helpers.NewRule("create").Groups(authzGroup).Resources("subjectaccessreviews").RuleOrDie(),
			rbacv1helpers.NewRule("get", "create", "update", "delete").Groups(templateGroup).Resources("brokertemplateinstances").RuleOrDie(),
			rbacv1helpers.NewRule("update").Groups(templateGroup).Resources("brokertemplateinstances/finalizers").RuleOrDie(),
			rbacv1helpers.NewRule("get", "create", "delete", "assign").Groups(templateGroup).Resources("templateinstances").RuleOrDie(),
			rbacv1helpers.NewRule("get", "list", "watch").Groups(templateGroup).Resources("templates").RuleOrDie(),
			rbacv1helpers.NewRule("get", "create", "delete").Groups(kapiGroup).Resources("secrets").RuleOrDie(),
			rbacv1helpers.NewRule("get").Groups(kapiGroup).Resources("services", "configmaps").RuleOrDie(),
			rbacv1helpers.NewRule("get").Groups(legacyRouteGroup).Resources("routes").RuleOrDie(),
			rbacv1helpers.NewRule("get").Groups(routeGroup).Resources("routes").RuleOrDie(),
			eventsRule(),
		},
	})

	// the controller needs to be bound to the roles it is going to try to create
	bindControllerRole(InfraDefaultRoleBindingsControllerServiceAccountName, ImagePullerRoleName)
	bindControllerRole(InfraDefaultRoleBindingsControllerServiceAccountName, ImageBuilderRoleName)
	bindControllerRole(InfraDefaultRoleBindingsControllerServiceAccountName, DeployerRoleName)
	addControllerRole(rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: saRolePrefix + InfraDefaultRoleBindingsControllerServiceAccountName},
		Rules: []rbacv1.PolicyRule{
			rbacv1helpers.NewRule("create").Groups(rbacGroup).Resources("rolebindings").RuleOrDie(),
			rbacv1helpers.NewRule("get", "list", "watch").Groups(kapiGroup).Resources("namespaces").RuleOrDie(),
			rbacv1helpers.NewRule("get", "list", "watch").Groups(rbacGroup).Resources("rolebindings").RuleOrDie(),
			eventsRule(),
		},
	})
}

// ControllerRoles returns the cluster roles used by controllers
func ControllerRoles() []rbacv1.ClusterRole {
	return controllerRoles
}

// ControllerRoleBindings returns the role bindings used by controllers
func ControllerRoleBindings() []rbacv1.ClusterRoleBinding {
	return controllerRoleBindings
}
