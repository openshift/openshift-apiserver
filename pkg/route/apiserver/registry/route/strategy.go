package route

import (
	"context"
	"fmt"

	authorizationapi "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/kubernetes/pkg/api/legacyscheme"

	routev1 "github.com/openshift/api/route/v1"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
	"github.com/openshift/openshift-apiserver/pkg/route/apis/route/validation"
	"github.com/openshift/openshift-apiserver/pkg/route/apiserver/admission/routehostassignment"
)

// Registry is an interface for performing subject access reviews
type SubjectAccessReviewInterface interface {
	Create(ctx context.Context, sar *authorizationapi.SubjectAccessReview, opts metav1.CreateOptions) (result *authorizationapi.SubjectAccessReview, err error)
}

var _ SubjectAccessReviewInterface = authorizationclient.SubjectAccessReviewInterface(nil)

type HostnameGenerator interface {
	GenerateHostname(*routev1.Route) (string, error)
}

type routeStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
	hostnameGenerator HostnameGenerator
	sarClient         SubjectAccessReviewInterface
}

// NewStrategy initializes the default logic that applies when creating and updating
// Route objects via the REST API.
func NewStrategy(allocator HostnameGenerator, sarClient SubjectAccessReviewInterface) routeStrategy {
	return routeStrategy{
		ObjectTyper:       legacyscheme.Scheme,
		NameGenerator:     names.SimpleNameGenerator,
		hostnameGenerator: allocator,
		sarClient:         sarClient,
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
	errs := routehostassignment.AllocateHost(ctx, route, s.sarClient, s.hostnameGenerator)
	errs = append(errs, validation.ValidateRoute(route)...)
	return errs
}

// WarningsOnCreate returns warnings for the creation of the given object.
func (routeStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return validation.Warnings(obj.(*routeapi.Route))
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
	errs := routehostassignment.ValidateHostUpdate(ctx, objRoute, oldRoute, s.sarClient)
	errs = append(errs, validation.ValidateRouteUpdate(objRoute, oldRoute)...)
	return errs
}

// WarningsOnUpdate returns warnings for the given update.
func (routeStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return validation.Warnings(obj.(*routeapi.Route))
}

func (routeStrategy) AllowUnconditionalUpdate() bool {
	return false
}

type routeStatusStrategy struct {
	routeStrategy
}

var StatusStrategy = routeStatusStrategy{NewStrategy(nil, nil)}

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
