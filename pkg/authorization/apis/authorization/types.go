package authorization

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	kapi "k8s.io/kubernetes/pkg/apis/core"
	rbacapi "k8s.io/kubernetes/pkg/apis/rbac"
)

// Authorization is calculated against
// 1. all deny RoleBinding PolicyRules in the master namespace - short circuit on match
// 2. all allow RoleBinding PolicyRules in the master namespace - short circuit on match
// 3. all deny RoleBinding PolicyRules in the namespace - short circuit on match
// 4. all allow RoleBinding PolicyRules in the namespace - short circuit on match
// 5. deny by default

const (
	// PolicyName is the name of Policy
	PolicyName     = "default"
	APIGroupAll    = "*"
	ResourceAll    = "*"
	VerbAll        = "*"
	NonResourceAll = "*"

	ScopesKey = "scopes.authorization.openshift.io"

	UserKind           = "User"
	GroupKind          = "Group"
	ServiceAccountKind = "ServiceAccount"
	SystemUserKind     = "SystemUser"
	SystemGroupKind    = "SystemGroup"
)

// PolicyRule holds information that describes a policy rule, but does not contain information
// about who the rule applies to or which namespace the rule applies to.
type PolicyRule struct {
	// Verbs is a list of Verbs that apply to ALL the ResourceKinds and AttributeRestrictions contained in this rule.  VerbAll represents all kinds.
	Verbs sets.String
	// AttributeRestrictions will vary depending on what the Authorizer/AuthorizationAttributeBuilder pair supports.
	// If the Authorizer does not recognize how to handle the AttributeRestrictions, the Authorizer should report an error.
	AttributeRestrictions kruntime.Object
	// APIGroups is the name of the APIGroup that contains the resources.  If this field is empty, then both kubernetes and origin API groups are assumed.
	// That means that if an action is requested against one of the enumerated resources in either the kubernetes or the origin API group, the request
	// will be allowed
	APIGroups []string
	// Resources is a list of resources this rule applies to.  ResourceAll represents all resources.
	Resources sets.String
	// ResourceNames is an optional white list of names that the rule applies to.  An empty set means that everything is allowed.
	ResourceNames sets.String
	// NonResourceURLs is a set of partial urls that a user should have access to.  *s are allowed, but only as the full, final step in the path
	// If an action is not a resource API request, then the URL is split on '/' and is checked against the NonResourceURLs to look for a match.
	NonResourceURLs sets.String
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IsPersonalSubjectAccessReview is a marker for PolicyRule.AttributeRestrictions that denotes that subjectaccessreviews on self should be allowed
type IsPersonalSubjectAccessReview struct {
	metav1.TypeMeta
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Role is a logical grouping of PolicyRules that can be referenced as a unit by RoleBindings.
type Role struct {
	metav1.TypeMeta
	// Standard object's metadata.
	metav1.ObjectMeta

	// Rules holds all the PolicyRules for this Role
	Rules []PolicyRule
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RoleBinding references a Role, but not contain it.  It can reference any Role in the same namespace or in the global namespace.
// It adds who information via Users and Groups and namespace information by which namespace it exists in.  RoleBindings in a given
// namespace only have effect in that namespace (excepting the master namespace which has power in all namespaces).
type RoleBinding struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// Subjects hold object references of to authorize with this rule
	Subjects []kapi.ObjectReference

	// RoleRef can only reference the current namespace and the global namespace
	// If the RoleRef cannot be resolved, the Authorizer must return an error.
	// Since Policy is a singleton, this is sufficient knowledge to locate a role
	RoleRef kapi.ObjectReference
}

// +genclient
// +genclient:onlyVerbs=create
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SelfSubjectRulesReview is a resource you can create to determine which actions you can perform in a namespace
type SelfSubjectRulesReview struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// Spec adds information about how to conduct the check
	Spec SelfSubjectRulesReviewSpec

	// Status is completed by the server to tell which permissions you have
	Status SubjectRulesReviewStatus
}

// SelfSubjectRulesReviewSpec adds information about how to conduct the check
type SelfSubjectRulesReviewSpec struct {
	// Scopes to use for the evaluation.  Empty means "use the unscoped (full) permissions of the user/groups".
	// Nil for a self-SubjectRulesReview, means "use the scopes on this request".
	// Nil for a regular SubjectRulesReview, means the same as empty.
	// +k8s:conversion-gen=false
	Scopes []string
}

// +genclient
// +genclient:onlyVerbs=create
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SubjectRulesReview is a resource you can create to determine which actions another user can perform in a namespace
type SubjectRulesReview struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// Spec adds information about how to conduct the check
	Spec SubjectRulesReviewSpec

	// Status is completed by the server to tell which permissions you have
	Status SubjectRulesReviewStatus
}

