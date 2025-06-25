package auth

import (
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	corev1informers "k8s.io/client-go/informers/core/v1"
	rbacv1informers "k8s.io/client-go/informers/rbac/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// ReactiveAuthorizationCacheV2 extends the base AuthorizationCacheV2 with event-driven
// updates, allowing the cache to respond to RBAC changes in real-time instead of only
// during periodic synchronization.
type ReactiveAuthorizationCacheV2 struct {
	*AuthorizationCacheV2

	// Event handling
	eventQueue   chan eventItem
	eventWorkers int
	stopCh       chan struct{}
	wg           sync.WaitGroup

	// Debouncing for rapid fire events
	debounceTimer       *time.Timer
	debounceDuration    time.Duration
	pendingNamespaces   map[string]struct{}
	debounceLock        sync.Mutex
	isGlobalSyncPending bool // Prevent namespace refresh from replacing a global sync during the debounce period
}

// eventItem represents a change event that needs processing
type eventItem struct {
	eventType EventType
	object    runtime.Object
	namespace string
}

// EventType represents the type of RBAC change event
type EventType int

const (
	EventTypeClusterRoleChanged EventType = iota
	EventTypeClusterRoleBindingChanged
	EventTypeRoleChanged
	EventTypeRoleBindingChanged
	EventTypeNamespaceChanged
)

// ReactiveAuthorizationCacheV2Config provides configuration options for the reactive cache
type ReactiveAuthorizationCacheV2Config struct {
	// DebounceDuration controls how long to wait before processing batched events
	DebounceDuration time.Duration
	// EventQueueSize controls the size of the event processing queue
	EventQueueSize int
	// EventWorkers controls the number of concurrent event processing workers
	EventWorkers int
}

// NewReactiveAuthorizationCacheV2 creates a new reactive authorization cache
func NewReactiveAuthorizationCacheV2(
	namespaceLister corev1listers.NamespaceLister,
	reviewer Reviewer,
	informers rbacv1informers.Interface,
	namespaceInformer corev1informers.NamespaceInformer,
) (*ReactiveAuthorizationCacheV2, error) {
	rac, err := NewReactiveAuthorizationCacheV2WithConfig(
		namespaceLister,
		reviewer,
		informers,
		namespaceInformer,
		DefaultReactiveAuthorizationCacheV2Config(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create reactive authorization cache: %v", err)
	}

	if err := rac.setupEventHandlers(informers, namespaceInformer); err != nil {
		return nil, fmt.Errorf("failed to setup event handlers: %v", err)
	}

	return rac, nil
}

// DefaultReactiveAuthorizationCacheV2Config returns sensible defaults
func DefaultReactiveAuthorizationCacheV2Config() *ReactiveAuthorizationCacheV2Config {
	return &ReactiveAuthorizationCacheV2Config{
		DebounceDuration: 500 * time.Millisecond,
		EventQueueSize:   1000,
		EventWorkers:     5,
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
		eventQueue:           make(chan eventItem, config.EventQueueSize),
		eventWorkers:         config.EventWorkers,
		stopCh:               make(chan struct{}),
		debounceDuration:     config.DebounceDuration,
		pendingNamespaces:    make(map[string]struct{}),
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

	// Cluster role events
	informers.ClusterRoles().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			rac.enqueueEvent(EventTypeClusterRoleChanged, obj, "")
		},
		UpdateFunc: func(oldObj, newObj any) {
			rac.enqueueEvent(EventTypeClusterRoleChanged, newObj, "")
		},
		DeleteFunc: func(obj any) {
			if missedDeletionObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = missedDeletionObj.Obj
			}
			rac.enqueueEvent(EventTypeClusterRoleChanged, obj, "")
		},
	})

	// Cluster role binding events
	informers.ClusterRoleBindings().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			rac.enqueueEvent(EventTypeClusterRoleBindingChanged, obj, "")
		},
		UpdateFunc: func(oldObj, newObj any) {
			rac.enqueueEvent(EventTypeClusterRoleBindingChanged, newObj, "")
		},
		DeleteFunc: func(obj any) {
			if missedDeletionObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = missedDeletionObj.Obj
			}
			rac.enqueueEvent(EventTypeClusterRoleBindingChanged, obj, "")
		},
	})

	// The following resources require type assertion so that we can safely grab the namespace

	// Role events
	informers.Roles().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if role, ok := obj.(*rbacv1.Role); ok {
				rac.enqueueEvent(EventTypeRoleChanged, obj, role.Namespace)
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			if role, ok := newObj.(*rbacv1.Role); ok {
				rac.enqueueEvent(EventTypeRoleChanged, newObj, role.Namespace)
			}
		},
		DeleteFunc: func(obj any) {
			if missedDeletionObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = missedDeletionObj.Obj
			}
			if role, ok := obj.(*rbacv1.Role); ok {
				rac.enqueueEvent(EventTypeRoleChanged, obj, role.Namespace)
			}
		},
	})

	// Role binding events
	informers.RoleBindings().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if rb, ok := obj.(*rbacv1.RoleBinding); ok {
				rac.enqueueEvent(EventTypeRoleBindingChanged, obj, rb.Namespace)
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			if rb, ok := newObj.(*rbacv1.RoleBinding); ok {
				rac.enqueueEvent(EventTypeRoleBindingChanged, newObj, rb.Namespace)
			}
		},
		DeleteFunc: func(obj any) {
			if missedDeletionObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = missedDeletionObj.Obj
			}
			if rb, ok := obj.(*rbacv1.RoleBinding); ok {
				rac.enqueueEvent(EventTypeRoleBindingChanged, obj, rb.Namespace)
			}
		},
	})

	// Namespace events
	namespaceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if ns, ok := obj.(*corev1.Namespace); ok {
				rac.enqueueEvent(EventTypeNamespaceChanged, obj, ns.Name)
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			if ns, ok := newObj.(*corev1.Namespace); ok {
				rac.enqueueEvent(EventTypeNamespaceChanged, newObj, ns.Name)
			}
		},
		DeleteFunc: func(obj any) {
			if missedDeletionObj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = missedDeletionObj.Obj
			}
			if ns, ok := obj.(*corev1.Namespace); ok {
				rac.enqueueEvent(EventTypeNamespaceChanged, obj, ns.Name)
			}
		},
	})

	return nil
}

