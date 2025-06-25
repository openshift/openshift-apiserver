package auth

import (
	"fmt"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/controller"
)

func TestSyncNamespaceV2(t *testing.T) {
	namespaceList := corev1.NamespaceList{
		Items: []corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", ResourceVersion: "1"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "bar", ResourceVersion: "2"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "car", ResourceVersion: "3"},
			},
		},
	}
	roleBindingList := rbacv1.RoleBindingList{
		Items: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo-binding",
					Namespace:       "foo",
					ResourceVersion: "1",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar-binding",
					Namespace:       "bar",
					ResourceVersion: "2",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "car-binding",
					Namespace:       "car",
					ResourceVersion: "3",
				},
			},
		},
	}

	mockKubeClient := fake.NewSimpleClientset(&namespaceList, &roleBindingList)

	reviewer := &mockReviewer{
		expectedResults: map[string]*mockReview{
			"foo": {
				users:  []string{alice.GetName(), bob.GetName()},
				groups: eve.GetGroups(),
			},
			"bar": {
				users:  []string{frank.GetName(), eve.GetName()},
				groups: []string{"random"},
			},
			"car": {
				users:  []string{},
				groups: []string{},
			},
		},
	}

	informers := informers.NewSharedInformerFactory(mockKubeClient, controller.NoResyncPeriodFunc())
	nsIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	nsLister := corev1listers.NewNamespaceLister(nsIndexer)

	authorizationCache, err := NewAuthorizationCacheV2(
		nsLister,
		reviewer,
		informers.Rbac().V1(),
	)
	if err != nil {
		t.Fatalf("Failed to create authorization cache: %v", err)
	}

	// we prime the data we need here since we are not running reflectors
	for i := range namespaceList.Items {
		nsIndexer.Add(&namespaceList.Items[i])
	}

	rbIndexer := informers.Rbac().V1().RoleBindings().Informer().GetIndexer()
	for i := range roleBindingList.Items {
		rbIndexer.Add(&roleBindingList.Items[i])
	}

	// synchronize the cache
	authorizationCache.synchronize()

	validateList(t, authorizationCache, alice, sets.NewString("foo"))
	validateList(t, authorizationCache, bob, sets.NewString("foo"))
	validateList(t, authorizationCache, eve, sets.NewString("foo", "bar"))
	validateList(t, authorizationCache, frank, sets.NewString("bar"))

	// modify access rules
	reviewer.expectedResults["foo"].users = []string{bob.GetName()}
	reviewer.expectedResults["foo"].groups = []string{"random"}
	reviewer.expectedResults["bar"].users = []string{alice.GetName(), eve.GetName()}
	reviewer.expectedResults["bar"].groups = []string{"employee"}
	reviewer.expectedResults["car"].users = []string{bob.GetName(), eve.GetName()}
	reviewer.expectedResults["car"].groups = []string{"employee"}

	// modify resource version on each role binding to simulate a change had occurred - force cache refresh on each namespace
	for i := range roleBindingList.Items {
		roleBinding := roleBindingList.Items[i]
		oldVersion, err := strconv.Atoi(roleBinding.ResourceVersion)
		if err != nil {
			t.Errorf("Bad test setup, resource versions should be numbered, %v", err)
		}
		newVersion := strconv.Itoa(oldVersion + 1)
		roleBinding.ResourceVersion = newVersion
		rbIndexer.Add(&roleBinding)
	}

	// synchronize again
	authorizationCache.synchronize()

	// make sure new rights hold
	validateList(t, authorizationCache, alice, sets.NewString("bar"))
	validateList(t, authorizationCache, bob, sets.NewString("foo", "bar", "car"))
	validateList(t, authorizationCache, eve, sets.NewString("bar", "car"))
	validateList(t, authorizationCache, frank, sets.NewString())
}

