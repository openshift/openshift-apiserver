package bootstrappolicy

import (
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	rbacv1helpers "k8s.io/kubernetes/pkg/apis/rbac/v1"
)

func addNamespaceRole(namespaceRoles map[string][]rbacv1.Role, namespace string, role rbacv1.Role) {
	if namespace != "openshift" && !strings.HasPrefix(namespace, "openshift-") {
		klog.Fatalf(`roles can only be bootstrapped into reserved "openshift" namespace or namespaces starting with "openshift-", not %q`, namespace)
	}

	existingRoles := namespaceRoles[namespace]
	for _, existingRole := range existingRoles {
		if role.Name == existingRole.Name {
			klog.Fatalf("role %q was already registered in %q", role.Name, namespace)
		}
	}

	role.Namespace = namespace
	addDefaultMetadata(&role)
	existingRoles = append(existingRoles, role)
	namespaceRoles[namespace] = existingRoles
}

func addNamespaceRoleBinding(namespaceRoleBindings map[string][]rbacv1.RoleBinding, namespace string, roleBinding rbacv1.RoleBinding) {
	if namespace != "openshift" && !strings.HasPrefix(namespace, "openshift-") {
		klog.Fatalf(`role bindings can only be bootstrapped into reserved "openshift" namespace or namespaces starting with "openshift-", not %q`, namespace)
	}

	existingRoleBindings := namespaceRoleBindings[namespace]
	for _, existingRoleBinding := range existingRoleBindings {
		if roleBinding.Name == existingRoleBinding.Name {
			klog.Fatalf("rolebinding %q was already registered in %q", roleBinding.Name, namespace)
		}
	}

	roleBinding.Namespace = namespace
	addDefaultMetadata(&roleBinding)
	existingRoleBindings = append(existingRoleBindings, roleBinding)
	namespaceRoleBindings[namespace] = existingRoleBindings
}

func buildNamespaceRolesAndBindings() (map[string][]rbacv1.Role, map[string][]rbacv1.RoleBinding) {
	// namespaceRoles is a map of namespace to slice of roles to create
	namespaceRoles := map[string][]rbacv1.Role{}
	// namespaceRoleBindings is a map of namespace to slice of roleBindings to create
	namespaceRoleBindings := map[string][]rbacv1.RoleBinding{}

	addNamespaceRole(namespaceRoles,
		DefaultOpenShiftSharedResourcesNamespace,
		rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: OpenshiftSharedResourceViewRoleName},
			Rules: []rbacv1.PolicyRule{
				rbacv1helpers.NewRule(read...).Groups(templateGroup, legacyTemplateGroup).Resources("templates").RuleOrDie(),
				rbacv1helpers.NewRule(read...).Groups(imageGroup, legacyImageGroup).Resources("imagestreams", "imagestreamtags", "imagetags", "imagestreamimages").RuleOrDie(),
				// so anyone can pull from openshift/* image streams
				rbacv1helpers.NewRule("get").Groups(imageGroup, legacyImageGroup).Resources("imagestreams/layers").RuleOrDie(),
				// so anyone can view from openshift/* configmaps. e.g. The motd configmap
				rbacv1helpers.NewRule("get").Groups("").Resources("configmaps").RuleOrDie(),
			},
		})

	addNamespaceRoleBinding(namespaceRoleBindings,
		DefaultOpenShiftSharedResourcesNamespace,
		newOriginRoleBinding(OpenshiftSharedResourceViewRoleBindingName, OpenshiftSharedResourceViewRoleName, DefaultOpenShiftSharedResourcesNamespace).Groups(AuthenticatedGroup).BindingOrDie())

	addNamespaceRole(namespaceRoles,
		DefaultOpenShiftNodeNamespace,
		rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: NodeConfigReaderRoleName},
			Rules: []rbacv1.PolicyRule{
				// Allow the reader to read config maps in a given namespace with a given name.
				rbacv1helpers.NewRule("get").Groups(kapiGroup).Resources("configmaps").RuleOrDie(),
			},
		})
	addNamespaceRoleBinding(namespaceRoleBindings,
		DefaultOpenShiftNodeNamespace,
		rbacv1helpers.NewRoleBinding(NodeConfigReaderRoleName, DefaultOpenShiftNodeNamespace).Groups(NodesGroup).BindingOrDie())

	return namespaceRoles, namespaceRoleBindings
}

// NamespaceRoles returns a map of namespace to slice of roles to create
func NamespaceRoles() map[string][]rbacv1.Role {
	namespaceRoles, _ := buildNamespaceRolesAndBindings()
	return namespaceRoles
}

// NamespaceRoleBindings returns a map of namespace to slice of role bindings to create
func NamespaceRoleBindings() map[string][]rbacv1.RoleBinding {
	_, namespaceRoleBindings := buildNamespaceRolesAndBindings()
	return namespaceRoleBindings
}
