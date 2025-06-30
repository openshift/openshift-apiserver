package auth

import (
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/cache"
)

func setupTestGlobalRBACCache(_ *testing.T) (*GlobalRBACCache, cache.Indexer, cache.Indexer) {
	crIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	crbIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})

	crLister := rbacv1listers.NewClusterRoleLister(crIndexer)
	crbLister := rbacv1listers.NewClusterRoleBindingLister(crbIndexer)

	crVersioner := &dynamicVersioner{version: 100}
	crbVersioner := &dynamicVersioner{version: 200}

	syncedCRLister := syncedClusterRoleLister{
		ClusterRoleLister: crLister,
		versioner:         crVersioner,
	}
	syncedCRBLister := syncedClusterRoleBindingLister{
		ClusterRoleBindingLister: crbLister,
		versioner:                crbVersioner,
	}

	return NewGlobalRBACCache(syncedCRLister, syncedCRBLister), crIndexer, crbIndexer
}

func TestGlobalRBACCache_hasResourceVersionChanged(t *testing.T) {
	tests := []struct {
		name                             string
		initialClusterRoleVersion        string
		initialClusterRoleBindingVersion string
		currentClusterRoleVersion        string
		currentClusterRoleBindingVersion string
		expectedChanged                  bool
	}{
		{
			name:                             "no change",
			initialClusterRoleVersion:        "123",
			initialClusterRoleBindingVersion: "456",
			currentClusterRoleVersion:        "123",
			currentClusterRoleBindingVersion: "456",
			expectedChanged:                  false,
		},
		{
			name:                             "cluster role version changed",
			initialClusterRoleVersion:        "123",
			initialClusterRoleBindingVersion: "456",
			currentClusterRoleVersion:        "124",
			currentClusterRoleBindingVersion: "456",
			expectedChanged:                  true,
		},
		{
			name:                             "cluster role binding version changed",
			initialClusterRoleVersion:        "123",
			initialClusterRoleBindingVersion: "456",
			currentClusterRoleVersion:        "123",
			currentClusterRoleBindingVersion: "457",
			expectedChanged:                  true,
		},
		{
			name:                             "both versions changed",
			initialClusterRoleVersion:        "123",
			initialClusterRoleBindingVersion: "456",
			currentClusterRoleVersion:        "124",
			currentClusterRoleBindingVersion: "457",
			expectedChanged:                  true,
		},
		{
			name:                             "no cached versions",
			initialClusterRoleVersion:        "",
			initialClusterRoleBindingVersion: "",
			currentClusterRoleVersion:        "123",
			currentClusterRoleBindingVersion: "456",
			expectedChanged:                  true,
		},
		{
			name:                             "missing cached cluster role version",
			initialClusterRoleVersion:        "",
			initialClusterRoleBindingVersion: "456",
			currentClusterRoleVersion:        "123",
			currentClusterRoleBindingVersion: "456",
			expectedChanged:                  true,
		},
		{
			name:                             "missing cached cluster role binding version",
			initialClusterRoleVersion:        "123",
			initialClusterRoleBindingVersion: "",
			currentClusterRoleVersion:        "123",
			currentClusterRoleBindingVersion: "456",
			expectedChanged:                  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterRoleLister := &mockClusterRoleLister{
				resourceVersion: tt.currentClusterRoleVersion,
			}
			clusterRoleBindingLister := &mockClusterRoleBindingLister{
				resourceVersion: tt.currentClusterRoleBindingVersion,
			}

			cache := NewGlobalRBACCache(clusterRoleLister, clusterRoleBindingLister)

			cache.lastClusterRoleVersion = tt.initialClusterRoleVersion
			cache.lastClusterRoleBindingVersion = tt.initialClusterRoleBindingVersion

			result := cache.hasResourceVersionChanged()

			if result != tt.expectedChanged {
				t.Errorf("Expected hasResourceVersionChanged to be %v, got %v", tt.expectedChanged, result)
			}
		})
	}
}

