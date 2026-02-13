package proxy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authentication/user"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	oapi "github.com/openshift/openshift-apiserver/pkg/api"
	projectapi "github.com/openshift/openshift-apiserver/pkg/project/apis/project"
)

// mockLister returns the namespaces in the list
type mockLister struct {
	namespaceList *corev1.NamespaceList
}

func (ml *mockLister) List(user user.Info, selector labels.Selector) (*corev1.NamespaceList, error) {
	return ml.namespaceList, nil
}

func TestListProjects(t *testing.T) {
	namespaceList := corev1.NamespaceList{
		Items: []corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
			},
		},
	}
	mockClient := fake.NewSimpleClientset(&namespaceList)
	storage := REST{
		client: mockClient.CoreV1().Namespaces(),
		lister: &mockLister{&namespaceList},
	}
	user := &user.DefaultInfo{
		Name:   "test-user",
		UID:    "test-uid",
		Groups: []string{"test-groups"},
	}
	ctx := apirequest.WithUser(apirequest.NewContext(), user)
	response, err := storage.List(ctx, nil)
	if err != nil {
		t.Errorf("%#v should be nil.", err)
	}
	projects := response.(*projectapi.ProjectList)
	if len(projects.Items) != 1 {
		t.Errorf("%#v projects.Items should have len 1.", projects.Items)
	}
	responseProject := projects.Items[0]
	if e, r := responseProject.Name, "foo"; e != r {
		t.Errorf("%#v != %#v.", e, r)
	}
}

func TestCreateProjectBadObject(t *testing.T) {
	storage := REST{}

	obj, err := storage.Create(apirequest.NewContext(), &projectapi.ProjectList{}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if obj != nil {
		t.Errorf("Expected nil, got %v", obj)
	}
	if strings.Index(err.Error(), "not a project:") == -1 {
		t.Errorf("Expected 'not an project' error, got %v", err)
	}
}

func TestCreateInvalidProject(t *testing.T) {
	mockClient := &fake.Clientset{}
	storage := NewREST(mockClient.CoreV1().Namespaces(), &mockLister{}, nil, nil)
	_, err := storage.Create(apirequest.NewContext(), &projectapi.Project{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{oapi.OpenShiftDisplayName: "h\t\ni"},
		},
	}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if !kerrors.IsInvalid(err) {
		t.Errorf("Expected 'invalid' error, got %v", err)
	}
}

func TestCreateProjectOK(t *testing.T) {
	mockClient := &fake.Clientset{}
	storage := NewREST(mockClient.CoreV1().Namespaces(), &mockLister{}, nil, nil)
	_, err := storage.Create(apirequest.NewContext(), &projectapi.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "foo"},
	}, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected non-nil error: %#v", err)
	}
	if len(mockClient.Actions()) != 1 {
		t.Errorf("Expected client action for create")
	}
	if !mockClient.Actions()[0].Matches("create", "namespaces") {
		t.Errorf("Expected call to create-namespace")
	}
}

func TestCreateProjectValidation(t *testing.T) {
	mockClient := &fake.Clientset{}
	storage := NewREST(mockClient.CoreV1().Namespaces(), &mockLister{}, nil, nil)

	validationCalled := false
	validationFunc := func(ctx context.Context, obj runtime.Object) error {
		validationCalled = true
		return nil
	}

	_, err := storage.Create(apirequest.NewContext(), &projectapi.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "foo"},
	}, validationFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected non-nil error: %#v", err)
	}
	if !validationCalled {
		t.Errorf("Expected validation function to be called")
	}
}

func TestGetProjectOK(t *testing.T) {
	mockClient := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "foo"}})
	storage := NewREST(mockClient.CoreV1().Namespaces(), &mockLister{}, nil, nil)
	project, err := storage.Get(apirequest.NewContext(), "foo", &metav1.GetOptions{})
	if project == nil {
		t.Error("Unexpected nil project")
	}
	if err != nil {
		t.Errorf("Unexpected non-nil error: %v", err)
	}
	if project.(*projectapi.Project).Name != "foo" {
		t.Errorf("Unexpected project: %#v", project)
	}
}

