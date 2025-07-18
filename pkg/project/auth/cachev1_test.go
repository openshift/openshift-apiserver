package auth

import (
	"fmt"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/controller"
)

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

	authorizationCache := NewAuthorizationCacheV1(
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

			ac := &AuthorizationCacheV1{
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

					actual := ac.invalidateCache()

					if actual != trial.expected {
						t.Errorf("expected %t on trial %d of %d, got %t", trial.expected, i+1, len(tc.trials), actual)
					}
				}()
			}
		})
	}
}
