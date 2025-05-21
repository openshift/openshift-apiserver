package localresourceaccessreview

import (
	"context"
	"errors"
	"reflect"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/user"
	kauthorizer "k8s.io/apiserver/pkg/authorization/authorizer"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	apiserverrest "k8s.io/apiserver/pkg/registry/rest"

	"github.com/openshift/library-go/pkg/authorization/authorizationutil"
	authorizationapi "github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/resourceaccessreview"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/util"
)

type resourceAccessTest struct {
	authorizer    *testAuthorizer
	reviewRequest *authorizationapi.LocalResourceAccessReview
}

type testAuthorizer struct {
	subjects []rbacv1.Subject
	err      string

	actualAttributes kauthorizer.Attributes
}

func (a *testAuthorizer) Authorize(ctx context.Context, attributes kauthorizer.Attributes) (decision kauthorizer.Decision, reason string, err error) {
	// allow the initial check for "can I run this RAR at all"
	if attributes.GetResource() == "localresourceaccessreviews" {
		return kauthorizer.DecisionAllow, "", nil
	}

	return kauthorizer.DecisionNoOpinion, "", errors.New("Unsupported")
}

func (a *testAuthorizer) AllowedSubjects(cxt context.Context, attributes kauthorizer.Attributes) ([]rbacv1.Subject, error) {
	a.actualAttributes = attributes
	if len(a.err) == 0 {
		return a.subjects, nil
	}
	return a.subjects, errors.New(a.err)
}

func TestNoNamespace(t *testing.T) {
	test := &resourceAccessTest{
		authorizer: &testAuthorizer{
			err: "namespace is required on this type: ",
		},
		reviewRequest: &authorizationapi.LocalResourceAccessReview{
			Action: authorizationapi.Action{
				Namespace: "",
				Verb:      "get",
				Resource:  "pods",
			},
		},
	}

	test.runTest(t)
}

func TestConflictingNamespace(t *testing.T) {
	authorizer := &testAuthorizer{}
	reviewRequest := &authorizationapi.LocalResourceAccessReview{
		Action: authorizationapi.Action{
			Namespace: "foo",
			Verb:      "get",
			Resource:  "pods",
		},
	}

	storage := NewREST(resourceaccessreview.NewRegistry(resourceaccessreview.NewREST(authorizer, authorizer)))
	ctx := apirequest.WithNamespace(apirequest.NewContext(), "bar")
	_, err := storage.Create(ctx, reviewRequest, apiserverrest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err == nil {
		t.Fatalf("unexpected non-error: %v", err)
	}
	if e, a := `namespace: Invalid value: "foo": namespace must be: bar`, err.Error(); e != a {
		t.Fatalf("expected %v, got %v", e, a)
	}
}

func TestEmptyReturn(t *testing.T) {
	test := &resourceAccessTest{
		authorizer: &testAuthorizer{},
		reviewRequest: &authorizationapi.LocalResourceAccessReview{
			Action: authorizationapi.Action{
				Namespace: "unittest",
				Verb:      "get",
				Resource:  "pods",
			},
		},
	}

	test.runTest(t)
}

func TestNoErrors(t *testing.T) {
	test := &resourceAccessTest{
		authorizer: &testAuthorizer{
			subjects: []rbacv1.Subject{
				{APIGroup: rbacv1.GroupName, Kind: rbacv1.UserKind, Name: "one"},
				{APIGroup: rbacv1.GroupName, Kind: rbacv1.UserKind, Name: "two"},
				{APIGroup: rbacv1.GroupName, Kind: rbacv1.GroupKind, Name: "three"},
				{APIGroup: rbacv1.GroupName, Kind: rbacv1.GroupKind, Name: "four"},
			},
		},
		reviewRequest: &authorizationapi.LocalResourceAccessReview{
			Action: authorizationapi.Action{
				Namespace: "unittest",
				Verb:      "delete",
				Resource:  "deploymentConfig",
			},
		},
	}

	test.runTest(t)
}

func (r *resourceAccessTest) runTest(t *testing.T) {
	storage := NewREST(resourceaccessreview.NewRegistry(resourceaccessreview.NewREST(r.authorizer, r.authorizer)))

	users, groups := authorizationutil.RBACSubjectsToUsersAndGroups(r.authorizer.subjects, r.reviewRequest.Action.Namespace)
	expectedResponse := &authorizationapi.ResourceAccessReviewResponse{
		Namespace: r.reviewRequest.Action.Namespace,
		Users:     sets.NewString(users...),
		Groups:    sets.NewString(groups...),
	}

	expectedAttributes := util.ToDefaultAuthorizationAttributes(nil, r.reviewRequest.Action.Namespace, r.reviewRequest.Action)

	ctx := apirequest.WithNamespace(apirequest.WithUser(apirequest.NewContext(), &user.DefaultInfo{}), r.reviewRequest.Action.Namespace)
	obj, err := storage.Create(ctx, r.reviewRequest, apiserverrest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil && len(r.authorizer.err) == 0 {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.authorizer.err) != 0 {
		if err == nil {
			t.Fatalf("unexpected non-error: %v", err)
		}
		if e, a := r.authorizer.err, err.Error(); e != a {
			t.Fatalf("expected %v, got %v", e, a)
		}

		return
	}

	switch obj.(type) {
	case *authorizationapi.ResourceAccessReviewResponse:
		if !reflect.DeepEqual(expectedResponse, obj) {
			t.Errorf("diff %v", diff.ObjectGoPrintDiff(expectedResponse, obj))
		}
	case nil:
		if len(r.authorizer.err) == 0 {
			t.Fatal("unexpected nil object")
		}
	default:
		t.Errorf("Unexpected obj type: %v", obj)
	}

	if !reflect.DeepEqual(expectedAttributes, r.authorizer.actualAttributes) {
		t.Errorf("diff %v", diff.ObjectGoPrintDiff(expectedAttributes, r.authorizer.actualAttributes))
	}
}
