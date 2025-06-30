package auth

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/user"
	rbacv1informers "k8s.io/client-go/informers/rbac/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"

	"github.com/openshift/apiserver-library-go/pkg/authorization/scope"
	authorizationapi "github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization"
)

// NewAuthorizationCacheV2 creates a new authorization cache using a hash-based architecture
// (as opposed to the original AuthorizationCache based on resource versions).
func NewAuthorizationCacheV2(
	namespaceLister corev1listers.NamespaceLister,
	reviewer Reviewer,
	informers rbacv1informers.Interface,
) (*AuthorizationCacheV2, error) {
	scrLister := syncedClusterRoleLister{
		informers.ClusterRoles().Lister(),
		informers.ClusterRoles().Informer(),
	}
	scrbLister := syncedClusterRoleBindingLister{
		informers.ClusterRoleBindings().Lister(),
		informers.ClusterRoleBindings().Informer(),
	}
	srLister := syncedRoleLister{
		informers.Roles().Lister(),
		informers.Roles().Informer(),
	}
	srbLister := syncedRoleBindingLister{
		informers.RoleBindings().Lister(),
		informers.RoleBindings().Informer(),
	}

	globalRBACCache := NewGlobalRBACCache(scrLister, scrbLister)

	ac := &AuthorizationCacheV2{
		namespaceLister: namespaceLister,
		reviewer:        reviewer,

		globalRBACCache:   globalRBACCache,
		roleLister:        srLister,
		roleBindingLister: srbLister,

		namespaceHashes:     make(map[string]string),
		namespaceAccessData: make(map[string]*accessRecord),
		userToNamespaces:    make(map[string]sets.Set[string]),
		groupToNamespaces:   make(map[string]sets.Set[string]),

		ready:    false,
		watchers: []CacheWatcher{},
	}

	if err := ac.validateConfiguration(); err != nil {
		return nil, err
	}

	return ac, nil
}

// AuthorizationCacheV2 maintains a hash-based cache on the set of namespaces a user or group can access
type AuthorizationCacheV2 struct {
	// Core dependencies
	namespaceLister corev1listers.NamespaceLister
	reviewer        Reviewer

	// Global RBAC cache for cluster-scoped resources
	globalRBACCache *GlobalRBACCache

	// RBAC listers for namespace-scoped resources
	roleLister        SyncedRoleLister
	roleBindingLister SyncedRoleBindingLister

	// Namespace change tracking
	namespaceHashes     map[string]string        // namespace -> computed hash
	namespaceAccessData map[string]*accessRecord // namespace -> cached access data
	hashLock            sync.RWMutex

	// Subject-to-namespace mappings
	userToNamespaces  map[string]sets.Set[string] // user -> set of accessible namespaces
	groupToNamespaces map[string]sets.Set[string] // group -> set of accessible namespaces
	subjectLock       sync.RWMutex

	// Synchronization state
	ready     bool
	readyLock sync.RWMutex
	syncLock  sync.Mutex

	// Watchers
	watchers    []CacheWatcher
	watcherLock sync.RWMutex
}

// Regarding locks:
// When both are necessary, hashLock should be acquired before subjectLock to avoid deadlocks.

// accessRecord holds the cached access information for a namespace
type accessRecord struct {
	users        []string  // users with access to this namespace
	groups       []string  // groups with access to this namespace
	lastReviewed time.Time // when this was last computed
	computedHash string    // the hash that was used to compute this access
}

// rbacResourceRef represents an RBAC resource for hashing purposes
type rbacResourceRef struct {
	uid             string
	resourceVersion string
	kind            string
}

// Run begins watching and synchronizing the cache
func (ac *AuthorizationCacheV2) Run(period time.Duration) {
	go func() {
		err := ac.synchronize()
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to synchronize cache: %v", err))
		}

		ticker := time.NewTicker(period)
		defer ticker.Stop()

		for range ticker.C {
			err := ac.synchronize()
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("failed to synchronize cache: %v", err))
			}
		}
	}()
}

// AddWatcher adds a cache watcher that will be notified of permission changes
func (ac *AuthorizationCacheV2) AddWatcher(watcher CacheWatcher) {
	ac.watcherLock.Lock()
	defer ac.watcherLock.Unlock()

	ac.watchers = append(ac.watchers, watcher)
}