// SubjectRulesReviewSpec adds information about how to conduct the check
type SubjectRulesReviewSpec struct {
	// User is optional.  At least one of User and Groups must be specified.
	User string
	// Groups is optional.  Groups is the list of groups to which the User belongs.  At least one of User and Groups must be specified.
	Groups []string
	// Scopes to use for the evaluation.  Empty means "use the unscoped (full) permissions of the user/groups".
	Scopes []string
}

// SubjectRulesReviewStatus is contains the result of a rules check
type SubjectRulesReviewStatus struct {
	// Rules is the list of rules (no particular sort) that are allowed for the subject
	Rules []PolicyRule
	// EvaluationError can appear in combination with Rules.  It means some error happened during evaluation
	// that may have prevented additional rules from being populated.
	EvaluationError string
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceAccessReviewResponse describes who can perform the action
type ResourceAccessReviewResponse struct {
	metav1.TypeMeta

	// Namespace is the namespace used for the access review
	Namespace string
	// Users is the list of users who can perform the action
	// +k8s:conversion-gen=false
	Users sets.String
	// Groups is the list of groups who can perform the action
	// +k8s:conversion-gen=false
	Groups sets.String

	// EvaluationError is an indication that some error occurred during resolution, but partial results can still be returned.
	// It is entirely possible to get an error and be able to continue determine authorization status in spite of it.  This is
	// most common when a bound role is missing, but enough roles are still present and bound to reason about the request.
	EvaluationError string
}

// +genclient
// +genclient:nonNamespaced
// +genclient:skipVerbs=apply,applyStatus,get,list,create,update,updateStatus,patch,delete,deleteCollection,watch
// +genclient:method=Create,verb=create,result=ResourceAccessReviewResponse
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceAccessReview is a means to request a list of which users and groups are authorized to perform the
// action specified by spec
type ResourceAccessReview struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// Action describes the action being tested
	Action
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SubjectAccessReviewResponse describes whether or not a user or group can perform an action
type SubjectAccessReviewResponse struct {
	metav1.TypeMeta

	// Namespace is the namespace used for the access review
	Namespace string
	// Allowed is required.  True if the action would be allowed, false otherwise.
	Allowed bool
	// Reason is optional.  It indicates why a request was allowed or denied.
	Reason string
	// EvaluationError is an indication that some error occurred during the authorization check.
	// It is entirely possible to get an error and be able to continue determine authorization status in spite of it.  This is
	// most common when a bound role is missing, but enough roles are still present and bound to reason about the request.
	EvaluationError string
}

// +genclient
// +genclient:nonNamespaced
// +genclient:skipVerbs=apply,applyStatus,get,list,create,update,updateStatus,patch,delete,deleteCollection,watch
// +genclient:method=Create,verb=create,result=SubjectAccessReviewResponse
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SubjectAccessReview is an object for requesting information about whether a user or group can perform an action
type SubjectAccessReview struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// Action describes the action being tested
	Action
	// User is optional.  If both User and Groups are empty, the current authenticated user is used.
	User string
	// Groups is optional.  Groups is the list of groups to which the User belongs.
	// +k8s:conversion-gen=false
	Groups sets.String
	// Scopes to use for the evaluation.  Empty means "use the unscoped (full) permissions of the user/groups".
	// Nil for a self-SAR, means "use the scopes on this request".
	// Nil for a regular SAR, means the same as empty.
	// +k8s:conversion-gen=false
	Scopes []string
}

