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
	"k8s.io/client-go/util/workqueue"
)

// Shared test setup helper
func setupReactiveCacheV2(t *testing.T, config *ReactiveAuthorizationCacheV2Config) (*ReactiveAuthorizationCacheV2, *mockReviewer, context.CancelFunc, *kubefake.Clientset) {
	// Start with an empty fake client but with proper objects list initialized
	fakeClient := kubefake.NewSimpleClientset()

	// Use a resync period for testing
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 30*time.Second)
	namespaceInformer := informerFactory.Core().V1().Namespaces()
	rbacInformers := informerFactory.Rbac().V1()
	reviewer := newMockReviewer()

	if config == nil {
		config = DefaultReactiveAuthorizationCacheV2Config()
		config.RateLimiter = workqueue.NewTypedItemFastSlowRateLimiter[string](
			10*time.Millisecond, 100*time.Millisecond, 5)
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

	synced := cache.WaitForCacheSync(ctx.Done(),
		namespaceInformer.Informer().HasSynced,
		rbacInformers.ClusterRoles().Informer().HasSynced,
		rbacInformers.ClusterRoleBindings().Informer().HasSynced,
		rbacInformers.Roles().Informer().HasSynced,
		rbacInformers.RoleBindings().Informer().HasSynced,
	)
	if !synced {
		cancel()
		t.Fatalf("Failed to sync informers")
	}

	return reactiveCache, reviewer, cancel, fakeClient
}

func TestReactiveCacheV2_DefaultConfiguration(t *testing.T) {
	reactiveCache, _, cancel, _ := setupReactiveCacheV2(t, nil)
	defer cancel()

	if reactiveCache.workers != 5 {
		t.Errorf("Expected 5 workers, got %d", reactiveCache.workers)
	}

	queued := reactiveCache.GetQueueStatus()
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

// covers enqueuing & processing via workqueue deduplication
func TestReactiveCacheV2_EventProcessing(t *testing.T) {
	config := &ReactiveAuthorizationCacheV2Config{
		Workers:     1,
		RateLimiter: workqueue.NewTypedItemFastSlowRateLimiter[string](10*time.Millisecond, 100*time.Millisecond, 5),
		QueueName:   "test_queue",
	}

	reactiveCache, reviewer, cancel, fakeClient := setupReactiveCacheV2(t, config)
	defer cancel()

	initialQueueLen := reactiveCache.queue.Len()

	if initialQueueLen != 0 {
		t.Errorf("Expected empty queue initially, got %d items", initialQueueLen)
	}

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

	reviewer.setReview("test-ns", []string{"user1"}, []string{"group1"})

	time.Sleep(200 * time.Millisecond)

	queueAfterAddingNamespace := reactiveCache.queue.Len()

	if queueAfterAddingNamespace != 1 {
		t.Errorf("Expected queue length 1, got %d", queueAfterAddingNamespace)
	}

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

	time.Sleep(200 * time.Millisecond)

	queueAfterClusterRoleCreation := reactiveCache.queue.Len()
	if queueAfterClusterRoleCreation != 2 {
		t.Errorf("Expected queue length 2 after cluster role creation, got %d", queueAfterClusterRoleCreation)
	}

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

	time.Sleep(200 * time.Millisecond)

	queueAfterRoleCreation := reactiveCache.queue.Len()
	
	// This namespace should already be in the queue from the initial namespace creation, no new queued workloads expected
	if queueAfterRoleCreation != 2 {
		t.Errorf("Expected queue length 2 after role creation, got %d", queueAfterRoleCreation)
	}
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
	time.Sleep(200 * time.Millisecond)

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

	time.Sleep(250 * time.Millisecond)

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

	time.Sleep(250 * time.Millisecond)

	notifications = watcher.getNotifications()
	if len(notifications) > 0 {
		t.Error("Removed watcher should not receive notifications")
	}
}