// enqueueEvent adds an event to the processing queue
func (rac *ReactiveAuthorizationCacheV2) enqueueEvent(eventType EventType, obj any, namespace string) {
	runtimeObj, ok := obj.(runtime.Object)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("expected runtime.Object but got %T", obj))
		return
	}

	event := eventItem{
		eventType: eventType,
		object:    runtimeObj,
		namespace: namespace,
	}

	select {
	case rac.eventQueue <- event:
		// Event queued successfully
	default:
		// Queue is full, log warning but don't block
		utilruntime.HandleError(fmt.Errorf("event queue is full, dropping event for %s", rac.getObjectName(runtimeObj)))
	}
}

// getObjectName extracts the name from a runtime object for logging
func (rac *ReactiveAuthorizationCacheV2) getObjectName(obj runtime.Object) string {
	if accessor, err := meta.Accessor(obj); err == nil {
		return accessor.GetName()
	}
	return "unknown"
}

// StartReactive begins the reactive event processing
func (rac *ReactiveAuthorizationCacheV2) StartReactive() {
	for i := 0; i < rac.eventWorkers; i++ {
		rac.wg.Add(1)
		go rac.eventWorker()
	}
}

// StopReactive stops the reactive event processing
func (rac *ReactiveAuthorizationCacheV2) StopReactive() {
	select {
	case <-rac.stopCh:
		// Already stopped
		return
	default:
		close(rac.stopCh)
	}

	rac.wg.Wait()

	// Stop any pending debounce timer
	rac.debounceLock.Lock()
	if rac.debounceTimer != nil {
		rac.debounceTimer.Stop()
		rac.debounceTimer = nil
	}
	rac.debounceLock.Unlock()
}