func TestShouldSkipSyncV2(t *testing.T) {
	cr := func(rv string) rbacv1.ClusterRole {
		return rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("clusterrole-%s", rv),
				ResourceVersion: rv,
			},
		}
	}

	crb := func(rv string) rbacv1.ClusterRoleBinding {
		return rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("clusterrolebinding-%s", rv),
				ResourceVersion: rv,
			},
		}
	}

	type trial struct {
		crs      []rbacv1.ClusterRole
		crbs     []rbacv1.ClusterRoleBinding
		expected bool
	}

	for _, tc := range []struct {
		name   string
		trials []trial
	}{
		{
			name: "no changes",
			trials: []trial{
				{
					crs:      []rbacv1.ClusterRole{cr("1")},
					crbs:     []rbacv1.ClusterRoleBinding{crb("a")},
					expected: false,
				},
				{
					crs:      []rbacv1.ClusterRole{cr("1")},
					crbs:     []rbacv1.ClusterRoleBinding{crb("a")},
					expected: true,
				},
			},
		},
		{
			name: "clusterrole change",
			trials: []trial{
				{
					crs:      []rbacv1.ClusterRole{cr("1")},
					expected: false,
				},
				{
					crs:      []rbacv1.ClusterRole{cr("2")},
					expected: false,
				},
			},
		},
		{
			name: "clusterrolebinding change",
			trials: []trial{
				{
					crbs:     []rbacv1.ClusterRoleBinding{crb("a")},
					expected: false,
				},
				{
					crbs:     []rbacv1.ClusterRoleBinding{crb("b")},
					expected: false,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			crs := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			crbs := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})

			ac := &AuthorizationCacheV2{
				clusterRoleLister:        syncedClusterRoleLister{ClusterRoleLister: rbacv1listers.NewClusterRoleLister(crs)},
				clusterRoleBindingLister: syncedClusterRoleBindingLister{ClusterRoleBindingLister: rbacv1listers.NewClusterRoleBindingLister(crbs)},
				namespaceLister:          corev1listers.NewNamespaceLister(cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})),
			}

			for i, trial := range tc.trials {
				func() {
					for i := range trial.crs {
						crs.Add(&trial.crs[i])
						defer crs.Delete(&trial.crs[i])
					}
					for i := range trial.crbs {
						crbs.Add(&trial.crbs[i])
						defer crbs.Delete(&trial.crbs[i])
					}

					skip, _, _ := ac.shouldSkipSync()
					ac.synchronize()

					if skip != trial.expected {
						t.Errorf("expected %t on trial %d of %d, got %t", trial.expected, i+1, len(tc.trials), skip)
					}
				}()
			}
		})
	}
}

func TestNamespaceDeletionV2(t *testing.T) {
	namespaceList := corev1.NamespaceList{
		Items: []corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "to-delete", ResourceVersion: "1"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "to-keep", ResourceVersion: "2"},
			},
		},
	}

	mockKubeClient := fake.NewSimpleClientset(&namespaceList)
	reviewer := &mockReviewer{
		expectedResults: map[string]*mockReview{
			"to-delete": {
				users:  []string{alice.GetName()},
				groups: []string{"employee"},
			},
			"to-keep": {
				users:  []string{bob.GetName()},
				groups: []string{"admin"},
			},
		},
	}

	informers := informers.NewSharedInformerFactory(mockKubeClient, controller.NoResyncPeriodFunc())
	nsIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	nsLister := corev1listers.NewNamespaceLister(nsIndexer)

	authorizationCache, err := NewAuthorizationCacheV2(
		nsLister,
		reviewer,
		informers.Rbac().V1(),
	)
	if err != nil {
		t.Fatalf("Failed to create authorization cache: %v", err)
	}

	for i := range namespaceList.Items {
		nsIndexer.Add(&namespaceList.Items[i])
	}

	// Initial sync
	authorizationCache.synchronize()

	// Verify initial state
	validateList(t, authorizationCache, alice, sets.NewString("to-delete"))
	validateList(t, authorizationCache, bob, sets.NewString("to-keep", "to-delete")) // Note: Bob has access to "to-delete" due to "employee" group membership

	// Delete namespace from lister
	nsIndexer.Delete(&namespaceList.Items[0])

	// Sync again to trigger cleanup
	authorizationCache.synchronize()

	// Verify deletion cleaned up access
	validateList(t, authorizationCache, alice, sets.NewString())
	validateList(t, authorizationCache, bob, sets.NewString("to-keep"))

	// Verify internal state is cleaned up
	authorizationCache.hashLock.RLock()
	_, exists := authorizationCache.namespaceHashes["to-delete"]
	authorizationCache.hashLock.RUnlock()
	if exists {
		t.Error("Expected deleted namespace to be removed from hash cache")
	}

	authorizationCache.subjectLock.RLock()
	aliceNamespaces := authorizationCache.userToNamespaces[alice.GetName()]
	bobNamespaces := authorizationCache.userToNamespaces[bob.GetName()]
	authorizationCache.subjectLock.RUnlock()
	if aliceNamespaces != nil && aliceNamespaces.Has("to-delete") {
		t.Error("Expected deleted namespace to be removed from Alice user mappings")
	}
	if bobNamespaces != nil && bobNamespaces.Has("to-delete") {
		t.Error("Expected deleted namespace to be removed from Bob user mappings")
	}
}

