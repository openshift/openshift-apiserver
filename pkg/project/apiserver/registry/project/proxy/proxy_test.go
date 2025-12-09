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
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authentication/user"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

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

func TestDeleteProject(t *testing.T) {
	mockClient := &fake.Clientset{}
	storage := REST{
		client: mockClient.CoreV1().Namespaces(),
	}
	obj, _, err := storage.Delete(apirequest.NewContext(), "foo", nil, &metav1.DeleteOptions{})
	if obj == nil {
		t.Error("Unexpected nil obj")
	}
	if err != nil {
		t.Errorf("Unexpected non-nil error: %#v", err)
	}
	status, ok := obj.(*metav1.Status)
	if !ok {
		t.Errorf("Expected status type, got: %#v", obj)
	}
	if status.Status != metav1.StatusSuccess {
		t.Errorf("Expected status=success, got: %#v", status)
	}
	if len(mockClient.Actions()) != 1 {
		t.Errorf("Expected client action for delete, got %v", mockClient.Actions())
	}
	if !mockClient.Actions()[0].Matches("delete", "namespaces") {
		t.Errorf("Expected call to delete-namespace, got %#v", mockClient.Actions()[0])
	}
}

func TestDeleteProjectValidation(t *testing.T) {
	mockClient := &fake.Clientset{}
	storage := REST{
		client: mockClient.CoreV1().Namespaces(),
	}
	validationCalled := false
	validationFunc := func(ctx context.Context, obj runtime.Object) error {
		validationCalled = true
		return nil
	}

	storage.Delete(apirequest.NewContext(), "foo", validationFunc, &metav1.DeleteOptions{})
	if !validationCalled {
		t.Errorf("Expected validation function to be called")
	}
}

func TestDeleteProjectValidationRetries(t *testing.T) {
	mockClient := &fake.Clientset{}
	storage := REST{
		client: mockClient.CoreV1().Namespaces(),
	}
	maxRetries := 3
	validationRetries := 0
	validationFunc := func(ctx context.Context, obj runtime.Object) error {
		if validationRetries < maxRetries {
			validationRetries++
			return fmt.Errorf("not there yet")
		}
		return nil
	}
	obj, _, err := storage.Delete(apirequest.NewContext(), "foo", validationFunc, &metav1.DeleteOptions{})
	if obj == nil {
		t.Error("Unexpected nil obj")
	}
	if err != nil {
		t.Errorf("Unexpected non-nil error: %#v", err)
	}
	status, ok := obj.(*metav1.Status)
	if !ok {
		t.Errorf("Expected status type, got: %#v", obj)
	}
	if status.Status != metav1.StatusSuccess {
		t.Errorf("Expected status=success, got: %#v", status)
	}
	if len(mockClient.Actions()) != maxRetries+2 {
		t.Errorf("Expected client action for get, got %v", mockClient.Actions())
	}
	for i := range len(mockClient.Actions()) - 1 {
		if !mockClient.Actions()[i].Matches("get", "namespaces") {
			t.Errorf("Expected call #%d to get-namespace, got %#v", i, mockClient.Actions()[i])
		}
	}
	if !mockClient.Actions()[len(mockClient.Actions())-1].Matches("delete", "namespaces") {
		t.Errorf("Expected call #%d to delete-namespace, got %#v", len(mockClient.Actions())-1, mockClient.Actions()[len(mockClient.Actions())-1])
	}
	if validationRetries != maxRetries {
		t.Errorf("Expected validation function to be retried %d times, got %d", maxRetries, validationRetries)
	}
}

