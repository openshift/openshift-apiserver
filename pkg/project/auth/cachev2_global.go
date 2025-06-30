package auth

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/labels"
)

// GlobalRBACCache manages cluster-scoped RBAC resources and their hashing
type GlobalRBACCache struct {
	// RBAC listers for cluster-scoped resources
	clusterRoleLister        SyncedClusterRoleLister
	clusterRoleBindingLister SyncedClusterRoleBindingLister

	// Global RBAC resource tracking
	globalRBACHash  string
	globalRBACLock  sync.RWMutex
	computeHashLock sync.Mutex

	// Resource version tracking for optimization
	lastClusterRoleVersion        string
	lastClusterRoleBindingVersion string
}

// NewGlobalRBACCache creates a new GlobalRBACCache
func NewGlobalRBACCache(
	clusterRoleLister SyncedClusterRoleLister,
	clusterRoleBindingLister SyncedClusterRoleBindingLister,
) *GlobalRBACCache {
	return &GlobalRBACCache{
		clusterRoleLister:        clusterRoleLister,
		clusterRoleBindingLister: clusterRoleBindingLister,
	}
}

// GetClusterRoleLister returns the cluster role lister
func (g *GlobalRBACCache) GetClusterRoleLister() SyncedClusterRoleLister {
	return g.clusterRoleLister
}

// GetCurrentHash returns the current global RBAC hash
func (g *GlobalRBACCache) GetCurrentHash() string {
	g.globalRBACLock.RLock()
	defer g.globalRBACLock.RUnlock()
	return g.globalRBACHash
}

// ComputeHash computes the hash of all cluster roles and cluster role bindings
func (g *GlobalRBACCache) ComputeHash() (string, error) {
	if !g.computeHashLock.TryLock() {
		// Another sync is in progress, skip this one (do not attempt to sync concurrently or queue)
		return "", fmt.Errorf("hash computation already in progress")
	}
	defer g.computeHashLock.Unlock()

	if !g.hasResourceVersionChanged() && g.GetCurrentHash() != "" {
		return g.GetCurrentHash(), nil
	}

	// Resource versions have changed, so we need to list and hash
	g.globalRBACLock.Lock()
	defer g.globalRBACLock.Unlock()

	refs, err := g.listClusterRBACRefs()
	if err != nil {
		return "", err
	}

	newHash := computeHashFromRefs(refs)

	g.globalRBACHash = newHash
	g.updateResourceVersions()

	return newHash, nil
}

// shouldSkipSyncGlobal checks if global RBAC has changed and determines if a full sync is needed.
func (g *GlobalRBACCache) shouldSkipSyncGlobal() (bool, error) {
	if g.GetCurrentHash() != "" && !g.hasResourceVersionChanged() {
		return true, nil
	}

	lastGlobalHash := g.GetCurrentHash()

	currentGlobalHash, err := g.ComputeHash()
	if err != nil {
		return false, err
	}

	return lastGlobalHash == currentGlobalHash, nil
}

func (g *GlobalRBACCache) hasResourceVersionChanged() bool {
	g.globalRBACLock.RLock()
	defer g.globalRBACLock.RUnlock()

	currentClusterRoleVersion := g.clusterRoleLister.LastSyncResourceVersion()
	currentClusterRoleBindingVersion := g.clusterRoleBindingLister.LastSyncResourceVersion()

	// If we don't have previous versions cached, consider it changed
	if g.lastClusterRoleVersion == "" || g.lastClusterRoleBindingVersion == "" {
		return true
	}

	return currentClusterRoleVersion != g.lastClusterRoleVersion || currentClusterRoleBindingVersion != g.lastClusterRoleBindingVersion
}

// updateResourceVersions expects to be called by a caller that has already acquired the globalRBACLock write lock
// i.e. it is a step within an update operation, not a discrete operation by itself
func (g *GlobalRBACCache) updateResourceVersions() {
	currentClusterRoleVersion := g.clusterRoleLister.LastSyncResourceVersion()
	currentClusterRoleBindingVersion := g.clusterRoleBindingLister.LastSyncResourceVersion()

	// Only update if we got valid resource versions
	if currentClusterRoleVersion != "" {
		g.lastClusterRoleVersion = currentClusterRoleVersion
	}
	if currentClusterRoleBindingVersion != "" {
		g.lastClusterRoleBindingVersion = currentClusterRoleBindingVersion
	}
}

func (g *GlobalRBACCache) listClusterRBACRefs() ([]rbacResourceRef, error) {
	var refs []rbacResourceRef

	clusterRoles, err := g.clusterRoleLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster roles: %v", err)
	}
	for _, cr := range clusterRoles {
		refs = append(refs, rbacResourceRef{
			uid:             string(cr.UID),
			resourceVersion: cr.ResourceVersion,
			kind:            "ClusterRole",
		})
	}

	clusterRoleBindings, err := g.clusterRoleBindingLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster role bindings: %v", err)
	}
	for _, crb := range clusterRoleBindings {
		refs = append(refs, rbacResourceRef{
			uid:             string(crb.UID),
			resourceVersion: crb.ResourceVersion,
			kind:            "ClusterRoleBinding",
		})
	}

	return refs, nil
}
