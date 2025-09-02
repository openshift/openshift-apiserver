package auth

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/utils/clock"
)

// mockedClock returns always the same values for Now() and Since().
type mockedClock struct {
	now   time.Time
	since time.Duration
}

// Now returns always the same time.
func (m *mockedClock) Now() time.Time {
	return m.now
}

// Since returns always the same duration regardless of the input time.
func (m *mockedClock) Since(time.Time) time.Duration {
	return m.since
}

// mockReview implements the Review interface for test cases
type mockReview struct {
	users  []string
	groups []string
	err    string
}

// Users returns the users that can access a resource
func (r *mockReview) Users() []string {
	return r.users
}

// Groups returns the groups that can access a resource
func (r *mockReview) Groups() []string {
	return r.groups
}

func (r *mockReview) EvaluationError() string {
	return r.err
}

// common test users
var (
	alice = &user.DefaultInfo{
		Name:   "Alice",
		UID:    "alice-uid",
		Groups: []string{},
	}
	bob = &user.DefaultInfo{
		Name:   "Bob",
		UID:    "bob-uid",
		Groups: []string{"employee"},
	}
	eve = &user.DefaultInfo{
		Name:   "Eve",
		UID:    "eve-uid",
		Groups: []string{"employee"},
	}
	frank = &user.DefaultInfo{
		Name:   "Frank",
		UID:    "frank-uid",
		Groups: []string{},
	}
)

// mockReviewer returns the specified values for each supplied resource
type mockReviewer struct {
	expectedResults map[string]*mockReview
}

// Review returns the mapped review from the mock object, or an error if none exists
func (mr *mockReviewer) Review(name string) (Review, error) {
	review := mr.expectedResults[name]
	if review == nil {
		return nil, fmt.Errorf("Item %s does not exist", name)
	}
	return review, nil
}

func validateList(t *testing.T, lister Lister, user user.Info, expectedSet sets.String) {
	namespaceList, err := lister.List(user, labels.Everything())
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	results := sets.String{}
	for _, namespace := range namespaceList.Items {
		results.Insert(namespace.Name)
	}
	if results.Len() != expectedSet.Len() || !results.HasAll(expectedSet.List()...) {
		t.Errorf("User %v, Expected: %v, Actual: %v", user.GetName(), expectedSet, results)
	}
}

func TestSyncNamespace(t *testing.T) {
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
	mockKubeClient := fake.NewSimpleClientset(&namespaceList)

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

	authorizationCache := NewAuthorizationCache(
		nsLister,
		informers.Core().V1().Namespaces().Informer(),
		reviewer,
		informers.Rbac().V1(),
	)
	// we prime the data we need here since we are not running reflectors
	for i := range namespaceList.Items {
		nsIndexer.Add(&namespaceList.Items[i])
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

	// modify resource version on each namespace to simulate a change had occurred to force cache refresh
	for i := range namespaceList.Items {
		namespace := namespaceList.Items[i]
		oldVersion, err := strconv.Atoi(namespace.ResourceVersion)
		if err != nil {
			t.Errorf("Bad test setup, resource versions should be numbered, %v", err)
		}
		newVersion := strconv.Itoa(oldVersion + 1)
		namespace.ResourceVersion = newVersion
		nsIndexer.Add(&namespace)
	}

	// now refresh the cache (which is resource version aware)
	authorizationCache.synchronize()

	// make sure new rights hold
	validateList(t, authorizationCache, alice, sets.NewString("bar"))
	validateList(t, authorizationCache, bob, sets.NewString("foo", "bar", "car"))
	validateList(t, authorizationCache, eve, sets.NewString("bar", "car"))
	validateList(t, authorizationCache, frank, sets.NewString())
}

func TestInvalidateCache(t *testing.T) {
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
					expected: true,
				},
				{
					crs:      []rbacv1.ClusterRole{cr("1")},
					crbs:     []rbacv1.ClusterRoleBinding{crb("a")},
					expected: false,
				},
			},
		},
		{
			name: "clusterrole change",
			trials: []trial{
				{
					crs:      []rbacv1.ClusterRole{cr("1")},
					expected: true,
				},
				{
					crs:      []rbacv1.ClusterRole{cr("2")},
					expected: true,
				},
			},
		},
		{
			name: "clusterrolebinding change",
			trials: []trial{
				{
					crbs:     []rbacv1.ClusterRoleBinding{crb("a")},
					expected: true,
				},
				{
					crbs:     []rbacv1.ClusterRoleBinding{crb("b")},
					expected: true,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			crs := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			crbs := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})

			ac := &AuthorizationCache{
				clusterRoleLister:        syncedClusterRoleLister{ClusterRoleLister: rbacv1listers.NewClusterRoleLister(crs)},
				clusterRoleBindingLister: syncedClusterRoleBindingLister{ClusterRoleBindingLister: rbacv1listers.NewClusterRoleBindingLister(crbs)},
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

					actual := ac.invalidateCache(false)

					if actual != trial.expected {
						t.Errorf("expected %t on trial %d of %d, got %t", trial.expected, i+1, len(tc.trials), actual)
					}
				}()
			}
		})
	}
}

