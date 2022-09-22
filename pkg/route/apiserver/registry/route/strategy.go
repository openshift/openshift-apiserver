package route

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/kubernetes/pkg/api/legacyscheme"

	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
	"github.com/openshift/openshift-apiserver/pkg/route/apis/route/validation"
)

type routeStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

// NewStrategy initializes the default logic that applies when creating and updating
// Route objects via the REST API.
func NewStrategy() routeStrategy {
	return routeStrategy{
		ObjectTyper:   legacyscheme.Scheme,
		NameGenerator: names.SimpleNameGenerator,
	}
}

func (routeStrategy) NamespaceScoped() bool {
	return true
}

func (s routeStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	route := obj.(*routeapi.Route)
	route.Status = routeapi.RouteStatus{}
	stripEmptyDestinationCACertificate(route)
}

func (s routeStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	route := obj.(*routeapi.Route)
	oldRoute := old.(*routeapi.Route)

	route.Status = oldRoute.Status
	stripEmptyDestinationCACertificate(route)
	// Ignore attempts to clear the spec Host
	// Prevents "immutable field" errors when applying the same route definition used to create
	if len(route.Spec.Host) == 0 {
		route.Spec.Host = oldRoute.Spec.Host
	}
}

func (s routeStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	route := obj.(*routeapi.Route)
	return validation.ValidateRoute(route)
}

// WarningsOnCreate returns warnings for the creation of the given object.
func (routeStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return hostAndSubdomainBothSetWarning(obj)
}

func (routeStrategy) AllowCreateOnUpdate() bool {
	return false
}

// Canonicalize normalizes the object after validation.
func (routeStrategy) Canonicalize(obj runtime.Object) {
}

func (s routeStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	oldRoute := old.(*routeapi.Route)
	objRoute := obj.(*routeapi.Route)
	return validation.ValidateRouteUpdate(objRoute, oldRoute)
}

func hasCertificateInfo(tls *routeapi.TLSConfig) bool {
	if tls == nil {
		return false
	}
	return len(tls.Certificate) > 0 ||
		len(tls.Key) > 0 ||
		len(tls.CACertificate) > 0 ||
		len(tls.DestinationCACertificate) > 0
}

func certificateChangeRequiresAuth(route, older *routeapi.Route) bool {
	switch {
	case route.Spec.TLS != nil && older.Spec.TLS != nil:
		a, b := route.Spec.TLS, older.Spec.TLS
		if !hasCertificateInfo(a) {
			// removing certificate info is allowed
			return false
		}
		return a.CACertificate != b.CACertificate ||
			a.Certificate != b.Certificate ||
			a.DestinationCACertificate != b.DestinationCACertificate ||
			a.Key != b.Key
	case route.Spec.TLS != nil:
		// using any default certificate is allowed
		return hasCertificateInfo(route.Spec.TLS)
	default:
		// all other cases we are not adding additional certificate info
		return false
	}
}

// WarningsOnUpdate returns warnings for the given update.
func (routeStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return hostAndSubdomainBothSetWarning(obj)
}

func (routeStrategy) AllowUnconditionalUpdate() bool {
	return false
}

type routeStatusStrategy struct {
	routeStrategy
}

var StatusStrategy = routeStatusStrategy{NewStrategy()}

func (routeStatusStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	newRoute := obj.(*routeapi.Route)
	oldRoute := old.(*routeapi.Route)
	newRoute.Spec = oldRoute.Spec
}

func (routeStatusStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return validation.ValidateRouteStatusUpdate(obj.(*routeapi.Route), old.(*routeapi.Route))
}

const emptyDestinationCertificate = `-----BEGIN COMMENT-----
This is an empty PEM file created to provide backwards compatibility
for reencrypt routes that have no destinationCACertificate. This 
content will only appear for routes accessed via /oapi/v1/routes.
-----END COMMENT-----
`

// stripEmptyDestinationCACertificate removes the empty destinationCACertificate if it matches
// the current route destination CA certificate.
func stripEmptyDestinationCACertificate(route *routeapi.Route) {
	tls := route.Spec.TLS
	if tls == nil || tls.Termination != routeapi.TLSTerminationReencrypt {
		return
	}
	if tls.DestinationCACertificate == emptyDestinationCertificate {
		tls.DestinationCACertificate = ""
	}
}

// DecorateLegacyRouteWithEmptyDestinationCACertificates is used for /oapi/v1 route endpoints
// to prevent legacy clients from seeing an empty destination CA certificate for reencrypt routes,
// which the 'route.openshift.io/v1' endpoint allows. These values are injected in REST responses
// and stripped in PrepareForCreate and PrepareForUpdate.
func DecorateLegacyRouteWithEmptyDestinationCACertificates(obj runtime.Object) error {
	switch t := obj.(type) {
	case *routeapi.Route:
		tls := t.Spec.TLS
		if tls == nil || tls.Termination != routeapi.TLSTerminationReencrypt {
			return nil
		}
		if len(tls.DestinationCACertificate) == 0 {
			tls.DestinationCACertificate = emptyDestinationCertificate
		}
		return nil
	case *routeapi.RouteList:
		for i := range t.Items {
			tls := t.Items[i].Spec.TLS
			if tls == nil || tls.Termination != routeapi.TLSTerminationReencrypt {
				continue
			}
			if len(tls.DestinationCACertificate) == 0 {
				tls.DestinationCACertificate = emptyDestinationCertificate
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown type passed to %T", obj)
	}
}

// hostAndSubdomainBothSetWarning returns a warning if a route has both
// spec.host and spec.subdomain set.
func hostAndSubdomainBothSetWarning(obj runtime.Object) []string {
	newRoute := obj.(*routeapi.Route)
	if len(newRoute.Spec.Host) != 0 && len(newRoute.Spec.Subdomain) != 0 {
		var warnings []string
		warnings = append(warnings, "spec.host is set; spec.subdomain may be ignored")
		return warnings
	}
	return nil
}
