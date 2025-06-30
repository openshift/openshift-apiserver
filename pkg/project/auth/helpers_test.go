package auth

import (
	"fmt"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/user"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
)

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

type mockReview struct {
	users  []string
	groups []string
	err    string
}

func (r *mockReview) Users() []string {
	return r.users
}

func (r *mockReview) Groups() []string {
	return r.groups
}

func (r *mockReview) EvaluationError() string {
	return r.err
}

type mockReviewer struct {
	expectedResults map[string]*mockReview
	mutex           sync.RWMutex
}

type mockWatcher struct {
	notifications []watcherNotification
	mutex         sync.Mutex
}

type watcherNotification struct {
	namespace string
	users     sets.String
	groups    sets.String
}

func newMockReviewer() *mockReviewer {
	return &mockReviewer{expectedResults: make(map[string]*mockReview)}
}

func newMockWatcher() *mockWatcher {
	return &mockWatcher{notifications: make([]watcherNotification, 0)}
}

func (mr *mockReviewer) Review(name string) (Review, error) {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()

	review := mr.expectedResults[name]
	if review == nil {
		return nil, fmt.Errorf("Item %s does not exist", name)
	}
	return review, nil
}

func (mr *mockReviewer) setReview(namespace string, users, groups []string) {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()
	mr.expectedResults[namespace] = &mockReview{users: users, groups: groups}
}

func (mw *mockWatcher) GroupMembershipChanged(namespace string, users, groups sets.String) {
	mw.mutex.Lock()
	defer mw.mutex.Unlock()
	mw.notifications = append(mw.notifications, watcherNotification{
		namespace: namespace,
		users:     users,
		groups:    groups,
	})
}

func (mw *mockWatcher) getNotifications() []watcherNotification {
	mw.mutex.Lock()
	defer mw.mutex.Unlock()
	result := make([]watcherNotification, len(mw.notifications))
	copy(result, mw.notifications)
	return result
}

func (mw *mockWatcher) clearNotifications() {
	mw.mutex.Lock()
	defer mw.mutex.Unlock()
	mw.notifications = []watcherNotification{}
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

func extractNamespaceNames(namespaces []corev1.Namespace) []string {
	names := make([]string, len(namespaces))
	for i, ns := range namespaces {
		names[i] = ns.Name
	}
	return names
}

type mockClusterRoleLister struct {
	rbacv1listers.ClusterRoleLister
	resourceVersion string
	clusterRoles    []*rbacv1.ClusterRole
}

func (m *mockClusterRoleLister) LastSyncResourceVersion() string {
	return m.resourceVersion
}

func (m *mockClusterRoleLister) List(selector labels.Selector) ([]*rbacv1.ClusterRole, error) {
	if m.clusterRoles == nil {
		return []*rbacv1.ClusterRole{}, nil
	}
	return m.clusterRoles, nil
}

func (m *mockClusterRoleLister) Get(name string) (*rbacv1.ClusterRole, error) {
	for _, cr := range m.clusterRoles {
		if cr.Name == name {
			return cr, nil
		}
	}
	return nil, fmt.Errorf("cluster role %s not found", name)
}

type mockClusterRoleBindingLister struct {
	rbacv1listers.ClusterRoleBindingLister
	resourceVersion     string
	clusterRoleBindings []*rbacv1.ClusterRoleBinding
}

func (m *mockClusterRoleBindingLister) LastSyncResourceVersion() string {
	return m.resourceVersion
}

func (m *mockClusterRoleBindingLister) List(selector labels.Selector) ([]*rbacv1.ClusterRoleBinding, error) {
	if m.clusterRoleBindings == nil {
		return []*rbacv1.ClusterRoleBinding{}, nil
	}
	return m.clusterRoleBindings, nil
}

func (m *mockClusterRoleBindingLister) Get(name string) (*rbacv1.ClusterRoleBinding, error) {
	for _, crb := range m.clusterRoleBindings {
		if crb.Name == name {
			return crb, nil
		}
	}
	return nil, fmt.Errorf("cluster role binding %s not found", name)
}

// dynamicVersioner increments the version each time it's accessed
type dynamicVersioner struct {
	version int
}

func (d *dynamicVersioner) LastSyncResourceVersion() string {
	d.version++
	return fmt.Sprintf("%d", d.version)
}
