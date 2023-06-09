package etcd

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	"github.com/openshift/api/template"
	templateapi "github.com/openshift/openshift-apiserver/pkg/template/apis/template"
	"github.com/openshift/openshift-apiserver/pkg/template/apiserver/registry/brokertemplateinstance"
	templateprinters "github.com/openshift/openshift-apiserver/pkg/template/printers/internalversion"
)

// REST implements a RESTStorage for brokertemplateinstances against etcd
type REST struct {
	*registry.Store
}

var _ rest.StandardStorage = &REST{}

// NewREST returns a RESTStorage object that will work against brokertemplateinstances.
func NewREST(optsGetter generic.RESTOptionsGetter) (*REST, error) {
	store := &registry.Store{
		NewFunc:                   func() runtime.Object { return &templateapi.BrokerTemplateInstance{} },
		NewListFunc:               func() runtime.Object { return &templateapi.BrokerTemplateInstanceList{} },
		DefaultQualifiedResource:  template.Resource("brokertemplateinstances"),
		SingularQualifiedResource: template.Resource("brokertemplateinstance"),

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(templateprinters.AddTemplateOpenShiftHandlers)},

		CreateStrategy: brokertemplateinstance.Strategy,
		UpdateStrategy: brokertemplateinstance.Strategy,
		DeleteStrategy: brokertemplateinstance.Strategy,
	}

	options := &generic.StoreOptions{RESTOptions: optsGetter}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, err
	}

	return &REST{store}, nil
}