// eventWorker processes events from the queue
func (rac *ReactiveAuthorizationCacheV2) eventWorker() {
	defer rac.wg.Done()

	for {
		select {
		case event := <-rac.eventQueue:
			switch event.eventType {
			case EventTypeClusterRoleChanged, EventTypeClusterRoleBindingChanged:
				rac.debounceGlobalRefresh()

			case EventTypeRoleChanged, EventTypeRoleBindingChanged:
				if event.namespace == "" {
					utilruntime.HandleError(fmt.Errorf("received namespace RBAC change with empty namespace"))
					return
				}

				rac.debounceNamespaceRefresh(event.namespace)

			case EventTypeNamespaceChanged:
				if event.namespace == "" {
					utilruntime.HandleError(fmt.Errorf("received namespace lifecycle change with empty namespace"))
					return
				}

				// For namespace events, refresh immediately
				go func() {
					if err := rac.refreshNamespace(event.namespace); err != nil {
						utilruntime.HandleError(fmt.Errorf("failed to refresh namespace %s after lifecycle change: %v", event.namespace, err))
					}
				}()
			}
		case <-rac.stopCh:
			return
		}
	}
}

// debounceGlobalRefresh implements debouncing for global RBAC changes
func (rac *ReactiveAuthorizationCacheV2) debounceGlobalRefresh() {
	rac.debounceLock.Lock()
	defer rac.debounceLock.Unlock()

	if rac.debounceTimer != nil {
		rac.debounceTimer.Stop()
	}

	rac.isGlobalSyncPending = true
	rac.debounceTimer = time.AfterFunc(rac.debounceDuration, func() {
		rac.debounceLock.Lock()
		rac.isGlobalSyncPending = false
		rac.debounceLock.Unlock()
		go rac.synchronize()
	})
}

// debounceNamespaceRefresh implements debouncing for namespace-specific changes
func (rac *ReactiveAuthorizationCacheV2) debounceNamespaceRefresh(namespace string) {
	rac.debounceLock.Lock()
	defer rac.debounceLock.Unlock()

	// If a more comprehensive global sync is already pending debounce, do not replace it with a namespace refresh
	if rac.isGlobalSyncPending {
		return
	}

	rac.pendingNamespaces[namespace] = struct{}{}

	if rac.debounceTimer != nil {
		rac.debounceTimer.Stop()
	}

	rac.debounceTimer = time.AfterFunc(rac.debounceDuration, func() {
		rac.processPendingNamespaces()
	})
}

// processPendingNamespaces processes all namespaces that have pending changes
func (rac *ReactiveAuthorizationCacheV2) processPendingNamespaces() {
	rac.debounceLock.Lock()
	namespacesToProcess := make([]string, 0, len(rac.pendingNamespaces))
	for ns := range rac.pendingNamespaces {
		namespacesToProcess = append(namespacesToProcess, ns)
	}
	rac.pendingNamespaces = make(map[string]struct{})
	rac.debounceLock.Unlock()

	globalHash, err := rac.computeGlobalRBACHash()
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to compute global RBAC hash: %v", err))
		return
	}

	for _, ns := range namespacesToProcess {
		nsHash, err := rac.computeNamespaceHashWithGlobal(ns, globalHash)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to compute hash for namespace %s: %v", ns, err))
			continue
		}
		if err := rac.refreshNamespaceWithHash(ns, nsHash); err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to refresh namespace %s: %v", ns, err))
		}
	}
}

// Run starts both the reactive event processing and the periodic synchronization
func (rac *ReactiveAuthorizationCacheV2) Run(period time.Duration) {
	// Start reactive processing
	rac.StartReactive()

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
	rac.StopReactive()
}

// GetEventQueueStatus returns information about the event processing queue
func (rac *ReactiveAuthorizationCacheV2) GetEventQueueStatus() (queued int, capacity int) {
	return len(rac.eventQueue), cap(rac.eventQueue)
}
