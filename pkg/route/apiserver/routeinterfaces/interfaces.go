package routeinterfaces

import (
	api "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

// RouteAllocator is the interface for the route allocation controller
// which handles requests for RouterShard allocation and name generation.
type RouteAllocator interface {
	AllocateRouterShard(*api.Route) (*api.RouterShard, error)
	GenerateHostname(*api.Route, *api.RouterShard) string
}
