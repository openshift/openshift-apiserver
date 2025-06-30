package auth

import (
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	corev1informers "k8s.io/client-go/informers/core/v1"
	rbacv1informers "k8s.io/client-go/informers/rbac/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	globalSyncKey      = "global"
	namespaceKeyPrefix = "namespace:"
)

// ReactiveAuthorizationCacheV2 extends the base AuthorizationCacheV2 with event-driven
// updates, allowing the cache to respond to RBAC changes in real-time instead of only
// during periodic synchronization.
type ReactiveAuthorizationCacheV2 struct {
	*AuthorizationCacheV2

	// Workqueue for processing sync operations
	queue   workqueue.TypedRateLimitingInterface[string]
	workers int
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// ReactiveAuthorizationCacheV2Config provides workqueue configuration options for the reactive cache
type ReactiveAuthorizationCacheV2Config struct {
	Workers     int
	RateLimiter workqueue.TypedRateLimiter[string]
	QueueName   string
}

// NewReactiveAuthorizationCacheV2 creates a new reactive authorization cache
func NewReactiveAuthorizationCacheV2(
	namespaceLister corev1listers.NamespaceLister,
	reviewer Reviewer,
	informers rbacv1informers.Interface,
	namespaceInformer corev1informers.NamespaceInformer,
) (*ReactiveAuthorizationCacheV2, error) {
	return NewReactiveAuthorizationCacheV2WithConfig(
		namespaceLister,
		reviewer,
		informers,
		namespaceInformer,
		DefaultReactiveAuthorizationCacheV2Config(),
	)
}

// DefaultReactiveAuthorizationCacheV2Config returns sensible defaults
func DefaultReactiveAuthorizationCacheV2Config() *ReactiveAuthorizationCacheV2Config {
	return &ReactiveAuthorizationCacheV2Config{
		Workers:     5,
		RateLimiter: workqueue.DefaultTypedControllerRateLimiter[string](),
		QueueName:   "reactive_authorization_cache_v2",
	}
}

// NewReactiveAuthorizationCacheV2WithConfig creates a reactive cache with custom configuration
func NewReactiveAuthorizationCacheV2WithConfig(
	namespaceLister corev1listers.NamespaceLister,
	reviewer Reviewer,
	informers rbacv1informers.Interface,
	namespaceInformer corev1informers.NamespaceInformer,
	config *ReactiveAuthorizationCacheV2Config,
) (*ReactiveAuthorizationCacheV2, error) {

	if config == nil {
		config = DefaultReactiveAuthorizationCacheV2Config()
	}

	baseCache, err := NewAuthorizationCacheV2(namespaceLister, reviewer, informers)
	if err != nil {
		return nil, fmt.Errorf("failed to create base authorization cache: %v", err)
	}

	rac := &ReactiveAuthorizationCacheV2{
		AuthorizationCacheV2: baseCache,
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			config.RateLimiter,
			workqueue.TypedRateLimitingQueueConfig[string]{
				Name: config.QueueName,
			},
		),
		workers: config.Workers,
		stopCh:  make(chan struct{}),
	}

	// Set up event handlers
	if err := rac.setupEventHandlers(informers, namespaceInformer); err != nil {
		return nil, fmt.Errorf("failed to setup event handlers: %v", err)
	}

	return rac, nil
}

