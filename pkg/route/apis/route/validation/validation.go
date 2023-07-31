package validation

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation/field"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	routev1 "github.com/openshift/api/route/v1"
	routecommon "github.com/openshift/library-go/pkg/route"
	routevalidation "github.com/openshift/library-go/pkg/route/validation"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
	routev1conversion "github.com/openshift/openshift-apiserver/pkg/route/apis/route/v1"
)

func toRouteV1(internal *routeapi.Route) (*routev1.Route, field.ErrorList) {
	var external routev1.Route
	if err := routev1conversion.Convert_route_Route_To_v1_Route(internal, &external, nil); err != nil {
		return nil, field.ErrorList{field.InternalError(field.NewPath(""), err)}
	}
	return &external, nil
}

// ValidateRoute tests if required fields in the route are set.
func ValidateRoute(ctx context.Context, route *routeapi.Route, sarc routecommon.SubjectAccessReviewCreator, secrets corev1client.SecretsGetter, opts routecommon.RouteValidationOptions) field.ErrorList {
	external, errs := toRouteV1(route)
	if len(errs) > 0 {
		return errs
	}

	return routevalidation.ValidateRoute(ctx, external, sarc, secrets, opts)
}

func ValidateRouteUpdate(ctx context.Context, route *routeapi.Route, oldRoute *routeapi.Route, sarc routecommon.SubjectAccessReviewCreator, secrets corev1client.SecretsGetter, opts routecommon.RouteValidationOptions) field.ErrorList {
	external, errs := toRouteV1(route)
	if len(errs) > 0 {
		return errs
	}

	oldExternal, errs := toRouteV1(oldRoute)
	if len(errs) > 0 {
		return errs
	}

	return routevalidation.ValidateRouteUpdate(ctx, external, oldExternal, sarc, secrets, opts)
}

// ValidateRouteStatusUpdate validates status updates for routes.
//
// Note that this function shouldn't call ValidateRouteUpdate, otherwise
// we are risking to break existing routes.
func ValidateRouteStatusUpdate(route *routeapi.Route, oldRoute *routeapi.Route) field.ErrorList {
	external, errs := toRouteV1(route)
	if len(errs) > 0 {
		return errs
	}

	oldExternal, errs := toRouteV1(oldRoute)
	if len(errs) > 0 {
		return errs
	}

	return routevalidation.ValidateRouteStatusUpdate(external, oldExternal)
}

func Warnings(route *routeapi.Route) []string {
	external, errs := toRouteV1(route)
	if len(errs) > 0 {
		return nil
	}

	return routevalidation.Warnings(external)
}