// RemoveWatcher removes a cache watcher
func (ac *AuthorizationCacheV2) RemoveWatcher(watcher CacheWatcher) {
	ac.watcherLock.Lock()
	defer ac.watcherLock.Unlock()

	for i, w := range ac.watchers {
		if w == watcher {
			ac.watchers[i] = ac.watchers[len(ac.watchers)-1]
			ac.watchers = ac.watchers[:len(ac.watchers)-1]
			break
		}
	}
}

// GetClusterRoleLister returns the cluster role lister
func (ac *AuthorizationCacheV2) GetClusterRoleLister() SyncedClusterRoleLister {
	return ac.globalRBACCache.GetClusterRoleLister()
}

// List returns the set of namespaces the user has access to view
func (ac *AuthorizationCacheV2) List(userInfo user.Info, selector labels.Selector) (*corev1.NamespaceList, error) {
	userNamespaces, err := ac.getUserNamespaces(userInfo.GetName())
	if err != nil {
		return nil, err
	}

	allNamespaces := sets.NewString(userNamespaces...)
	for _, group := range userInfo.GetGroups() {
		groupNamespaces, err := ac.getGroupNamespaces(group)
		if err != nil {
			return nil, err
		}
		allNamespaces.Insert(groupNamespaces...)
	}

	// Apply scope restrictions
	allowedNamespaces, err := scope.ScopesToVisibleNamespaces(
		userInfo.GetExtra()[authorizationapi.ScopesKey],
		ac.globalRBACCache.GetClusterRoleLister(),
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compute visible namespaces for user %q: %v", userInfo.GetName(), err)
	}

	namespaceList := &corev1.NamespaceList{}
	for _, nsName := range allNamespaces.List() {
		if !allowedNamespaces.Has("*") && !allowedNamespaces.Has(nsName) {
			continue
		}

		namespace, err := ac.namespaceLister.Get(nsName)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
			// Namespace was deleted, skip it
			continue
		}

		if !selector.Matches(labels.Set(namespace.Labels)) {
			continue
		}

		namespaceList.Items = append(namespaceList.Items, *namespace)
	}

	return namespaceList, nil
}

// ReadyForAccess returns true when the cache is ready to serve requests
func (ac *AuthorizationCacheV2) ReadyForAccess() bool {
	ac.readyLock.RLock()
	defer ac.readyLock.RUnlock()
	return ac.ready
}

// Internal functions

// synchronize is responsible for the synchronization of the cache both initially and periodically
func (ac *AuthorizationCacheV2) synchronize() error {
	if !ac.syncLock.TryLock() {
		// Another sync is in progress, skip this one (do not attempt to sync concurrently or queue)
		return fmt.Errorf("sync is already in progress")
	}
	defer ac.syncLock.Unlock()

	skip, namespacesToUpdate := ac.shouldSkipSync()
	if skip {
		return nil
	}

	if namespacesToUpdate != nil {
		// Partial sync: only update specific namespaces
		for ns, hash := range namespacesToUpdate {
			if err := ac.refreshNamespaceWithHash(ns, hash); err != nil {
				// Log error but continue with other namespaces
				utilruntime.HandleError(fmt.Errorf("failed to refresh namespace %s: %v", ns, err))
			}
		}
	} else {
		// Full sync: global RBAC changed, hash has already been computed and cached
		if err := ac.synchronizeAllNamespaces(); err != nil {
			return fmt.Errorf("full sync failed: %v", err)
		}
	}

	ac.readyLock.Lock()
	defer ac.readyLock.Unlock()
	ac.ready = true

	return nil
}

// synchronizeNamespacesWithGlobalHash processes all namespaces using a precomputed global RBAC hash
func (ac *AuthorizationCacheV2) synchronizeAllNamespaces() error {
	namespaces, err := ac.namespaceLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %v", err)
	}

	for _, ns := range namespaces {
		currentHash, err := ac.computeNamespaceHash(ns.Name)
		if err != nil {
			return fmt.Errorf("failed to compute hash for namespace %s: %v", ns.Name, err)
		}

		// no further logic needed here because refreshNamespaceWithHash will skip if the hash is unchanged
		if err := ac.refreshNamespaceWithHash(ns.Name, currentHash); err != nil {
			return fmt.Errorf("failed to refresh namespace %s: %v", ns.Name, err)
		}
	}

	// Clean up any namespaces that no longer exist
	ac.cleanupDeletedNamespaces(namespaces)

	return nil
}

