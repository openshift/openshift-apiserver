package etcd

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	"github.com/openshift/api/build"
	buildapi "github.com/openshift/openshift-apiserver/pkg/build/apis/build"
	"github.com/openshift/openshift-apiserver/pkg/build/apiserver/registry/buildconfig"
	buildprinters "github.com/openshift/openshift-apiserver/pkg/build/printers/internalversion"
)

type REST struct {
	*registry.Store
}

var _ rest.StandardStorage = &REST{}
var _ rest.ShortNamesProvider = &REST{}
var _ rest.CategoriesProvider = &REST{}

// Categories implements the CategoriesProvider interface. Returns a list of categories a resource is part of.
func (r *REST) Categories() []string {
	return []string{"all"}
}

// ShortNames implements the ShortNamesProvider interface. Returns a list of short names for a resource.
func (r *REST) ShortNames() []string {
	return []string{"bc"}
}

// NewREST returns a RESTStorage object that will work against BuildConfig.
func NewREST(optsGetter generic.RESTOptionsGetter) (*REST, error) {
	store := &registry.Store{
		NewFunc:                   func() runtime.Object { return &buildapi.BuildConfig{} },
		NewListFunc:               func() runtime.Object { return &buildapi.BuildConfigList{} },
		DefaultQualifiedResource:  build.Resource("buildconfigs"),
		SingularQualifiedResource: build.Resource("buildconfig"),

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(buildprinters.AddBuildOpenShiftHandlers)},

		CreateStrategy: buildconfig.GroupStrategy,
		UpdateStrategy: buildconfig.GroupStrategy,
		DeleteStrategy: buildconfig.GroupStrategy,
	}

	options := &generic.StoreOptions{RESTOptions: optsGetter}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, err
	}

	return &REST{store}, nil
}

// LegacyREST allows us to wrap and alter some behavior
type LegacyREST struct {
	*REST
}

func (r *LegacyREST) Categories() []string {
	return []string{}
}
