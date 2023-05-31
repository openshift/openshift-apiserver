package proxy

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	"github.com/openshift/api/project"
	"github.com/openshift/apiserver-library-go/pkg/authorization/scope"
	"github.com/openshift/openshift-apiserver/pkg/api/apihelpers"
	authorizationapi "github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization"
	projectapi "github.com/openshift/openshift-apiserver/pkg/project/apis/project"
	projectregistry "github.com/openshift/openshift-apiserver/pkg/project/apiserver/registry/project"
	projectauth "github.com/openshift/openshift-apiserver/pkg/project/auth"
	projectcache "github.com/openshift/openshift-apiserver/pkg/project/cache"
	projectprinters "github.com/openshift/openshift-apiserver/pkg/project/printers/internalversion"
	projectutil "github.com/openshift/openshift-apiserver/pkg/project/util"
)

type REST struct {
	// client can modify Kubernetes namespaces
	client corev1client.NamespaceInterface
	// lister can enumerate project lists that enforce policy
	lister projectauth.Lister
	// Allows extended behavior during creation, required
	createStrategy rest.RESTCreateStrategy
	// Allows extended behavior during updates, required
	updateStrategy rest.RESTUpdateStrategy

	authCache    *projectauth.AuthorizationCache
	projectCache *projectcache.ProjectCache

	rest.TableConvertor
}

var _ rest.Lister = &REST{}
var _ rest.CreaterUpdater = &REST{}
var _ rest.GracefulDeleter = &REST{}
var _ rest.Watcher = &REST{}
var _ rest.Scoper = &REST{}
var _ rest.Storage = &REST{}
var _ rest.SingularNameProvider = &REST{}

// NewREST returns a RESTStorage object that will work against Project resources
func NewREST(client corev1client.NamespaceInterface, lister projectauth.Lister, authCache *projectauth.AuthorizationCache, projectCache *projectcache.ProjectCache) *REST {
	return &REST{
		client:         client,
		lister:         lister,
		createStrategy: projectregistry.Strategy,
		updateStrategy: projectregistry.Strategy,

		authCache:    authCache,
		projectCache: projectCache,

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(projectprinters.AddProjectOpenShiftHandlers)},
	}
}

// New returns a new Project
func (s *REST) New() runtime.Object {
	return &projectapi.Project{}
}

func (s *REST) Destroy() {}

// NewList returns a new ProjectList
func (*REST) NewList() runtime.Object {
	return &projectapi.ProjectList{}
}

func (s *REST) NamespaceScoped() bool {
	return false
}

func (s *REST) GetSingularName() string {
	return "project"
}

// List retrieves a list of Projects that match label.

func (s *REST) List(ctx context.Context, options *metainternal.ListOptions) (runtime.Object, error) {
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, kerrors.NewForbidden(project.Resource("project"), "", fmt.Errorf("unable to list projects without a user on the context"))
	}
	labelSelector, _ := apihelpers.InternalListOptionsToSelectors(options)
	namespaceList, err := s.lister.List(user, labelSelector)
	if err != nil {
		return nil, err
	}
	return projectutil.ConvertNamespaceList(namespaceList)
}

func (s *REST) Watch(ctx context.Context, options *metainternal.ListOptions) (watch.Interface, error) {
	if ctx == nil {
		return nil, fmt.Errorf("Context is nil")
	}
	userInfo, exists := apirequest.UserFrom(ctx)
	if !exists {
		return nil, fmt.Errorf("no user")
	}

	includeAllExistingProjects := (options != nil) && options.ResourceVersion == "0"

	allowedNamespaces, err := scope.ScopesToVisibleNamespaces(userInfo.GetExtra()[authorizationapi.ScopesKey], s.authCache.GetClusterRoleLister(), true)
	if err != nil {
		return nil, err
	}

	m := projectutil.MatchProject(apihelpers.InternalListOptionsToSelectors(options))
	watcher := projectauth.NewUserProjectWatcher(userInfo, allowedNamespaces, s.projectCache, s.authCache, includeAllExistingProjects, m)
	s.authCache.AddWatcher(watcher)

	go watcher.Watch()
	return watcher, nil
}

var _ = rest.Getter(&REST{})

// Get retrieves a Project by name
func (s *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	opts := metav1.GetOptions{}
	if options != nil {
		opts = *options
	}
	namespace, err := s.client.Get(ctx, name, opts)
	if err != nil {
		return nil, err
	}
	return projectutil.ConvertNamespaceFromExternal(namespace)
}

var _ = rest.Creater(&REST{})

// Create registers the given Project.
func (s *REST) Create(ctx context.Context, obj runtime.Object, creationValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	projectObj, ok := obj.(*projectapi.Project)
	if !ok {
		return nil, fmt.Errorf("not a project: %#v", obj)
	}
	rest.FillObjectMetaSystemFields(&projectObj.ObjectMeta)
	s.createStrategy.PrepareForCreate(ctx, obj)
	if errs := s.createStrategy.Validate(ctx, obj); len(errs) > 0 {
		return nil, kerrors.NewInvalid(project.Kind("Project"), projectObj.Name, errs)
	}
	if err := creationValidation(ctx, projectObj.DeepCopyObject()); err != nil {
		return nil, err
	}
	projectExternal, err := projectutil.ConvertProjectToExternal(projectObj)
	if err != nil {
		return nil, err
	}
	namespace, err := s.client.Create(ctx, projectExternal, *options)
	if err != nil {
		return nil, err
	}
	return projectutil.ConvertNamespaceFromExternal(namespace)
}

var _ = rest.Updater(&REST{})

func (s *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, creationValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	oldObj, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	obj, err := objInfo.UpdatedObject(ctx, oldObj)
	if err != nil {
		return nil, false, err
	}

	projectObj, ok := obj.(*projectapi.Project)
	if !ok {
		return nil, false, fmt.Errorf("not a project: %#v", obj)
	}

	s.updateStrategy.PrepareForUpdate(ctx, obj, oldObj)
	if errs := s.updateStrategy.ValidateUpdate(ctx, obj, oldObj); len(errs) > 0 {
		return nil, false, kerrors.NewInvalid(project.Kind("Project"), projectObj.Name, errs)
	}
	if err := updateValidation(ctx, obj.DeepCopyObject(), oldObj.DeepCopyObject()); err != nil {
		return nil, false, err
	}

	projectExternal, err := projectutil.ConvertProjectToExternal(projectObj)
	if err != nil {
		return nil, false, err
	}
	namespace, err := s.client.Update(ctx, projectExternal, *options)
	if err != nil {
		return nil, false, err
	}

	projectInternal, err := projectutil.ConvertNamespaceFromExternal(namespace)
	return projectInternal, false, err
}

var _ = rest.GracefulDeleter(&REST{})

// Delete deletes a Project specified by its name
func (s *REST) Delete(ctx context.Context, name string, objectFunc rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	var opts metav1.DeleteOptions
	if options != nil {
		opts = *options
	}
	return &metav1.Status{Status: metav1.StatusSuccess}, false, s.client.Delete(ctx, name, opts)
}