type objectValidator struct {
	called     int
	objectFunc rest.ValidateObjectFunc
}

func (ov *objectValidator) ValidateObjectFunc() rest.ValidateObjectFunc {
	return func(ctx context.Context, obj runtime.Object) error {
		ov.called++
		return ov.objectFunc(ctx, obj)
	}
}

type reactionFunc func(called int) (runtime.Object, error)

type reactor struct {
	called        int
	expectedCalls int
	verb          string
	resource      string
	reaction      reactionFunc
}

func (r *reactor) Handles(action ktesting.Action) bool {
	if !(r.verb == "*" || r.verb == action.GetVerb()) {
		return false
	}

	actionableRes := action.GetResource().Resource
	if action.GetSubresource() != "" {
		actionableRes = fmt.Sprintf("%s/%s", action.GetResource().Resource, action.GetSubresource())
	}

	if !(r.resource == "*" || r.resource == actionableRes) {
		return false
	}

	return true
}

func (r *reactor) React(action ktesting.Action) (bool, runtime.Object, error) {
	var out runtime.Object
	var err error
	if r.Handles(action) {
		out, err = r.reaction(r.called)
		r.called++
	}

	return r.Handles(action), out, err
}

func newReactor(verb, resource string, reaction reactionFunc, expectedCalls int) *reactor {
	return &reactor{
		verb:          verb,
		resource:      resource,
		reaction:      reaction,
		expectedCalls: expectedCalls,
	}
}

