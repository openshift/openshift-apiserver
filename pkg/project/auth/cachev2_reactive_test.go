package auth

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

// Shared test setup helper
func setupReactiveCacheV2(t *testing.T, config *ReactiveAuthorizationCacheV2Config) (*ReactiveAuthorizationCacheV2, *mockReviewer, context.CancelFunc, *kubefake.Clientset) {
	fakeClient := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	namespaceInformer := informerFactory.Core().V1().Namespaces()
	rbacInformers := informerFactory.Rbac().V1()
	reviewer := newMockReviewer()

	if config == nil {
		config = DefaultReactiveAuthorizationCacheV2Config()
		config.DebounceDuration = 50 * time.Millisecond // Faster than default for testing
	}

	reactiveCache, err := NewReactiveAuthorizationCacheV2WithConfig(
		namespaceInformer.Lister(),
		reviewer,
		rbacInformers,
		namespaceInformer,
		config,
	)
	if err != nil {
		t.Fatalf("Failed to create reactive cache: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	informerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), namespaceInformer.Informer().HasSynced)

	return reactiveCache, reviewer, cancel, fakeClient
}

func TestReactiveCacheV2_DefaultConfiguration(t *testing.T) {
	reactiveCache, _, cancel, _ := setupReactiveCacheV2(t, nil)
	defer cancel()

	if reactiveCache.eventWorkers != 5 {
		t.Errorf("Expected 5 event workers, got %d", reactiveCache.eventWorkers)
	}

	// Normal default debounce is 500ms, but we set it to 50ms for simple tests
	if reactiveCache.debounceDuration != 50*time.Millisecond {
		t.Errorf("Expected 50ms debounce duration, got %v", reactiveCache.debounceDuration)
	}

	queued, capacity := reactiveCache.GetEventQueueStatus()
	if capacity != 1000 {
		t.Errorf("Expected queue capacity of 1000, got %d", capacity)
	}
	if queued != 0 {
		t.Errorf("Expected empty queue initially, got %d items", queued)
	}

	// Test lifecycle
	reactiveCache.Run(5 * time.Minute)

	if queued != 0 {
		t.Errorf("Expected empty queue given no items to process, got %d items", queued)
	}

	reactiveCache.Stop()
}

// covers enqueuing, processing, and basic debouncing
func TestReactiveCacheV2_EventProcessing(t *testing.T) {
	config := &ReactiveAuthorizationCacheV2Config{
		DebounceDuration: 150 * time.Millisecond,
		EventQueueSize:   50,
		EventWorkers:     1,
	}

	reactiveCache, reviewer, cancel, _ := setupReactiveCacheV2(t, config)
	defer cancel()

	reviewer.setReview("test-ns", []string{"user1"}, []string{"group1"})

	reactiveCache.Run(5 * time.Minute)
	defer reactiveCache.Stop()

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role",
			ResourceVersion: "1",
		},
	}

	// Enqueue several events
	for i := 0; i < 30; i++ {
		reactiveCache.enqueueEvent(EventTypeClusterRoleChanged, role, "")
	}

	// Wait less than debounce duration - events should be processed off the queue but debounce should still be pending
	time.Sleep(50 * time.Millisecond)

	// Check that the event queue has processed the events
	queued, _ := reactiveCache.GetEventQueueStatus()
	if queued != 0 {
		t.Errorf("Expected event queue to have been processed into debounced calls, got %d items still enqueued", queued)
	}

	// Check that debounce timer is set up
	reactiveCache.debounceLock.Lock()
	if reactiveCache.debounceTimer == nil {
		t.Error("Expected debounce timer to be initialized")
	}

	// Check that a global sync is pending
	if !reactiveCache.isGlobalSyncPending {
		t.Error("Expected global sync to be pending")
	}
	reactiveCache.debounceLock.Unlock()

	// Wait for debounce & sync to complete
	time.Sleep(100 * time.Millisecond)

	reactiveCache.debounceLock.Lock()
	if reactiveCache.isGlobalSyncPending {
		t.Error("Expected global sync to have completed after debounce")
	}
	reactiveCache.debounceLock.Unlock()
}

func TestReactiveCacheV2_WatcherNotifications(t *testing.T) {
	reactiveCache, reviewer, cancel, fakeClient := setupReactiveCacheV2(t, nil)
	defer cancel()

	ctx := context.Background()

	testNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
		},
	}
	_, err := fakeClient.CoreV1().Namespaces().Create(ctx, testNS, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test namespace: %v", err)
	}

	reviewer.setReview("test-ns", []string{""}, []string{""})

	watcher := newMockWatcher()

	reactiveCache.Run(5 * time.Minute)
	defer reactiveCache.Stop()

	// initial sync
	time.Sleep(100 * time.Millisecond)

	reactiveCache.AddWatcher(watcher)

	// change the reviewer data and create a namespace-scoped RBAC resource to trigger a change
	reviewer.setReview("test-ns", []string{"user1"}, []string{"group1"})
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role",
			Namespace:       "test-ns",
			ResourceVersion: "1",
		},
	}

	_, err = fakeClient.RbacV1().Roles("test-ns").Create(ctx, role, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	notifications := watcher.getNotifications()
	if len(notifications) != 1 {
		t.Error("Expected watcher to receive a notification after role creation")
	}

	// Verify the notification contains the expected data
	if notifications[0].namespace != "test-ns" {
		t.Errorf("Expected notification for namespace 'test-ns', got: %+v", notifications[0])
	}
	expectedUsers := sets.NewString("user1")
	if !notifications[0].users.Equal(expectedUsers) {
		t.Errorf("Expected users to be 'user1', got: %v", notifications[0].users)
	}
	expectedGroups := sets.NewString("group1")
	if !notifications[0].groups.Equal(expectedGroups) {
		t.Errorf("Expected groups to be 'group1', got: %v", notifications[0].groups)
	}

	// setup for second test case
	watcher.clearNotifications()

	// change the reviewer data and create a cluster RBAC resource to trigger a change
	reviewer.setReview("test-ns", []string{"user1", "user2"}, []string{"group1", "group2"})
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role",
			ResourceVersion: "1",
		},
	}

	_, err = fakeClient.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create clusterrole: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	notifications = watcher.getNotifications()
	if len(notifications) != 1 {
		t.Error("Expected watcher to receive a notification after role creation")
	}

	// Verify the notification contains the expected data
	if notifications[0].namespace != "test-ns" {
		t.Errorf("Expected notification for namespace 'test-ns', got: %+v", notifications[0])
	}
	expectedUsers = sets.NewString("user1", "user2")
	if !notifications[0].users.Equal(expectedUsers) {
		t.Errorf("Expected users to be 'user1', 'user2', got: %v", notifications[0].users)
	}
	expectedGroups = sets.NewString("group1", "group2")
	if !notifications[0].groups.Equal(expectedGroups) {
		t.Errorf("Expected groups to be 'group1', 'group2', got: %v", notifications[0].groups)
	}

	// Test watcher removal
	reactiveCache.RemoveWatcher(watcher)
	watcher.clearNotifications()

	// Change data again and create another RBAC resource
	reviewer.setReview("test-ns", []string{"user3"}, []string{"group3"})
	role2 := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role-2",
			Namespace:       "test-ns",
			ResourceVersion: "2",
		},
	}

	_, err = fakeClient.RbacV1().Roles("test-ns").Create(ctx, role2, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create role2: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	notifications = watcher.getNotifications()
	if len(notifications) > 0 {
		t.Error("Removed watcher should not receive notifications")
	}
}