func TestWatcherNotificationsV2(t *testing.T) {
	namespaceList := corev1.NamespaceList{
		Items: []corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ns", ResourceVersion: "1"},
			},
		},
	}

	mockKubeClient := fake.NewSimpleClientset(&namespaceList)
	reviewer := &mockReviewer{
		expectedResults: map[string]*mockReview{
			"test-ns": {
				users:  []string{alice.GetName()},
				groups: []string{"employee"},
			},
		},
	}

	informers := informers.NewSharedInformerFactory(mockKubeClient, controller.NoResyncPeriodFunc())
	nsIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	nsLister := corev1listers.NewNamespaceLister(nsIndexer)

	authorizationCache, err := NewAuthorizationCacheV2(
		nsLister,
		reviewer,
		informers.Rbac().V1(),
	)
	if err != nil {
		t.Fatalf("Failed to create authorization cache: %v", err)
	}

	nsIndexer.Add(&namespaceList.Items[0])

	// Create mock watcher
	watcher := &mockWatcher{
		notifications: make([]watcherNotification, 0),
	}
	authorizationCache.AddWatcher(watcher)

	// Initial sync
	authorizationCache.synchronize()

	// Should have one notification after initial sync
	if len(watcher.notifications) != 1 {
		t.Errorf("Expected 1 notification, got %d", len(watcher.notifications))
	}

	// Change permissions
	reviewer.expectedResults["test-ns"] = &mockReview{
		users:  []string{bob.GetName()},
		groups: []string{"admin"},
	}

	// Force a change via hash update
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-binding",
			Namespace:       "test-ns",
			ResourceVersion: "2",
		},
	}
	informers.Rbac().V1().RoleBindings().Informer().GetIndexer().Add(roleBinding)

	// Sync again
	authorizationCache.synchronize()

	// Should have two notifications now
	if len(watcher.notifications) != 2 {
		t.Errorf("Expected 2 notifications, got %d", len(watcher.notifications))
	}

	// Test watcher removal
	authorizationCache.RemoveWatcher(watcher)

	// Simulate another change
	reviewer.expectedResults["test-ns"].users = []string{alice.GetName()}
	informers.Rbac().V1().RoleBindings().Informer().GetIndexer().Delete(roleBinding)

	// Another sync should not add more notifications
	authorizationCache.synchronize()
	if len(watcher.notifications) != 2 {
		t.Errorf("Expected watcher removal to prevent new notifications, got %d", len(watcher.notifications))
	}
}

func TestErrorHandlingV2(t *testing.T) {
	mockKubeClient := fake.NewSimpleClientset()
	failingReviewer := &mockReviewer{
		expectedResults: map[string]*mockReview{}, // empty will cause review attempt for error-ns to return an error
	}

	informers := informers.NewSharedInformerFactory(mockKubeClient, controller.NoResyncPeriodFunc())
	nsIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	nsLister := corev1listers.NewNamespaceLister(nsIndexer)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "error-ns", ResourceVersion: "1"},
	}
	nsIndexer.Add(ns)

	authorizationCache, err := NewAuthorizationCacheV2(
		nsLister,
		failingReviewer,
		informers.Rbac().V1(),
	)
	if err != nil {
		t.Fatalf("Failed to create authorization cache: %v", err)
	}

	// Sync should handle reviewer errors gracefully - log but don't crash
	authorizationCache.synchronize()

	// Cache should still function for List operations even with review errors
	namespaceList, err := authorizationCache.List(alice, labels.Everything())
	if err != nil {
		t.Errorf("List should handle reviewer errors gracefully, got: %v", err)
	}
	if len(namespaceList.Items) != 0 {
		t.Errorf("Expected empty list due to review errors, got %d items", len(namespaceList.Items))
	}
}

