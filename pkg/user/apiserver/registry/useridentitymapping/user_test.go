package useridentitymapping

import (
	"context"

	kerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	userapi "github.com/openshift/api/user/v1"
	"github.com/openshift/client-go/user/clientset/versioned/typed/user/v1/fake"
)

type UserRegistry struct {
	// included to fill out the interface for testing
	*fake.FakeUsers

	GetErr   map[string]error
	GetUsers map[string]*userapi.User

	CreateErr  error
	CreateUser *userapi.User

	UpdateErr  map[string]error
	UpdateUser *userapi.User

	ListErr   error
	ListUsers *userapi.UserList

	Actions *[]Action
}

func (r *UserRegistry) Get(_ context.Context, name string, options metav1.GetOptions) (*userapi.User, error) {
	*r.Actions = append(*r.Actions, Action{"GetUser", name})
	if user, ok := r.GetUsers[name]; ok {
		return user, nil
	}
	if err, ok := r.GetErr[name]; ok {
		return nil, err
	}
	return nil, kerrs.NewNotFound(userapi.Resource("user"), name)
}

func (r *UserRegistry) Create(_ context.Context, u *userapi.User, _ metav1.CreateOptions) (*userapi.User, error) {
	*r.Actions = append(*r.Actions, Action{"CreateUser", u})
	if r.CreateUser == nil && r.CreateErr == nil {
		return u, nil
	}
	return r.CreateUser, r.CreateErr
}

func (r *UserRegistry) Update(_ context.Context, u *userapi.User, _ metav1.UpdateOptions) (*userapi.User, error) {
	*r.Actions = append(*r.Actions, Action{"UpdateUser", u})
	err, _ := r.UpdateErr[u.Name]
	if r.UpdateUser == nil && err == nil {
		return u, nil
	}
	return r.UpdateUser, err
}

func (r *UserRegistry) List(_ context.Context, options metav1.ListOptions) (*userapi.UserList, error) {
	*r.Actions = append(*r.Actions, Action{"ListUsers", options})
	if r.ListUsers == nil && r.ListErr == nil {
		return &userapi.UserList{}, nil
	}
	return r.ListUsers, r.ListErr
}
