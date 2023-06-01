package selfsubjectrulesreview

import (
	"context"
	"fmt"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kutilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	rbaclisters "k8s.io/client-go/listers/rbac/v1"
	rbacregistryvalidation "k8s.io/kubernetes/pkg/registry/rbac/validation"

	authorizationv1 "github.com/openshift/api/authorization/v1"
	authorizationapi "github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization/rbacconversion"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/subjectrulesreview"
)

type REST struct {
	ruleResolver      rbacregistryvalidation.AuthorizationRuleResolver
	clusterRoleGetter rbaclisters.ClusterRoleLister
}

var _ rest.Creater = &REST{}
var _ rest.Scoper = &REST{}
var _ rest.Storage = &REST{}
var _ rest.SingularNameProvider = &REST{}

func NewREST(ruleResolver rbacregistryvalidation.AuthorizationRuleResolver, clusterRoleGetter rbaclisters.ClusterRoleLister) *REST {
	return &REST{ruleResolver: ruleResolver, clusterRoleGetter: clusterRoleGetter}
}

func (r *REST) New() runtime.Object {
	return &authorizationapi.SelfSubjectRulesReview{}
}

func (r *REST) Destroy() {}

func (r *REST) NamespaceScoped() bool {
	return true
}

func (s *REST) GetSingularName() string {
	return "selfsubjectrulesreview"
}

// Create registers a given new ResourceAccessReview instance to r.registry.
func (r *REST) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	rulesReview, ok := obj.(*authorizationapi.SelfSubjectRulesReview)
	if !ok {
		return nil, kapierrors.NewBadRequest(fmt.Sprintf("not a SelfSubjectRulesReview: %#v", obj))
	}
	namespace := apirequest.NamespaceValue(ctx)
	if len(namespace) == 0 {
		return nil, kapierrors.NewBadRequest(fmt.Sprintf("namespace is required on this type: %v", namespace))
	}
	callingUser, exists := apirequest.UserFrom(ctx)
	if !exists {
		return nil, kapierrors.NewBadRequest(fmt.Sprintf("user missing from context"))
	}

	// copy the user to avoid mutating the original extra map
	userToCheck := &user.DefaultInfo{
		Name:   callingUser.GetName(),
		Groups: callingUser.GetGroups(),
		Extra:  map[string][]string{},
	}
	switch {
	case rulesReview.Spec.Scopes == nil:
		for k, v := range callingUser.GetExtra() {
			userToCheck.Extra[k] = v
		}

	case len(rulesReview.Spec.Scopes) > 0:
		userToCheck.Extra[authorizationv1.ScopesKey] = rulesReview.Spec.Scopes
	}

	rules, errors := subjectrulesreview.GetEffectivePolicyRules(apirequest.WithUser(ctx, userToCheck), r.ruleResolver, r.clusterRoleGetter)

	ret := &authorizationapi.SelfSubjectRulesReview{
		Status: authorizationapi.SubjectRulesReviewStatus{
			Rules: rbacconversion.Convert_rbacv1_PolicyRules_To_authorization_PolicyRules(rules), //TODO can we fix this ?
		},
	}

	if len(errors) != 0 {
		ret.Status.EvaluationError = kutilerrors.NewAggregate(errors).Error()
	}

	return ret, nil
}