// +genclient
// +genclient:skipVerbs=apply,applyStatus,get,list,create,update,updateStatus,patch,delete,deleteCollection,watch
// +genclient:method=Create,verb=create,result=ResourceAccessReviewResponse
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LocalResourceAccessReview is a means to request a list of which users and groups are authorized to perform the action specified by spec in a particular namespace
type LocalResourceAccessReview struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// Action describes the action being tested
	Action
}

// +genclient
// +genclient:skipVerbs=apply,applyStatus,get,list,create,update,updateStatus,patch,delete,deleteCollection,watch
// +genclient:method=Create,verb=create,result=SubjectAccessReviewResponse
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LocalSubjectAccessReview is an object for requesting information about whether a user or group can perform an action in a particular namespace
type LocalSubjectAccessReview struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// Action describes the action being tested.  The Namespace element is FORCED to the current namespace.
	Action
	// User is optional.  If both User and Groups are empty, the current authenticated user is used.
	User string
	// Groups is optional.  Groups is the list of groups to which the User belongs.
	// +k8s:conversion-gen=false
	Groups sets.String
	// Scopes to use for the evaluation.  Empty means "use the unscoped (full) permissions of the user/groups".
	// Nil for a self-SAR, means "use the scopes on this request".
	// Nil for a regular SAR, means the same as empty.
	// +k8s:conversion-gen=false
	Scopes []string
}

