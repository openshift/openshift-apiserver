package etcd

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	"github.com/openshift/api/build"
	buildapi "github.com/openshift/openshift-apiserver/pkg/build/apis/build"
	buildregistry "github.com/openshift/openshift-apiserver/pkg/build/apiserver/registry/build"
	buildprinters "github.com/openshift/openshift-apiserver/pkg/build/printers/internalversion"
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

// NewREST returns a RESTStorage object that will work against Build objects.
func NewREST(optsGetter generic.RESTOptionsGetter) (*REST, *DetailsREST, error) {
	store := &registry.Store{
		NewFunc:                   func() runtime.Object { return &buildapi.Build{} },
		NewListFunc:               func() runtime.Object { return &buildapi.BuildList{} },
		DefaultQualifiedResource:  build.Resource("builds"),
		SingularQualifiedResource: build.Resource("build"),

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(buildprinters.AddBuildOpenShiftHandlers)},

		CreateStrategy: buildregistry.Strategy,
		UpdateStrategy: buildregistry.Strategy,
		DeleteStrategy: buildregistry.Strategy,
	}

	options := &generic.StoreOptions{
		RESTOptions: optsGetter,
		AttrFunc:    storage.AttrFunc(storage.DefaultNamespaceScopedAttr).WithFieldMutation(buildapi.BuildFieldSelector),
	}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, nil, err
	}

	detailsStore := *store
	detailsStore.UpdateStrategy = buildregistry.DetailsStrategy

	return &REST{store}, &DetailsREST{&detailsStore}, nil
}

type DetailsREST struct {
	store *registry.Store
}

var _ rest.Updater = &DetailsREST{}
var _ rest.Storage = &DetailsREST{}

// New returns an empty object that can be used with Update after request data has been put into it.
func (r *DetailsREST) New() runtime.Object {
	return r.store.New()
}

// Update finds a resource in the storage and updates it.
func (r *DetailsREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return r.store.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
}

func (r *DetailsREST) Destroy() {
	r.store.Destroy()
}

// LegacyREST allows us to wrap and alter some behavior
type LegacyREST struct {
	*REST
}

func (r *LegacyREST) Categories() []string {
	return []string{}
}