func TestAuthorizationCache_cacheHasExpired(t *testing.T) {
	for _, tt := range []struct {
		name             string
		clock            clock.PassiveClock
		maxCacheLifespan time.Duration
		expected         bool
	}{
		{
			name:             "no time elapsed and zero lifespan",
			clock:            &mockedClock{},
			maxCacheLifespan: 0,
			expected:         false,
		},
		{
			name:             "some time has elapsed but not enough",
			clock:            &mockedClock{since: time.Minute},
			maxCacheLifespan: 2 * time.Minute,
			expected:         false,
		},
		{
			name:             "exactly at max lifespan",
			clock:            &mockedClock{since: time.Minute},
			maxCacheLifespan: time.Minute,
			expected:         false,
		},
		{
			name:             "more than enough time has elapsed",
			clock:            &mockedClock{since: time.Minute},
			maxCacheLifespan: time.Minute - time.Second,
			expected:         true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ac := AuthorizationCache{
				clock:            tt.clock,
				maxCacheLifespan: tt.maxCacheLifespan,
			}
			if result := ac.cacheHasExpired(); result != tt.expected {
				t.Errorf("expected %t, got %t", tt.expected, result)
			}
		})
	}
}

func TestAuthorizationCache_invalidateCache(t *testing.T) {
	for _, tt := range []struct {
		name                     string
		expired                  bool
		expected                 bool
		extraClusterRoles        []*rbacv1.ClusterRole
		extraClusterRoleBindings []*rbacv1.ClusterRoleBinding
	}{
		{
			name:     "zero time elapsed",
			expired:  false,
			expected: false,
		},
		{
			name:     "expired cache",
			expired:  true,
			expected: true,
		},
		{
			name:     "zero time elapsed but extra clusterrole",
			expired:  false,
			expected: true,
			extraClusterRoles: []*rbacv1.ClusterRole{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "clusterrole", ResourceVersion: "rv",
					},
				},
			},
		},
		{
			name:     "zero time elapsed but extra clusterrolebinding",
			expired:  false,
			expected: true,
			extraClusterRoleBindings: []*rbacv1.ClusterRoleBinding{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "clusterrolebinding", ResourceVersion: "rv",
					},
				},
			},
		},
		{
			name:     "expired cache with extra clusterrole",
			expired:  true,
			expected: true,
			extraClusterRoles: []*rbacv1.ClusterRole{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "clusterrole", ResourceVersion: "rv",
					},
				},
			},
		},
		{
			name:     "expired cache with extra clusterrolebindings",
			expired:  true,
			expected: true,
			extraClusterRoleBindings: []*rbacv1.ClusterRoleBinding{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "clusterrole", ResourceVersion: "rv",
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			crs := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			crbs := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})

			ac := &AuthorizationCache{
				clusterRoleLister: syncedClusterRoleLister{
					ClusterRoleLister: rbacv1listers.NewClusterRoleLister(crs),
				},
				clusterRoleBindingLister: syncedClusterRoleBindingLister{
					ClusterRoleBindingLister: rbacv1listers.NewClusterRoleBindingLister(crbs),
				},
			}

			for _, cr := range tt.extraClusterRoles {
				crs.Add(cr)
			}

			for _, crb := range tt.extraClusterRoleBindings {
				crbs.Add(crb)
			}

			if result := ac.invalidateCache(tt.expired); result != tt.expected {
				t.Errorf("expected %t, got %t", tt.expected, result)
			}
		})
	}
}
