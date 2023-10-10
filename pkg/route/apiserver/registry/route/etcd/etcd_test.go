package etcd

import (
	"context"
	"testing"

	authorizationapi "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistrytest "k8s.io/apiserver/pkg/registry/generic/testing"
	"k8s.io/apiserver/pkg/registry/rest"
	etcdtesting "k8s.io/apiserver/pkg/storage/etcd3/testing"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/api/legacyscheme"

	routev1 "github.com/openshift/api/route/v1"
	routehostassignment "github.com/openshift/library-go/pkg/route/hostassignment"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
	_ "github.com/openshift/openshift-apiserver/pkg/route/apis/route/install"
)

type testAllocator struct {
	Hostname string
	Err      error
	Generate bool
}

func (a *testAllocator) GenerateHostname(*routev1.Route) (string, error) {
	a.Generate = true
	return a.Hostname, a.Err
}

type testSAR struct {
	allow bool
	err   error
	sar   *authorizationapi.SubjectAccessReview
}

func (t *testSAR) Create(_ context.Context, subjectAccessReview *authorizationapi.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationapi.SubjectAccessReview, error) {
	t.sar = subjectAccessReview
	return &authorizationapi.SubjectAccessReview{
		Status: authorizationapi.SubjectAccessReviewStatus{
			Allowed: t.allow,
		},
	}, t.err
}

func newStorage(t *testing.T, allocator HostnameGenerator) (*REST, *etcdtesting.EtcdTestServer) {
	server, etcdStorage := etcdtesting.NewUnsecuredEtcd3TestClientServer(t)
	etcdStorage.Codec = legacyscheme.Codecs.LegacyCodec(schema.GroupVersion{Group: "route.openshift.io", Version: "v1"})
	etcdStorageConfigForRoutes := &storagebackend.ConfigForResource{Config: *etcdStorage, GroupResource: schema.GroupResource{Group: "route.openshift.io", Resource: "routes"}}
	restOptions := generic.RESTOptions{StorageConfig: etcdStorageConfigForRoutes, Decorator: generic.UndecoratedStorage, DeleteCollectionWorkers: 1, ResourcePrefix: "routes"}
	fake := fake.NewSimpleClientset(&corev1.SecretList{})
	storage, _, err := NewREST(restOptions, allocator, &testSAR{allow: true}, fake.CoreV1(), true)
	if err != nil {
		t.Fatal(err)
	}
	return storage, server
}

func validRoute() *routeapi.Route {
	return &routeapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: routeapi.RouteSpec{
			To: routeapi.RouteTargetReference{
				Name: "test",
				Kind: "Service",
			},
		},
	}
}

func TestCreate(t *testing.T) {
	storage, server := newStorage(t, nil)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)
	test.TestCreate(
		// valid
		validRoute(),
		// invalid
		&routeapi.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "_-a123-a_"},
		},
	)
}

func TestCreateWithAllocation(t *testing.T) {
	allocator := &testAllocator{Hostname: "bar"}
	storage, server := newStorage(t, allocator)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()

	obj, err := storage.Create(apirequest.NewDefaultContext(), validRoute(), rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create object: %v", err)
	}
	result := obj.(*routeapi.Route)
	if result.Spec.Host != "bar" {
		t.Fatalf("unexpected route: %#v", result)
	}
	if v, ok := result.Annotations[routehostassignment.HostGeneratedAnnotationKey]; !ok || v != "true" {
		t.Fatalf("unexpected route: %#v", result)
	}
	if !allocator.Generate {
		t.Fatalf("hostname generator not invoked: %#v", allocator)
	}
}

func TestUpdate(t *testing.T) {
	storage, server := newStorage(t, nil)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)

	test.TestUpdate(
		validRoute(),
		// valid update
		func(obj runtime.Object) runtime.Object {
			object := obj.(*routeapi.Route)
			if object.Annotations == nil {
				object.Annotations = map[string]string{}
			}
			object.Annotations["updated"] = "true"
			return object
		},
		// invalid update
		func(obj runtime.Object) runtime.Object {
			object := obj.(*routeapi.Route)
			object.Spec.Path = "invalid/path"
			return object
		},
	)
}

func TestList(t *testing.T) {
	storage, server := newStorage(t, nil)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)
	test.TestList(
		validRoute(),
	)
}

func TestGet(t *testing.T) {
	storage, server := newStorage(t, &testAllocator{})
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)
	test.TestGet(
		validRoute(),
	)
}

func TestDelete(t *testing.T) {
	storage, server := newStorage(t, nil)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)
	test.TestDelete(
		validRoute(),
	)
}

func TestWatch(t *testing.T) {
	storage, server := newStorage(t, nil)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)

	valid := validRoute()
	valid.Name = "foo"
	valid.Labels = map[string]string{"foo": "bar"}

	test.TestWatch(
		valid,
		// matching labels
		[]labels.Set{{"foo": "bar"}},
		// not matching labels
		[]labels.Set{{"foo": "baz"}},
		// matching fields
		[]fields.Set{
			{"metadata.name": "foo"},
		},
		// not matching fields
		[]fields.Set{
			{"metadata.name": "bar"},
		},
	)
}