func TestGlobalRBACCache_updateResourceVersions(t *testing.T) {
	tests := []struct {
		name                       string
		clusterRoleVersion         string
		clusterRoleBindingVersion  string
	}{
		{
			name:                       "update both versions",
			clusterRoleVersion:         "123",
			clusterRoleBindingVersion:  "456",
		},
		{
			name:                       "empty cluster role version",
			clusterRoleVersion:         "",
			clusterRoleBindingVersion:  "456",
		},
		{
			name:                       "empty cluster role binding version",
			clusterRoleVersion:         "123",
			clusterRoleBindingVersion:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterRoleLister := &mockClusterRoleLister{
				resourceVersion: tt.clusterRoleVersion,
			}
			clusterRoleBindingLister := &mockClusterRoleBindingLister{
				resourceVersion: tt.clusterRoleBindingVersion,
			}

			cache := NewGlobalRBACCache(clusterRoleLister, clusterRoleBindingLister)
			cache.updateResourceVersions()

			if cache.lastClusterRoleVersion != tt.clusterRoleVersion {
				t.Errorf("Expected lastClusterRoleVersion to be %s, got %s", tt.clusterRoleVersion, cache.lastClusterRoleVersion)
			}
			if cache.lastClusterRoleBindingVersion != tt.clusterRoleBindingVersion {
				t.Errorf("Expected lastClusterRoleBindingVersion to be %s, got %s", tt.clusterRoleBindingVersion, cache.lastClusterRoleBindingVersion)
			}
		})
	}
}

func TestGlobalRBACCache_listClusterRBACRefs(t *testing.T) {
	cache, crIndexer, crbIndexer := setupTestGlobalRBACCache(t)

	clusterRole1 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role-1",
			ResourceVersion: "1",
			UID:             types.UID("uid-1"),
		},
	}
	clusterRole2 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role-2",
			ResourceVersion: "2",
			UID:             types.UID("uid-2"),
		},
	}

	clusterRoleBinding1 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-binding-1",
			ResourceVersion: "3",
			UID:             types.UID("uid-3"),
		},
	}
	clusterRoleBinding2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-binding-2",
			ResourceVersion: "4",
			UID:             types.UID("uid-4"),
		},
	}

	if err := crIndexer.Add(clusterRole1); err != nil {
		t.Fatalf("Failed to add cluster role 1: %v", err)
	}
	if err := crIndexer.Add(clusterRole2); err != nil {
		t.Fatalf("Failed to add cluster role 2: %v", err)
	}
	if err := crbIndexer.Add(clusterRoleBinding1); err != nil {
		t.Fatalf("Failed to add cluster role binding 1: %v", err)
	}
	if err := crbIndexer.Add(clusterRoleBinding2); err != nil {
		t.Fatalf("Failed to add cluster role binding 2: %v", err)
	}

	refs, err := cache.listClusterRBACRefs()
	if err != nil {
		t.Fatalf("listClusterRBACRefs failed: %v", err)
	}

	expectedRefs := []rbacResourceRef{
		{uid: "uid-1", resourceVersion: "1", kind: "ClusterRole"},
		{uid: "uid-2", resourceVersion: "2", kind: "ClusterRole"},
		{uid: "uid-3", resourceVersion: "3", kind: "ClusterRoleBinding"},
		{uid: "uid-4", resourceVersion: "4", kind: "ClusterRoleBinding"},
	}

	if len(refs) != len(expectedRefs) {
		t.Errorf("Expected %d refs, got %d", len(expectedRefs), len(refs))
	}

	refMap := make(map[string]rbacResourceRef)
	for _, ref := range refs {
		refMap[ref.uid] = ref
	}

	for _, expected := range expectedRefs {
		actual, found := refMap[expected.uid]
		if !found {
			t.Errorf("Expected ref with UID %s not found", expected.uid)
			continue
		}
		if actual != expected {
			t.Errorf("Expected ref %+v, got %+v", expected, actual)
		}
	}
}

func TestGlobalRBACCache_ComputeHash(t *testing.T) {
	cache, crIndexer, crbIndexer := setupTestGlobalRBACCache(t)

	hash1, err := cache.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	if hash1 == "" {
		t.Error("Expected non-empty hash")
	}

	if cache.GetCurrentHash() != hash1 {
		t.Error("Hash was not cached properly")
	}

	// Add a cluster role and verify hash changes
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role",
			ResourceVersion: "1",
			UID:             types.UID("uid-1"),
		},
	}
	if err := crIndexer.Add(clusterRole); err != nil {
		t.Fatalf("Failed to add cluster role: %v", err)
	}

	hash2, err := cache.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	if hash2 == hash1 {
		t.Error("Hash should have changed after adding cluster role")
	}

	// Add a cluster role binding and verify hash changes again
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-binding",
			ResourceVersion: "2",
			UID:             types.UID("uid-2"),
		},
	}
	if err := crbIndexer.Add(clusterRoleBinding); err != nil {
		t.Fatalf("Failed to add cluster role binding: %v", err)
	}

	hash3, err := cache.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	if hash3 == hash2 {
		t.Error("Hash should have changed after adding cluster role binding")
	}

	// Verify that computing the same resources produces the same hash
	hash4, err := cache.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	if hash4 != hash3 {
		t.Error("Hash should be consistent for the same resources")
	}
}