func TestReadyStateV2(t *testing.T) {
	mockKubeClient := fake.NewSimpleClientset()
	reviewer := &mockReviewer{
		expectedResults: map[string]*mockReview{},
	}

	informers := informers.NewSharedInformerFactory(mockKubeClient, controller.NoResyncPeriodFunc())
	nsIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	nsLister := corev1listers.NewNamespaceLister(nsIndexer)

	authorizationCache, err := NewAuthorizationCacheV2(
		nsLister,
		reviewer,
		informers.Rbac().V1(),
	)
	if err != nil {
		t.Fatalf("Failed to create authorization cache: %v", err)
	}

	// Initially not ready
	if authorizationCache.ReadyForAccess() {
		t.Error("Cache should not be ready before first synchronization")
	}

	authorizationCache.synchronize()

	// After sync should be ready
	if !authorizationCache.ReadyForAccess() {
		t.Error("Cache should be ready after synchronization")
	}
}

func TestHashComputationV2(t *testing.T) {
	mockKubeClient := fake.NewSimpleClientset()
	reviewer := &mockReviewer{
		expectedResults: map[string]*mockReview{},
	}

	informers := informers.NewSharedInformerFactory(mockKubeClient, controller.NoResyncPeriodFunc())
	nsIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	nsLister := corev1listers.NewNamespaceLister(nsIndexer)

	authorizationCache, err := NewAuthorizationCacheV2(
		nsLister,
		reviewer,
		informers.Rbac().V1(),
	)
	if err != nil {
		t.Fatalf("Failed to create authorization cache: %v", err)
	}

	// Test hash computation with no resources
	hash1, err := authorizationCache.computeGlobalRBACHash()
	if err != nil {
		t.Errorf("Failed to compute initial hash: %v", err)
	}

	// Add a cluster role
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role",
			ResourceVersion: "1",
			UID:             "test-uid",
		},
	}
	informers.Rbac().V1().ClusterRoles().Informer().GetIndexer().Add(clusterRole)

	// Hash should change
	hash2, err := authorizationCache.computeGlobalRBACHash()
	if err != nil {
		t.Errorf("Failed to compute hash after adding cluster role: %v", err)
	}

	if hash1 == hash2 {
		t.Error("Hash should change when cluster role is added")
	}

	// Test deterministic hash computation
	hash3, err := authorizationCache.computeGlobalRBACHash()
	if err != nil {
		t.Errorf("Failed to compute hash again: %v", err)
	}

	if hash2 != hash3 {
		t.Error("Hash computation should be deterministic")
	}
}

func TestLabelSelectorV2(t *testing.T) {
	namespaceList := corev1.NamespaceList{
		Items: []corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "labeled-ns",
					ResourceVersion: "1",
					Labels: map[string]string{
						"environment": "test",
						"team":        "alpha",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "unlabeled-ns",
					ResourceVersion: "2",
				},
			},
		},
	}

	mockKubeClient := fake.NewSimpleClientset(&namespaceList)
	reviewer := &mockReviewer{
		expectedResults: map[string]*mockReview{
			"labeled-ns": {
				users:  []string{alice.GetName()},
				groups: []string{},
			},
			"unlabeled-ns": {
				users:  []string{alice.GetName()},
				groups: []string{},
			},
		},
	}

	informers := informers.NewSharedInformerFactory(mockKubeClient, controller.NoResyncPeriodFunc())
	nsIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	nsLister := corev1listers.NewNamespaceLister(nsIndexer)

	authorizationCache, err := NewAuthorizationCacheV2(
		nsLister,
		reviewer,
		informers.Rbac().V1(),
	)
	if err != nil {
		t.Fatalf("Failed to create authorization cache: %v", err)
	}

	for i := range namespaceList.Items {
		nsIndexer.Add(&namespaceList.Items[i])
	}

	authorizationCache.synchronize()

	// Test with environment=test selector
	envSelector, _ := labels.Parse("environment=test")
	namespaceListCheck, err := authorizationCache.List(alice, envSelector)
	if err != nil {
		t.Errorf("List with label selector failed: %v", err)
	}

	if len(namespaceListCheck.Items) != 1 || namespaceListCheck.Items[0].Name != "labeled-ns" {
		t.Errorf("Expected only labeled-ns to match environment=test selector, got %v",
			extractNamespaceNames(namespaceList.Items))
	}

	// Test with team=beta selector (should match nothing)
	teamSelector, _ := labels.Parse("team=beta")
	namespaceListCheck, err = authorizationCache.List(alice, teamSelector)
	if err != nil {
		t.Errorf("List with label selector failed: %v", err)
	}

	if len(namespaceListCheck.Items) != 0 {
		t.Errorf("Expected no namespaces to match team=beta selector, got %v",
			extractNamespaceNames(namespaceListCheck.Items))
	}
}