// setupEventHandlers configures informer event handlers for reactive updates
func (rac *ReactiveAuthorizationCacheV2) setupEventHandlers(
	informers rbacv1informers.Interface,
	namespaceInformer corev1informers.NamespaceInformer,
) error {
	// Cluster role events - trigger global sync
	informers.ClusterRoles().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { rac.queue.Add(globalSyncKey) },
		UpdateFunc: func(oldObj, newObj any) { rac.queue.Add(globalSyncKey) },
		DeleteFunc: func(obj any) { rac.queue.Add(globalSyncKey) },
	})

	// Cluster role binding events - trigger global sync
	informers.ClusterRoleBindings().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { rac.queue.Add(globalSyncKey) },
		UpdateFunc: func(oldObj, newObj any) { rac.queue.Add(globalSyncKey) },
		DeleteFunc: func(obj any) { rac.queue.Add(globalSyncKey) },
	})

	// Role events - trigger namespace sync
	informers.Roles().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if role, ok := obj.(*rbacv1.Role); ok {
				rac.queue.Add(namespaceKeyPrefix + role.Namespace)
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			if role, ok := newObj.(*rbacv1.Role); ok {
				rac.queue.Add(namespaceKeyPrefix + role.Namespace)
			}
		},
		DeleteFunc: func(obj any) {
			if missedDeletionObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = missedDeletionObj.Obj
			}
			if role, ok := obj.(*rbacv1.Role); ok {
				rac.queue.Add(namespaceKeyPrefix + role.Namespace)
			}
		},
	})

	// Role binding events - trigger namespace sync
	informers.RoleBindings().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if rb, ok := obj.(*rbacv1.RoleBinding); ok {
				rac.queue.Add(namespaceKeyPrefix + rb.Namespace)
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			if rb, ok := newObj.(*rbacv1.RoleBinding); ok {
				rac.queue.Add(namespaceKeyPrefix + rb.Namespace)
			}
		},
		DeleteFunc: func(obj any) {
			if missedDeletionObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = missedDeletionObj.Obj
			}
			if rb, ok := obj.(*rbacv1.RoleBinding); ok {
				rac.queue.Add(namespaceKeyPrefix + rb.Namespace)
			}
		},
	})

	// Namespace events - trigger namespace sync
	namespaceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if ns, ok := obj.(*corev1.Namespace); ok {
				rac.queue.Add(namespaceKeyPrefix + ns.Name)
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			if ns, ok := newObj.(*corev1.Namespace); ok {
				rac.queue.Add(namespaceKeyPrefix + ns.Name)
			}
		},
		DeleteFunc: func(obj any) {
			if missedDeletionObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = missedDeletionObj.Obj
			}
			if ns, ok := obj.(*corev1.Namespace); ok {
				rac.queue.Add(namespaceKeyPrefix + ns.Name)
			}
		},
	})

	return nil
}

// Run starts both the reactive event processing and the periodic synchronization
func (rac *ReactiveAuthorizationCacheV2) Run(period time.Duration) {
	// Start queue workers
	for i := 0; i < rac.workers; i++ {
		rac.wg.Add(1)
		go rac.worker()
	}

	// Start periodic synchronization
	go func() {
		// Initial sync
		rac.synchronize()

		ticker := time.NewTicker(period)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rac.synchronize()
			case <-rac.stopCh:
				return
			}
		}
	}()
}

// Stop stops both reactive processing and periodic synchronization
func (rac *ReactiveAuthorizationCacheV2) Stop() {
	select {
	case <-rac.stopCh:
		// Already stopped
		return
	default:
		close(rac.stopCh)
	}

	rac.queue.ShutDown()

	rac.wg.Wait()
}

// worker processes items from the queue
func (rac *ReactiveAuthorizationCacheV2) worker() {
	defer rac.wg.Done()

	for rac.processNextWorkItem() {
	}
}

// processNextWorkItem processes a single work item from the queue
func (rac *ReactiveAuthorizationCacheV2) processNextWorkItem() bool {
	key, quit := rac.queue.Get()
	if quit {
		return false
	}
	defer rac.queue.Done(key)

	err := rac.syncHandler(key)
	if err == nil {
		rac.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error syncing %q: %v", key, err))

	if rac.queue.NumRequeues(key) < 5 {
		// Requeue with rate limiting
		rac.queue.AddRateLimited(key)
		return true
	}

	// Give up after too many retries
	rac.queue.Forget(key)
	utilruntime.HandleError(fmt.Errorf("dropping %q out of the queue: %v", key, err))

	return true
}

// syncHandler processes a single sync operation
func (rac *ReactiveAuthorizationCacheV2) syncHandler(key string) error {
	if key == globalSyncKey {
		// Global sync
		return rac.synchronize()
	} else if strings.HasPrefix(key, namespaceKeyPrefix) {
		// Namespace sync
		return rac.refreshNamespace(strings.TrimPrefix(key, namespaceKeyPrefix))
	}

	return fmt.Errorf("unknown sync key: %s", key)
}

// GetQueueStatus returns information about the queue
func (rac *ReactiveAuthorizationCacheV2) GetQueueStatus() (queued int) {
	return rac.queue.Len()
}