func TestGlobalRBACCache_ComputeHash_Optimization(t *testing.T) {
	clusterRoleLister := &mockClusterRoleLister{
		resourceVersion: "100",
		clusterRoles:    []*rbacv1.ClusterRole{},
	}
	clusterRoleBindingLister := &mockClusterRoleBindingLister{
		resourceVersion:     "200",
		clusterRoleBindings: []*rbacv1.ClusterRoleBinding{},
	}
	cache := NewGlobalRBACCache(clusterRoleLister, clusterRoleBindingLister)

	// Set up initial state
	cache.globalRBACHash = "initial-hash"
	cache.lastClusterRoleVersion = "100"
	cache.lastClusterRoleBindingVersion = "200"

	// Since resource versions haven't changed, it should return cached hash
	hash, err := cache.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	if hash != "initial-hash" {
		t.Errorf("Expected cached hash 'initial-hash', got %s", hash)
	}

	// Change a resource version
	clusterRoleLister.resourceVersion = "101"

	// Now it should compute a new hash
	hash2, err := cache.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	// The hash should be different since we force recomputation
	if hash2 == "initial-hash" {
		t.Error("Expected hash to change when resource version changes")
	}
}

func TestGlobalRBACCache_shouldSkipSyncGlobal(t *testing.T) {
	tests := []struct {
		name                    string
		initialHash             string
		resourceVersionsChanged bool
		canOptimize             bool
		expectedSkip            bool
	}{
		{
			name:                    "can optimize and no changes",
			initialHash:             "test-hash",
			resourceVersionsChanged: false,
			canOptimize:             true,
			expectedSkip:            true,
		},
		{
			name:                    "can optimize but resource versions changed",
			initialHash:             "test-hash",
			resourceVersionsChanged: true,
			canOptimize:             true,
			expectedSkip:            false,
		},
		{
			name:                    "cannot optimize, hash unchanged",
			initialHash:             computeHashFromRefs([]rbacResourceRef{}),
			resourceVersionsChanged: false,
			canOptimize:             false,
			expectedSkip:            true,
		},
		{
			name:                    "cannot optimize, hash changed",
			initialHash:             "old-hash",
			resourceVersionsChanged: false,
			canOptimize:             false,
			expectedSkip:            false,
		},
		{
			name:                    "empty initial hash forces computation",
			initialHash:             "",
			resourceVersionsChanged: false,
			canOptimize:             true,
			expectedSkip:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cache *GlobalRBACCache
			cache, _, _ = setupTestGlobalRBACCache(t)

			// Override the optimization behavior for testing
			if tt.canOptimize {
				// Set up versioned listers
				cache.clusterRoleLister = &mockClusterRoleLister{
					resourceVersion: "100",
					clusterRoles:    []*rbacv1.ClusterRole{},
				}
				cache.clusterRoleBindingLister = &mockClusterRoleBindingLister{
					resourceVersion:     "200",
					clusterRoleBindings: []*rbacv1.ClusterRoleBinding{},
				}

				if tt.resourceVersionsChanged {
					// Set different cached versions to simulate change
					cache.lastClusterRoleVersion = "99"
					cache.lastClusterRoleBindingVersion = "199"
				} else {
					// Set same cached versions to simulate no change
					cache.lastClusterRoleVersion = "100"
					cache.lastClusterRoleBindingVersion = "200"
				}
			} else {
				cache.clusterRoleLister = &mockClusterRoleLister{
					resourceVersion: "",
					clusterRoles:    []*rbacv1.ClusterRole{},
				}
				cache.clusterRoleBindingLister = &mockClusterRoleBindingLister{
					resourceVersion:     "",
					clusterRoleBindings: []*rbacv1.ClusterRoleBinding{},
				}
			}

			// Set initial hash
			cache.globalRBACHash = tt.initialHash

			skip, err := cache.shouldSkipSyncGlobal()

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if skip != tt.expectedSkip {
				t.Errorf("Expected skip to be %v, got %v", tt.expectedSkip, skip)
			}
		})
	}
}