// Action describes a request to be authorized
type Action struct {
	// Namespace is the namespace of the action being requested.  Currently, there is no distinction between no namespace and all namespaces
	Namespace string
	// Verb is one of: get, list, watch, create, update, delete
	Verb string
	// Group is the API group of the resource
	Group string
	// Version is the API version of the resource
	Version string
	// Resource is one of the existing resource types
	Resource string
	// ResourceName is the name of the resource being requested for a "get" or deleted for a "delete"
	ResourceName string
	// Path is the path of a non resource URL
	Path string
	// IsNonResourceURL is true if this is a request for a non-resource URL (outside of the resource hieraarchy)
	IsNonResourceURL bool
	// Content is the actual content of the request for create and update
	Content kruntime.Object
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RoleBindingList is a collection of RoleBindings
type RoleBindingList struct {
	metav1.TypeMeta
	// Standard object's metadata.
	metav1.ListMeta

	// Items is a list of roleBindings
	Items []RoleBinding
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RoleList is a collection of Roles
type RoleList struct {
	metav1.TypeMeta
	// Standard object's metadata.
	metav1.ListMeta

	// Items is a list of roles
	Items []Role
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterRole is a logical grouping of PolicyRules that can be referenced as a unit by ClusterRoleBindings.
type ClusterRole struct {
	metav1.TypeMeta
	// Standard object's metadata.
	metav1.ObjectMeta

	// Rules holds all the PolicyRules for this ClusterRole
	Rules []PolicyRule

	// AggregationRule is an optional field that describes how to build the Rules for this ClusterRole.
	// If AggregationRule is set, then the Rules are controller managed and direct changes to Rules will be
	// stomped by the controller.
	AggregationRule *rbacapi.AggregationRule
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterRoleBinding references a ClusterRole, but not contain it.  It can reference any ClusterRole in the same namespace or in the global namespace.
// It adds who information via Users and Groups and namespace information by which namespace it exists in.  ClusterRoleBindings in a given
// namespace only have effect in that namespace (excepting the master namespace which has power in all namespaces).
type ClusterRoleBinding struct {
	metav1.TypeMeta
	// Standard object's metadata.
	metav1.ObjectMeta

	// Subjects hold object references of to authorize with this rule
	Subjects []kapi.ObjectReference

	// RoleRef can only reference the current namespace and the global namespace
	// If the ClusterRoleRef cannot be resolved, the Authorizer must return an error.
	// Since Policy is a singleton, this is sufficient knowledge to locate a role
	RoleRef kapi.ObjectReference
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterRoleBindingList is a collection of ClusterRoleBindings
type ClusterRoleBindingList struct {
	metav1.TypeMeta
	// Standard object's metadata.
	metav1.ListMeta

	// Items is a list of ClusterRoleBindings
	Items []ClusterRoleBinding
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterRoleList is a collection of ClusterRoles
type ClusterRoleList struct {
	metav1.TypeMeta
	// Standard object's metadata.
	metav1.ListMeta

	// Items is a list of ClusterRoles
	Items []ClusterRole
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RoleBindingRestriction is an object that can be matched against a subject
// (user, group, or service account) to determine whether rolebindings on that
// subject are allowed in the namespace to which the RoleBindingRestriction
// belongs.  If any one of those RoleBindingRestriction objects matches
// a subject, rolebindings on that subject in the namespace are allowed.
type RoleBindingRestriction struct {
	metav1.TypeMeta

	// Standard object's metadata.
	metav1.ObjectMeta

	// Spec defines the matcher.
	Spec RoleBindingRestrictionSpec
}

// RoleBindingRestrictionSpec defines a rolebinding restriction.  Exactly one
// field must be non-nil.
type RoleBindingRestrictionSpec struct {
	// UserRestriction matches against user subjects.
	UserRestriction *UserRestriction

	// GroupRestriction matches against group subjects.
	GroupRestriction *GroupRestriction

	// ServiceAccountRestriction matches against service-account subjects.
	ServiceAccountRestriction *ServiceAccountRestriction
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RoleBindingRestrictionList is a collection of RoleBindingRestriction objects.
type RoleBindingRestrictionList struct {
	metav1.TypeMeta

	// Standard object's metadata.
	metav1.ListMeta

	// Items is a list of RoleBindingRestriction objects.
	Items []RoleBindingRestriction
}

// UserRestriction matches a user either by a string match on the user name,
// a string match on the name of a group to which the user belongs, or a label
// selector applied to the user labels.
type UserRestriction struct {
	// Users specifies a list of literal user names.
	Users []string

	// Groups is a list of groups used to match against an individual user's
	// groups. If the user is a member of one of the whitelisted groups, the user
	// is allowed to be bound to a role.
	Groups []string

	// Selectors specifies a list of label selectors over user labels.
	Selectors []metav1.LabelSelector
}

// GroupRestriction matches a group either by a string match on the group name
// or a label selector applied to group labels.
type GroupRestriction struct {
	// Groups specifies a list of literal group names.
	Groups []string

	// Selectors specifies a list of label selectors over group labels.
	Selectors []metav1.LabelSelector
}

// ServiceAccountRestriction matches a service account by a string match on
// either the service-account name or the name of the service account's
// namespace.
type ServiceAccountRestriction struct {
	// ServiceAccounts specifies a list of literal service-account names.
	ServiceAccounts []ServiceAccountReference

	// Namespaces specifies a list of literal namespace names.  ServiceAccounts
	// from inside the whitelisted namespaces are allowed to be bound to roles.
	Namespaces []string
}

// ServiceAccountReference specifies a service account and namespace by their
// names.
type ServiceAccountReference struct {
	// Name is the name of the service account.
	Name string

	// Namespace is the namespace of the service account.  Service accounts from
	// inside the whitelisted namespaces are allowed to be bound to roles.  If
	// Namespace is empty, then the namespace of the RoleBindingRestriction in
	// which the ServiceAccountReference is embedded is used.
	Namespace string
}
