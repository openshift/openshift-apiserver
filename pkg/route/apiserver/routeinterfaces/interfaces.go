package routeinterfaces

import (
	api "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

// RouteAllocator is the interface for the route allocation controller
// which handles requests for RouterShard allocation and name generation.
type RouteAllocator interface {
	GenerateHostname(*api.Route) string
}
