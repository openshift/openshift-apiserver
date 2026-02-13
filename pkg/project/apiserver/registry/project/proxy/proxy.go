package proxy

import (
	"context"
	"fmt"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
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

var (
	_ rest.Lister               = &REST{}
	_ rest.CreaterUpdater       = &REST{}
	_ rest.GracefulDeleter      = &REST{}
	_ rest.Watcher              = &REST{}
	_ rest.Scoper               = &REST{}
	_ rest.Storage              = &REST{}
	_ rest.SingularNameProvider = &REST{}
)

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

	// If there is no admission validation to run against this operation, we can safely
	// run the delete call using the user provided information and delete options
	// and return the result.
	if objectFunc == nil {
		err := s.client.Delete(ctx, name, opts)
		if err != nil {
			return &metav1.Status{Status: metav1.StatusFailure}, false, err
		}
		return &metav1.Status{Status: metav1.StatusSuccess}, false, nil
	}

	// If there is admission validation to run against this operation,
	// we need to first perform a GET for the Project being deleted.
	// This allows us to have a concrete instance of the Project
	// to validate.
	//
	// If the user has provided specific delete preconditions,
	// we should validate that they are correct before performing validation
	// and attempting the delete request. If the delete request fails
	// with user provided delete preconditions, we should not retry.
	//
	// If the user has *not* provided delete preconditions,
	// we build them based on the metadata of the fetched Project
	// that has passed the validation check to ensure we only
	// attempt to delete the resource that we ran validation on.
	// If we encounter conflict errors during a delete attempt, we
	// should retry with a fresh fetch of the Project.

	var lastErr error
	backoff := wait.Backoff{
		Steps:    10,
		Duration: time.Millisecond,
		Cap:      time.Second,
		Factor:   2,
	}
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		project, err := s.getProjectForDeletion(ctx, name, &opts)
		if err != nil {
			lastErr = fmt.Errorf("getting project for deletion: %w", err)
			return false, nil
		}

		// Do not retry if the deletion preconditions are invalid because
		// the request will always fail.
		err = validateDeletePreconditionsForProject(project, &opts)
		if err != nil {
			return false, fmt.Errorf("validating preconditions: %w", err)
		}

		if err := objectFunc(ctx, project); err != nil {
			lastErr = fmt.Errorf("validating project: %w", err)
			return false, nil
		}

		// If the user did not supply any preconditions, set them to the Project
		// we fetched for validation to ensure we are only deleting the Project
		// that we validated.
		// If the user *did* supply preconditions, we already inherited them and
		// should continue using them.
		deleteOpts := opts
		userProvidedPreconditions := (options != nil && options.Preconditions != nil)
		if !userProvidedPreconditions {
			deleteOpts.Preconditions = &metav1.Preconditions{
				UID:             &project.UID,
				ResourceVersion: &project.ResourceVersion,
			}
		}

		err = s.client.Delete(ctx, name, deleteOpts)
		switch {
		case err == nil:
			return true, nil
		case kerrors.IsConflict(err) && !userProvidedPreconditions:
			lastErr = err
			return false, nil
		default:
			return false, err
		}
	})
	if err != nil {
		if wait.Interrupted(err) && lastErr != nil {
			err = lastErr
		}
		return &metav1.Status{Status: metav1.StatusFailure}, false, err
	}
	return &metav1.Status{Status: metav1.StatusSuccess}, false, nil
}

func (s *REST) getProjectForDeletion(ctx context.Context, name string, deleteOptions *metav1.DeleteOptions) (*projectapi.Project, error) {
	getOpts := metav1.GetOptions{}

	obj, err := s.Get(ctx, name, &getOpts)
	if err != nil {
		return nil, fmt.Errorf("unable to get project: %w", err)
	}

	projectObj, ok := obj.(*projectapi.Project)
	if !ok || projectObj == nil {
		return nil, fmt.Errorf("not a project: %#v", obj)
	}

	return projectObj, nil
}

func validateDeletePreconditionsForProject(project *projectapi.Project, deleteOptions *metav1.DeleteOptions) error {
	if deleteOptions == nil || deleteOptions.Preconditions == nil {
		return nil
	}

	if deleteOptions.Preconditions.UID != nil && project.UID != *deleteOptions.Preconditions.UID {
		return fmt.Errorf("uid precondition %q does not match project uid %q", *deleteOptions.Preconditions.UID, project.UID)
	}

	if deleteOptions.Preconditions.ResourceVersion != nil && project.ResourceVersion != *deleteOptions.Preconditions.ResourceVersion {
		return fmt.Errorf("resourceVersion precondition %q does not match project resourceVersion %q", *deleteOptions.Preconditions.ResourceVersion, project.ResourceVersion)
	}

	return nil
}
