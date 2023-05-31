package rolebinding

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	rbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	authorizationapi "github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization"
	utilregistry "github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/registry"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/util"
	authprinters "github.com/openshift/openshift-apiserver/pkg/authorization/printers/internalversion"
	authclient "github.com/openshift/openshift-apiserver/pkg/client/impersonatingclient"
)

type REST struct {
	privilegedClient restclient.Interface
	rest.TableConvertor
}

var _ rest.Lister = &REST{}
var _ rest.Getter = &REST{}
var _ rest.CreaterUpdater = &REST{}
var _ rest.GracefulDeleter = &REST{}
var _ rest.Scoper = &REST{}
var _ rest.Storage = &REST{}
var _ rest.SingularNameProvider = &REST{}

func NewREST(client restclient.Interface) utilregistry.NoWatchStorage {
	return utilregistry.WrapNoWatchStorageError(&REST{
		privilegedClient: client,
		TableConvertor:   printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(authprinters.AddAuthorizationOpenShiftHandler)},
	})
}

func (s *REST) New() runtime.Object {
	return &authorizationapi.RoleBinding{}
}

func (s *REST) Destroy() {}

func (s *REST) NewList() runtime.Object {
	return &authorizationapi.RoleBindingList{}
}

func (s *REST) NamespaceScoped() bool {
	return true
}

func (s *REST) GetSingularName() string {
	return "rolebinding"
}

func (s *REST) List(ctx context.Context, options *metainternal.ListOptions) (runtime.Object, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, err
	}

	optv1 := metav1.ListOptions{}
	if err := metainternal.Convert_internalversion_ListOptions_To_v1_ListOptions(options, &optv1, nil); err != nil {
		return nil, err
	}

	bindings, err := client.List(ctx, optv1)
	if err != nil {
		return nil, err
	}

	ret := &authorizationapi.RoleBindingList{ListMeta: bindings.ListMeta}
	for _, curr := range bindings.Items {
		role, err := util.RoleBindingFromRBAC(&curr)
		if err != nil {
			return nil, err
		}
		ret.Items = append(ret.Items, *role)
	}
	return ret, nil
}

func (s *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, err
	}

	ret, err := client.Get(ctx, name, *options)
	if err != nil {
		return nil, err
	}

	binding, err := util.RoleBindingFromRBAC(ret)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

func (s *REST) Delete(ctx context.Context, name string, objectFunc rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, false, err
	}

	if err := client.Delete(ctx, name, *options); err != nil {
		return nil, false, err
	}

	return &metav1.Status{Status: metav1.StatusSuccess}, true, nil
}

func (s *REST) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, err
	}

	rb := obj.(*authorizationapi.RoleBinding)

	// Default the namespace if it is not specified so conversion does not error
	// Normally this is done during the REST strategy but we avoid those here to keep the proxies simple
	if ns, ok := apirequest.NamespaceFrom(ctx); ok && len(ns) > 0 && len(rb.Namespace) == 0 && len(rb.RoleRef.Namespace) > 0 {
		deepcopiedObj := rb.DeepCopy()
		deepcopiedObj.Namespace = ns
		rb = deepcopiedObj
	}

	convertedObj, err := util.RoleBindingToRBAC(rb)
	if err != nil {
		return nil, err
	}

	ret, err := client.Create(ctx, convertedObj, *options)
	if err != nil {
		return nil, err
	}

	binding, err := util.RoleBindingFromRBAC(ret)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

func (s *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, _ rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, false, err
	}

	old, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	oldRoleBinding, err := util.RoleBindingFromRBAC(old)
	if err != nil {
		return nil, false, err
	}

	obj, err := objInfo.UpdatedObject(ctx, oldRoleBinding)
	if err != nil {
		return nil, false, err
	}

	updatedRoleBinding, err := util.RoleBindingToRBAC(obj.(*authorizationapi.RoleBinding))
	if err != nil {
		return nil, false, err
	}

	ret, err := client.Update(ctx, updatedRoleBinding, *options)
	if err != nil {
		return nil, false, err
	}

	role, err := util.RoleBindingFromRBAC(ret)
	if err != nil {
		return nil, false, err
	}
	return role, false, err
}

func (s *REST) getImpersonatingClient(ctx context.Context) (rbacv1.RoleBindingInterface, error) {
	namespace, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, apierrors.NewBadRequest("namespace parameter required")
	}
	rbacClient, err := authclient.NewImpersonatingRBACFromContext(ctx, s.privilegedClient)
	if err != nil {
		return nil, err
	}
	return rbacClient.RoleBindings(namespace), nil
}