func TestConcurrentAccessV2(t *testing.T) {
	namespaceList := corev1.NamespaceList{
		Items: []corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "concurrent-ns", ResourceVersion: "1"},
			},
		},
	}

	mockKubeClient := fake.NewSimpleClientset(&namespaceList)
	reviewer := &mockReviewer{
		expectedResults: map[string]*mockReview{
			"concurrent-ns": {
				users:  []string{alice.GetName()},
				groups: []string{},
			},
		},
	}

	informers := informers.NewSharedInformerFactory(mockKubeClient, controller.NoResyncPeriodFunc())
	nsIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	nsLister := corev1listers.NewNamespaceLister(nsIndexer)

	authorizationCache, err := NewAuthorizationCacheV2(
		nsLister,
		reviewer,
		informers.Rbac().V1(),
	)
	if err != nil {
		t.Fatalf("Failed to create authorization cache: %v", err)
	}

	nsIndexer.Add(&namespaceList.Items[0])

	crIndexer := informers.Rbac().V1().ClusterRoles().Informer().GetIndexer()
	rbIndexer := informers.Rbac().V1().RoleBindings().Informer().GetIndexer()

	const numGoroutines = 20
	const numOperationsPerGoroutine = 50

	done := make(chan bool, numGoroutines)
	errCh := make(chan error, numGoroutines*numOperationsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for j := 0; j < numOperationsPerGoroutine; j++ {
				operationType := j % 3

				// Simulate some work
				// 1/3 will be list operations, 1/3 will update a cluster role, and 1/3 will update a role binding
				// should exercise both full sync and namespace-specific changes.
				switch operationType {
				case 0:
					// List operation
					_, err := authorizationCache.List(alice, labels.Everything())
					if err != nil {
						select {
						case errCh <- fmt.Errorf("List operation failed: %v", err):
						default:
						}
					}
				case 1:
					// Update a cluster role to trigger full sync
					clusterRole := &rbacv1.ClusterRole{
						ObjectMeta: metav1.ObjectMeta{
							Name:            fmt.Sprintf("test-role-%d", goroutineID),
							ResourceVersion: fmt.Sprintf("%d", j+1),
							UID:             types.UID(fmt.Sprintf("uid-%d-%d", goroutineID, j)),
						},
					}
					if err := crIndexer.Add(clusterRole); err != nil {
						select {
						case errCh <- fmt.Errorf("ClusterRole add failed: %v", err):
						default:
						}
					}

					authorizationCache.synchronize()
				case 2:
					// Update a role binding to trigger namespace-specific changes
					roleBinding := &rbacv1.RoleBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:            fmt.Sprintf("test-binding-%d", goroutineID),
							Namespace:       "concurrent-ns",
							ResourceVersion: fmt.Sprintf("%d", j+1),
							UID:             types.UID(fmt.Sprintf("uid-%d-%d", goroutineID, j)),
						},
					}
					if err := rbIndexer.Add(roleBinding); err != nil {
						select {
						case errCh <- fmt.Errorf("RoleBinding add failed: %v", err):
						default:
						}
					}

					authorizationCache.synchronize()
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	select {
	case err := <-errCh:
		t.Errorf("Concurrent operation encountered an error: %v", err)
	default:
		// No errors, all operations succeeded
	}

	validateList(t, authorizationCache, alice, sets.NewString("concurrent-ns"))
}