// refreshNamespace updates the cache for a specific namespace in the absence of a precomputed hash
func (ac *AuthorizationCacheV2) refreshNamespace(namespace string) error {
	currentHash, err := ac.computeNamespaceHash(namespace)
	if err != nil {
		return fmt.Errorf("failed to compute hash for namespace %s: %v", namespace, err)
	}

	return ac.refreshNamespaceWithHash(namespace, currentHash)
}

// refreshNamespaceWithHash updates the cache for a specific namespace using a precomputed hash
func (ac *AuthorizationCacheV2) refreshNamespaceWithHash(namespace string, currentHash string) error {
	// If hash is empty, check if this is a deleted namespace
	if currentHash == "" {
		_, err := ac.namespaceLister.Get(namespace)
		if apierrors.IsNotFound(err) {
			// Namespace was deleted - handle deletion instead of refresh
			return ac.handleNamespaceDeletion(namespace)
		}
		// Namespace exists but we were erroneously passed an empty hash, fall back to full refresh
		return ac.refreshNamespace(namespace)
	}

	ac.hashLock.RLock()
	cachedHash, exists := ac.namespaceHashes[namespace]
	ac.hashLock.RUnlock()

	if exists && cachedHash == currentHash {
		// No change, skip refresh
		return nil
	}

	// Get current access data & perform access review

	oldAccess := ac.getNamespaceAccess(namespace)

	review, err := ac.reviewer.Review(namespace)
	if err != nil {
		return fmt.Errorf("access review failed for namespace %s: %v", namespace, err)
	}

	newUsers := review.Users()
	newGroups := review.Groups()

	newAccess := &accessRecord{
		users:        newUsers,
		groups:       newGroups,
		lastReviewed: time.Now(),
		computedHash: currentHash,
	}

	ac.updateNamespaceCache(namespace, currentHash, oldAccess, newAccess)

	// Notify watchers of changes
	var oldUserSet, oldGroupSet sets.Set[string]
	if oldAccess != nil {
		oldUserSet = sets.New(oldAccess.users...)
		oldGroupSet = sets.New(oldAccess.groups...)
	}

	newUserSet := sets.New(newUsers...)
	newGroupSet := sets.New(newGroups...)

	ac.notifyWatchersV2(namespace, oldUserSet, newUserSet, oldGroupSet, newGroupSet)

	return nil
}

// updateNamespaceCache updates all cache structures for a namespace
func (ac *AuthorizationCacheV2) updateNamespaceCache(namespace, hash string, oldAccess, newAccess *accessRecord) {
	ac.hashLock.Lock()
	ac.namespaceHashes[namespace] = hash
	ac.namespaceAccessData[namespace] = newAccess
	ac.hashLock.Unlock()

	ac.subjectLock.Lock()
	defer ac.subjectLock.Unlock()

	// Remove old mappings
	if oldAccess != nil {
		for _, user := range oldAccess.users {
			if namespaces, exists := ac.userToNamespaces[user]; exists {
				namespaces.Delete(namespace)
				if namespaces.Len() == 0 {
					delete(ac.userToNamespaces, user)
				}
			}
		}
		for _, group := range oldAccess.groups {
			if namespaces, exists := ac.groupToNamespaces[group]; exists {
				namespaces.Delete(namespace)
				if namespaces.Len() == 0 {
					delete(ac.groupToNamespaces, group)
				}
			}
		}
	}

	// Add new mappings
	for _, user := range newAccess.users {
		if _, exists := ac.userToNamespaces[user]; !exists {
			ac.userToNamespaces[user] = sets.New[string]()
		}
		ac.userToNamespaces[user].Insert(namespace)
	}
	for _, group := range newAccess.groups {
		if _, exists := ac.groupToNamespaces[group]; !exists {
			ac.groupToNamespaces[group] = sets.New[string]()
		}
		ac.groupToNamespaces[group].Insert(namespace)
	}
}

// cleanupDeletedNamespaces removes namespaces that no longer exist
func (ac *AuthorizationCacheV2) cleanupDeletedNamespaces(existingNamespaces []*corev1.Namespace) {
	existingNames := sets.New[string]()
	for _, ns := range existingNamespaces {
		existingNames.Insert(ns.Name)
	}

	ac.hashLock.RLock()
	var namespacesToDelete []string
	for cachedNS := range ac.namespaceHashes {
		if !existingNames.Has(cachedNS) {
			namespacesToDelete = append(namespacesToDelete, cachedNS)
		}
	}
	ac.hashLock.RUnlock()

	for _, ns := range namespacesToDelete {
		if err := ac.handleNamespaceDeletion(ns); err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to handle deletion of namespace %s: %v", ns, err))
		}
	}
}

