package rangeallocations

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	"github.com/openshift/api/security"

	securityapi "github.com/openshift/openshift-apiserver/pkg/security/apis/security"
	securityprinters "github.com/openshift/openshift-apiserver/pkg/security/printers/internalversion"
)

type REST struct {
	*genericregistry.Store
}

var _ rest.StandardStorage = &REST{}

func NewREST(optsGetter generic.RESTOptionsGetter) *REST {
	store := &genericregistry.Store{
		NewFunc:                   func() runtime.Object { return &securityapi.RangeAllocation{} },
		NewListFunc:               func() runtime.Object { return &securityapi.RangeAllocationList{} },
		DefaultQualifiedResource:  security.Resource("rangeallocations"),
		SingularQualifiedResource: security.Resource("rangeallocation"),

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(securityprinters.AddSecurityOpenShiftHandler)},

		CreateStrategy: strategyInstance,
		UpdateStrategy: strategyInstance,
		DeleteStrategy: strategyInstance,
	}
	options := &generic.StoreOptions{RESTOptions: optsGetter}
	if err := store.CompleteWithOptions(options); err != nil {
		panic(err) // TODO: Propagate error up
	}
	return &REST{store}

}