func TestDeleteProject(t *testing.T) {
	type testcase struct {
		name                    string
		reactors                []*reactor
		objectValidator         *objectValidator
		options                 *metav1.DeleteOptions
		expectedStatus          *metav1.Status
		expectedErr             error
		expectedValidationCalls int
	}

	testcases := []testcase{
		{
			name:           "no validation, no options, request succeeds",
			expectedStatus: &metav1.Status{Status: metav1.StatusSuccess},
		},
		{
			name: "has validation, getting project succeeds, no options, validation successful, validation called once, request succeeds",
			objectValidator: &objectValidator{
				objectFunc: func(ctx context.Context, obj runtime.Object) error {
					return nil
				},
			},
			expectedStatus:          &metav1.Status{Status: metav1.StatusSuccess},
			expectedValidationCalls: 1,
		},
		{
			name: "has validation, getting project succeeds, no options, validation fails, validation retried, request fails",
			objectValidator: &objectValidator{
				objectFunc: func(ctx context.Context, obj runtime.Object) error {
					return errors.New("boom")
				},
			},
			expectedStatus:          &metav1.Status{Status: metav1.StatusFailure},
			expectedValidationCalls: 10,
			expectedErr:             errors.New("validating project: boom"),
		},
		{
			name: "has validation, getting project perma-fails with non-terminal error, no options, validation never called, request fails",
			reactors: []*reactor{
				newReactor(
					"get",
					"namespaces",
					func(called int) (runtime.Object, error) {
						return nil, errors.New("boom")
					},
					10,
				),
			},
			objectValidator: &objectValidator{
				objectFunc: func(ctx context.Context, obj runtime.Object) error {
					return nil
				},
			},
			expectedStatus: &metav1.Status{Status: metav1.StatusFailure},
			expectedErr:    errors.New("getting project for deletion: unable to get project: boom"),
		},
		{
			name: "has validation, getting project transient failure, no options, validation called once, validation successful, request succeeds",
			reactors: []*reactor{
				newReactor(
					"get",
					"namespaces",
					func(called int) (runtime.Object, error) {
						if called < 1 {
							return nil, errors.New("transient")
						}

						return nil, nil
					},
					2,
				),
			},
			objectValidator: &objectValidator{
				objectFunc: func(ctx context.Context, obj runtime.Object) error {
					return nil
				},
			},
			expectedStatus:          &metav1.Status{Status: metav1.StatusSuccess},
			expectedValidationCalls: 1,
		},
		{
			name: "has validation, getting project succeeds, uid precondition validation failed, validation not called, request fails",
			reactors: []*reactor{
				newReactor(
					"get",
					"namespaces",
					func(called int) (runtime.Object, error) {
						namespace := &corev1.Namespace{
							ObjectMeta: metav1.ObjectMeta{
								UID:  types.UID("two"),
								Name: "test",
							},
						}
						return namespace, nil
					},
					1,
				),
			},
			objectValidator: &objectValidator{
				objectFunc: func(ctx context.Context, obj runtime.Object) error {
					return nil
				},
			},
			options: &metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{
					UID:             ptr.To(types.UID("one")),
					ResourceVersion: ptr.To("12345"),
				},
			},
			expectedStatus: &metav1.Status{Status: metav1.StatusFailure},
			expectedErr:    errors.New("validating preconditions: uid precondition \"one\" does not match project uid \"two\""),
		},
		{
			name: "has validation, getting project succeeds, resourceVersion precondition validation failed, validation not called, request fails",
			reactors: []*reactor{
				newReactor(
					"get",
					"namespaces",
					func(called int) (runtime.Object, error) {
						namespace := &corev1.Namespace{
							ObjectMeta: metav1.ObjectMeta{
								UID:             types.UID("one"),
								Name:            "test",
								ResourceVersion: "12347",
							},
						}
						return namespace, nil
					},
					1,
				),
			},
			objectValidator: &objectValidator{
				objectFunc: func(ctx context.Context, obj runtime.Object) error {
					return nil
				},
			},
			options: &metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{
					UID:             ptr.To(types.UID("one")),
					ResourceVersion: ptr.To("12345"),
				},
			},
			expectedStatus: &metav1.Status{Status: metav1.StatusFailure},
			expectedErr:    errors.New("validating preconditions: resourceVersion precondition \"12345\" does not match project resourceVersion \"12347\""),
		},
		{
			name: "no validation, delete successful, request succeeds, no retries happen",
			reactors: []*reactor{
				newReactor(
					"delete",
					"namespaces",
					func(called int) (runtime.Object, error) {
						return nil, nil
					},
					1,
				),
			},
			expectedStatus: &metav1.Status{Status: metav1.StatusSuccess},
		},
		{
			name: "no validation, delete has initial conflict, request fails",
			reactors: []*reactor{
				newReactor(
					"delete",
					"namespaces",
					func(called int) (runtime.Object, error) {
						if called < 1 {
							return nil, kerrors.NewConflict(corev1.Resource("namespaces"), "foo", errors.New("boom"))
						}
						return nil, nil
					},
					1,
				),
			},
			expectedStatus: &metav1.Status{Status: metav1.StatusFailure},
			expectedErr:    kerrors.NewConflict(corev1.Resource("namespaces"), "foo", errors.New("boom")),
		},
		{
			name: "no validation, delete has non-conflict failure, request not retried, request fails",
			reactors: []*reactor{
				newReactor("delete", "namespaces", func(called int) (runtime.Object, error) {
					return nil, errors.New("boom")
				}, 1),
			},
			expectedStatus: &metav1.Status{Status: metav1.StatusFailure},
			expectedErr:    errors.New("boom"),
		},
		{
			name: "no validation, delete has persistent conflict failure, request not retried, request fails",
			reactors: []*reactor{
				newReactor("delete", "namespaces", func(called int) (runtime.Object, error) {
					return nil, kerrors.NewConflict(corev1.Resource("namespaces"), "foo", errors.New("boom"))
				}, 1),
			},
			expectedStatus: &metav1.Status{Status: metav1.StatusFailure},
			expectedErr:    kerrors.NewConflict(corev1.Resource("namespaces"), "foo", errors.New("boom")),
		},
		{
			name: "no validation, delete has persistent conflict failure, user supplied preconditions, no retry, request fails",
			reactors: []*reactor{
				newReactor("delete", "namespaces", func(called int) (runtime.Object, error) {
					return nil, kerrors.NewConflict(corev1.Resource("namespaces"), "foo", errors.New("boom"))
				}, 1),
			},
			options: &metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{
					UID:             ptr.To(types.UID("1234")),
					ResourceVersion: ptr.To("1234"),
				},
			},
			expectedStatus: &metav1.Status{Status: metav1.StatusFailure},
			expectedErr:    kerrors.NewConflict(corev1.Resource("namespaces"), "foo", errors.New("boom")),
		},
		{
			name: "validation, validation successful, delete has persistent conflict failure, user supplied preconditions, no retry, request fails",
			reactors: []*reactor{
				newReactor("get", "namespaces", func(called int) (runtime.Object, error) {
					namespace := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							UID:             types.UID("1234"),
							Name:            "test",
							ResourceVersion: "1234",
						},
					}
					return namespace, nil
				}, 1),
				newReactor("delete", "namespaces", func(called int) (runtime.Object, error) {
					return nil, kerrors.NewConflict(corev1.Resource("namespaces"), "foo", errors.New("boom"))
				}, 1),
			},
			objectValidator: &objectValidator{
				objectFunc: func(ctx context.Context, obj runtime.Object) error {
					return nil
				},
			},
			options: &metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{
					UID:             ptr.To(types.UID("1234")),
					ResourceVersion: ptr.To("1234"),
				},
			},
			expectedStatus:          &metav1.Status{Status: metav1.StatusFailure},
			expectedErr:             kerrors.NewConflict(corev1.Resource("namespaces"), "foo", errors.New("boom")),
			expectedValidationCalls: 1,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var validationFunc rest.ValidateObjectFunc
			if tc.objectValidator != nil {
				validationFunc = tc.objectValidator.ValidateObjectFunc()
			}

			client := &fake.Clientset{}
			if len(tc.reactors) > 0 {
				reactors := []ktesting.Reactor{}
				for _, reactor := range tc.reactors {
					reactors = append(reactors, reactor)
				}
				client.ReactionChain = reactors
			}

			storage := &REST{
				client: client.CoreV1().Namespaces(),
			}

			obj, _, err := storage.Delete(apirequest.NewContext(), "foo", validationFunc, tc.options)
			switch {
			case err != nil && tc.expectedErr == nil:
				t.Fatalf("received an unexpected error: %v", err)
			case err == nil && tc.expectedErr != nil:
				t.Fatalf("expected an error but did not receive one. expected error: %v", tc.expectedErr)
			case err != nil && tc.expectedErr != nil && err.Error() != tc.expectedErr.Error():
				t.Fatalf("received error does not match expected error. expected error: %v , received error: %v", tc.expectedErr, err)
			}

			if tc.expectedStatus.Status != obj.(*metav1.Status).Status {
				t.Fatalf("received an unexpected status. expected status: %v , received status: %v", tc.expectedStatus, obj.(*metav1.Status))
			}

			if tc.objectValidator != nil {
				if tc.objectValidator.called != tc.expectedValidationCalls {
					t.Fatalf("expected validation to be called %d times, but was called %d times", tc.expectedValidationCalls, tc.objectValidator.called)
				}
			}

			if len(tc.reactors) > 0 {
				for _, reactor := range tc.reactors {
					if reactor.called != reactor.expectedCalls {
						t.Fatalf("expected reaction for %q to be called %d times, but was called %d times", fmt.Sprintf("%s %s", reactor.verb, reactor.resource), reactor.expectedCalls, reactor.called)
					}
				}
			}
		})
	}
}