// getNamespaceAccess retrieves the current access record for a namespace
func (ac *AuthorizationCacheV2) getNamespaceAccess(namespace string) *accessRecord {
	ac.hashLock.RLock()
	defer ac.hashLock.RUnlock()

	if access, exists := ac.namespaceAccessData[namespace]; exists {
		// Return a copy to avoid any race conditions/concurrency issues
		return &accessRecord{
			users:        append([]string(nil), access.users...),
			groups:       append([]string(nil), access.groups...),
			lastReviewed: access.lastReviewed,
			computedHash: access.computedHash,
		}
	}
	return nil
}

func (ac *AuthorizationCacheV2) notifyWatchersV2(namespace string, oldUsers, newUsers, oldGroups, newGroups sets.Set[string]) {
	ac.watcherLock.RLock()
	watchers := make([]CacheWatcher, len(ac.watchers))
	copy(watchers, ac.watchers)
	ac.watcherLock.RUnlock()

	// Only notify if there are actual changes
	if !oldUsers.Equal(newUsers) || !oldGroups.Equal(newGroups) {
		for _, watcher := range watchers {
			watcher.GroupMembershipChanged(
				namespace,
				sets.NewString(newUsers.UnsortedList()...),
				sets.NewString(newGroups.UnsortedList()...),
			)
		}
	}
}

// shouldSkipSync checks if the cache can skip synchronization based on RBAC changes.
// Returns: skip (bool), namespacesToUpdate (map of ns name string -> new hash string)
func (ac *AuthorizationCacheV2) shouldSkipSync() (bool, map[string]string) {
	// Check if global RBAC has changed
	skipGlobal, err := ac.globalRBACCache.shouldSkipSyncGlobal()
	if err != nil {
		// We cannot continue without a valid global RBAC hash - log it as an error and don't sync
		utilruntime.HandleError(fmt.Errorf("failed to compute global RBAC hash - sync cannot continue: %v", err))
		return true, nil
	}

	// If global RBAC changed, we need to do a full sync
	if !skipGlobal {
		return false, nil
	}

	// Global RBAC hasn't changed, check namespace-specific changes
	namespaceHashes := ac.findNamespacesWithChanges()

	if len(namespaceHashes) == 0 {
		// No global or namespace changes, can fully skip
		return true, nil
	}

	// Namespace changes detected, need partial sync
	return false, namespaceHashes
}

// findNamespacesWithChanges identifies namespaces whose RBAC hash has changed
// Returns a map of changed namespaces to their computed hashes
func (ac *AuthorizationCacheV2) findNamespacesWithChanges() map[string]string {
	namespaces, err := ac.namespaceLister.List(labels.Everything())
	if err != nil {
		// If we can't list namespaces, assume all need refresh
		return map[string]string{}
	}

	namespaceHashes := make(map[string]string)

	ac.hashLock.RLock()
	cachedHashes := make(map[string]string, len(ac.namespaceHashes))
	for ns, hash := range ac.namespaceHashes {
		cachedHashes[ns] = hash
	}
	ac.hashLock.RUnlock()

	for _, ns := range namespaces {
		currentHash, err := ac.computeNamespaceHash(ns.Name)
		if err != nil {
			// If we can't compute hash, assume it changed
			// Pass empty hash as placeholder - deletion check & recomputation will be attempted later
			namespaceHashes[ns.Name] = ""
			continue
		}

		cachedHash, exists := cachedHashes[ns.Name]
		if !exists || cachedHash != currentHash {
			namespaceHashes[ns.Name] = currentHash
		}
	}

	// Also check for deleted namespaces
	for cachedNS := range cachedHashes {
		found := false
		for _, ns := range namespaces {
			if ns.Name == cachedNS {
				found = true
				break
			}
		}
		if !found {
			// For deleted namespaces, passing empty hash prompts check for deletion & delete handler
			namespaceHashes[cachedNS] = ""
		}
	}

	return namespaceHashes
}

