package auth

import (
	"time"
)

// AuthorizationCacheInterface defines the common interface for both V1 and V2 authorization caches.
type AuthorizationCache interface {
	// Lister provides namespace listing functionality based on user permissions
	Lister

	// Run begins watching and synchronizing the cache with the specified period
	Run(period time.Duration)

	// ReadyForAccess returns true when the cache is ready to serve requests
	ReadyForAccess() bool

	// AddWatcher adds a cache watcher that will be notified of permission changes
	AddWatcher(watcher CacheWatcher)

	// RemoveWatcher removes a cache watcher
	RemoveWatcher(watcher CacheWatcher)

	// GetClusterRoleLister returns the cluster role lister for scope validation
	GetClusterRoleLister() SyncedClusterRoleLister
}