func TestDeleteProjectValidationError(t *testing.T) {
	mockClient := &fake.Clientset{}
	storage := REST{
		client: mockClient.CoreV1().Namespaces(),
	}
	validationError := fmt.Errorf("faulty function")

	validationFunc := func(ctx context.Context, obj runtime.Object) error {
		return validationError
	}
	obj, _, err := storage.Delete(apirequest.NewContext(), "foo", validationFunc, &metav1.DeleteOptions{})
	if obj == nil {
		t.Error("Unexpected nil obj")
	}
	status, ok := obj.(*metav1.Status)
	if !ok {
		t.Errorf("Expected status type, got: %#v", obj)
	}
	if status.Status != metav1.StatusFailure {
		t.Errorf("Expected status=failure, got: %#v", status)
	}
	if err == nil {
		t.Errorf("Expected error but got nil")
	}
	if errors.Unwrap(err) != validationError {
		t.Errorf("Unexpected error: %#v", errors.Unwrap(err))
	}
}

func TestDeleteProjectValidationPreconditionUID(t *testing.T) {
	uid := ktypes.UID("first-uid")
	resourceVersion := "10"
	meta := metav1.ObjectMeta{Name: "foo", UID: uid, ResourceVersion: resourceVersion}
	namespaceList := corev1.NamespaceList{
		Items: []corev1.Namespace{
			{
				ObjectMeta: meta,
			},
		},
	}

	tests := []struct {
		testName             string
		uid, resourceVersion string
	}{
		{
			testName: "all unset",
		},
		{
			testName: "uid set",
			uid:      "first-uid",
		},
		{
			testName:        "resourceVersion set",
			resourceVersion: "10",
		},
		{
			testName:        "both set",
			uid:             "first-uid",
			resourceVersion: "10",
		},
	}
	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			mockClient := fake.NewSimpleClientset(&namespaceList)
			storage := REST{
				client: mockClient.CoreV1().Namespaces(),
			}
			validationFunc := func(ctx context.Context, obj runtime.Object) error {
				return nil
			}

			expectedUID := uid
			expectedRV := resourceVersion
			deleteOpts := &metav1.DeleteOptions{}
			if len(test.uid) > 0 {
				expectedUID = ktypes.UID(test.uid)
				if deleteOpts.Preconditions == nil {
					deleteOpts.Preconditions = &metav1.Preconditions{}
				}
				deleteOpts.Preconditions.UID = &expectedUID
			}
			if len(test.resourceVersion) > 0 {
				expectedRV = test.resourceVersion
				if deleteOpts.Preconditions == nil {
					deleteOpts.Preconditions = &metav1.Preconditions{}
				}
				deleteOpts.Preconditions.ResourceVersion = &expectedRV
			}

			storage.Delete(apirequest.NewContext(), "foo", validationFunc, deleteOpts)
			if len(mockClient.Actions()) != 2 {
				t.Errorf("Expected client action for get and delete, got %v", mockClient.Actions())
			}
			if !mockClient.Actions()[0].Matches("get", "namespaces") {
				t.Errorf("Expected call to get-namespace, got %#v", mockClient.Actions()[0])
			}
			lastAction := mockClient.Actions()[1]
			if !lastAction.Matches("delete", "namespaces") {
				t.Errorf("Expected call to delete-namespace, got %#v", mockClient.Actions()[1])
			}
			deleteAction := lastAction.(ktesting.DeleteActionImpl)
			preconditions := deleteAction.DeleteOptions.Preconditions
			if preconditions == nil {
				t.Fatalf("Expected DeleteOptions precondition to be non-nil")
			}
			if preconditions.UID == nil {
				t.Fatalf("Expected DeleteOptions precondition UID to be non-nil")
			}
			if *preconditions.UID != expectedUID {
				t.Errorf("Expected DeleteOptions precondition UID to %#v, got %#v", expectedUID, *preconditions.UID)
			}
			if preconditions.ResourceVersion == nil {
				t.Fatalf("Expected DeleteOptions precondition ResourceVersion to be non-nil")
			}
			if *preconditions.ResourceVersion != expectedRV {
				t.Errorf("Expected DeleteOptions precondition ResourceVersion to %#v, got %#v", expectedRV, *preconditions.ResourceVersion)
			}
		})
	}
}