// computeNamespaceHash computes a hash for a specific namespace
// This ensures that namespace-specific roles and bindings are combined with the global RBAC state to ultimately determine access
func (ac *AuthorizationCacheV2) computeNamespaceHash(namespace string) (string, error) {
	var refs []rbacResourceRef

	globalHash := ac.globalRBACCache.GetCurrentHash()

	roles, err := ac.roleLister.Roles(namespace).List(labels.Everything())
	if err != nil {
		return "", fmt.Errorf("failed to list roles in namespace %s: %v", namespace, err)
	}
	for _, role := range roles {
		refs = append(refs, rbacResourceRef{
			uid:             string(role.UID),
			resourceVersion: role.ResourceVersion,
			kind:            "Role",
		})
	}

	roleBindings, err := ac.roleBindingLister.RoleBindings(namespace).List(labels.Everything())
	if err != nil {
		return "", fmt.Errorf("failed to list role bindings in namespace %s: %v", namespace, err)
	}
	for _, rb := range roleBindings {
		refs = append(refs, rbacResourceRef{
			uid:             string(rb.UID),
			resourceVersion: rb.ResourceVersion,
			kind:            "RoleBinding",
		})
	}

	// Combine global hash with namespace-specific hash
	return combineHashes(globalHash, computeHashFromRefs(refs)), nil
}

// computeHashFromRefs computes a deterministic hash from RBAC resource references
func computeHashFromRefs(refs []rbacResourceRef) string {
	// Sort refs to ensure deterministic hash
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].kind != refs[j].kind {
			return refs[i].kind < refs[j].kind
		}
		if refs[i].uid != refs[j].uid {
			return refs[i].uid < refs[j].uid
		}
		return refs[i].resourceVersion < refs[j].resourceVersion
	})

	// FNV-1a is used here for speed and memory efficiency, since cryptographically strong
	// hashes are not really required for change detection in this context
	h := fnv.New64a()
	for _, ref := range refs {
		h.Write([]byte(ref.kind))
		h.Write([]byte(":"))
		h.Write([]byte(ref.uid))
		h.Write([]byte(":"))
		h.Write([]byte(ref.resourceVersion))
		h.Write([]byte("|"))
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// combineHashes combines two hashes into a single hash
func combineHashes(hash1, hash2 string) string {
	h := fnv.New64a()
	h.Write([]byte(hash1))
	h.Write([]byte("|"))
	h.Write([]byte(hash2))
	return fmt.Sprintf("%016x", h.Sum64())
}

func (ac *AuthorizationCacheV2) getUserNamespaces(username string) ([]string, error) {
	ac.subjectLock.RLock()
	defer ac.subjectLock.RUnlock()

	if namespaces, exists := ac.userToNamespaces[username]; exists {
		return namespaces.UnsortedList(), nil
	}
	return []string{}, nil
}

func (ac *AuthorizationCacheV2) getGroupNamespaces(groupname string) ([]string, error) {
	ac.subjectLock.RLock()
	defer ac.subjectLock.RUnlock()

	if namespaces, exists := ac.groupToNamespaces[groupname]; exists {
		return namespaces.UnsortedList(), nil
	}
	return []string{}, nil
}

func (ac *AuthorizationCacheV2) validateConfiguration() error {
	if ac.namespaceLister == nil {
		return fmt.Errorf("namespaceLister is required")
	}
	if ac.reviewer == nil {
		return fmt.Errorf("reviewer is required")
	}
	if ac.roleLister == nil {
		return fmt.Errorf("roleLister is required")
	}
	if ac.roleBindingLister == nil {
		return fmt.Errorf("roleBindingLister is required")
	}
	if ac.globalRBACCache == nil {
		return fmt.Errorf("globalRBACCache not initialized")
	}
	return nil
}

func (ac *AuthorizationCacheV2) handleNamespaceDeletion(namespace string) error {
	oldAccess := ac.getNamespaceAccess(namespace)

	ac.hashLock.Lock()
	delete(ac.namespaceHashes, namespace)
	delete(ac.namespaceAccessData, namespace)
	ac.hashLock.Unlock()

	ac.subjectLock.Lock()
	for user, namespaces := range ac.userToNamespaces {
		namespaces.Delete(namespace)
		if namespaces.Len() == 0 {
			delete(ac.userToNamespaces, user)
		}
	}
	for group, namespaces := range ac.groupToNamespaces {
		namespaces.Delete(namespace)
		if namespaces.Len() == 0 {
			delete(ac.groupToNamespaces, group)
		}
	}
	ac.subjectLock.Unlock()

	// Notify watchers
	oldUserSet := sets.Set[string]{}
	oldGroupSet := sets.Set[string]{}
	if oldAccess != nil {
		oldUserSet = sets.New(oldAccess.users...)
		oldGroupSet = sets.New(oldAccess.groups...)
	}
	ac.notifyWatchersV2(namespace, oldUserSet, sets.Set[string]{}, oldGroupSet, sets.Set[string]{})

	return nil
}
