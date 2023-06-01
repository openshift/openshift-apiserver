package etcd

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	kapirest "k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	routegroup "github.com/openshift/api/route"
	routev1 "github.com/openshift/api/route/v1"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
	routeregistry "github.com/openshift/openshift-apiserver/pkg/route/apiserver/registry/route"
	routeprinters "github.com/openshift/openshift-apiserver/pkg/route/printers/internalversion"
)

type REST struct {
	*registry.Store
}

var _ rest.StandardStorage = &REST{}
var _ rest.CategoriesProvider = &REST{}

// Categories implements the CategoriesProvider interface. Returns a list of categories a resource is part of.
func (r *REST) Categories() []string {
	return []string{"all"}
}

type HostnameGenerator interface {
	GenerateHostname(*routev1.Route) (string, error)
}

// NewREST returns a RESTStorage object that will work against routes.
func NewREST(optsGetter generic.RESTOptionsGetter, allocator HostnameGenerator, sarClient routeregistry.SubjectAccessReviewInterface) (*REST, *StatusREST, error) {
	strategy := routeregistry.NewStrategy(allocator, sarClient)

	store := &registry.Store{
		NewFunc:                   func() runtime.Object { return &routeapi.Route{} },
		NewListFunc:               func() runtime.Object { return &routeapi.RouteList{} },
		DefaultQualifiedResource:  routegroup.Resource("routes"),
		SingularQualifiedResource: routegroup.Resource("route"),

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(routeprinters.AddRouteOpenShiftHandlers)},

		CreateStrategy: strategy,
		UpdateStrategy: strategy,
		DeleteStrategy: strategy,
	}

	options := &generic.StoreOptions{
		RESTOptions: optsGetter,
		AttrFunc:    storage.AttrFunc(storage.DefaultNamespaceScopedAttr).WithFieldMutation(routeapi.RouteFieldSelector),
	}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, nil, err
	}

	statusStore := *store
	statusStore.UpdateStrategy = routeregistry.StatusStrategy

	return &REST{store}, &StatusREST{&statusStore}, nil
}

// StatusREST implements the REST endpoint for changing the status of a route.
type StatusREST struct {
	store *registry.Store
}

var _ kapirest.Patcher = &StatusREST{}
var _ rest.Storage = &StatusREST{}

// New creates a new route resource
func (r *StatusREST) New() runtime.Object {
	return &routeapi.Route{}
}

// Get retrieves the object from the storage. It is required to support Patch.
func (r *StatusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, options)
}

// Update alters the status subset of an object.
func (r *StatusREST) Update(ctx context.Context, name string, objInfo kapirest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return r.store.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
}

func (r *StatusREST) Destroy() {
	r.store.Destroy()
}

// LegacyREST allows us to wrap and alter some behavior
type LegacyREST struct {
	*REST
}

func (r *LegacyREST) Categories() []string {
	return []string{}
}
