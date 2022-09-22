package routehostassignment

import (
	"context"
	"fmt"
	"io"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/initializer"
	"k8s.io/client-go/kubernetes"

	routegroup "github.com/openshift/api/route"
	routev1 "github.com/openshift/api/route/v1"
	routeinternal "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
	routev1conversion "github.com/openshift/openshift-apiserver/pkg/route/apis/route/v1"
)

const (
	pluginName = "route.openshift.io/RouteHostAssignment"
)

func Register(plugins *admission.Plugins) {
	plugins.Register(pluginName,
		func(_ io.Reader) (admission.Interface, error) {
			return NewRouteHostAssignment(), nil
		})
}

type SubjectAccessReviewCreator interface {
	Create(context.Context, *authorizationv1.SubjectAccessReview, metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error)
}

type routeHostAssignment struct {
	*admission.Handler
	sarc          SubjectAccessReviewCreator
	hostDefaulter *HostDefaulter
}

var _ = initializer.WantsExternalKubeClientSet(&routeHostAssignment{})
var _ = WantsRouteHostDefaulter(&routeHostAssignment{})
var _ = admission.MutationInterface(&routeHostAssignment{})
var _ = admission.ValidationInterface(&routeHostAssignment{})

func toRouteV1(obj runtime.Object) (*routev1.Route, *routeinternal.Route, error) {
	internal, ok := obj.(*routeinternal.Route)
	if !ok {
		return nil, nil, nil
	}

	var external routev1.Route
	err := routev1conversion.Convert_route_Route_To_v1_Route(internal, &external, nil)
	if err != nil {
		return nil, internal, err
	}

	return &external, internal, nil
}

func (p *routeHostAssignment) Admit(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) error {
	if a.GetResource().GroupResource() != routegroup.Resource("routes") {
		return nil
	}

	if a.GetOperation() != admission.Create {
		return nil
	}

	route, internal, err := toRouteV1(a.GetObject())
	if err != nil {
		return errors.NewInternalError(err)
	}

	errs := AllocateHost(ctx, route, p.sarc, nil)
	if len(errs) > 0 {
		// compat: this was previously performed during (RESTCreateStrategy).Validate
		return errors.NewInvalid(route.GroupVersionKind().GroupKind(), route.Name, errs)
	}

	err = routev1conversion.Convert_v1_Route_To_route_Route(route, internal, nil)
	if err != nil {
		return errors.NewInternalError(err)
	}

	return nil
}

func (p *routeHostAssignment) Validate(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) error {
	if a.GetResource().GroupResource() != routegroup.Resource("routes") {
		return nil
	}

	if a.GetOperation() != admission.Update {
		return nil
	}

	route, _, err := toRouteV1(a.GetObject())
	if err != nil {
		return errors.NewInternalError(err)
	}

	oldRoute, _, err := toRouteV1(a.GetOldObject())
	if err != nil {
		return errors.NewInternalError(err)
	}

	errs := ValidateHostUpdate(ctx, route, oldRoute, p.sarc)
	if len(errs) > 0 {
		// compat: this was previously performed during (RESTUpdateStrategy).ValidateUpdate
		return errors.NewInvalid(route.GroupVersionKind().GroupKind(), route.Name, errs)
	}

	return nil
}

func (p *routeHostAssignment) SetExternalKubeClientSet(client kubernetes.Interface) {
	p.sarc = client.AuthorizationV1().SubjectAccessReviews()
}

func (p *routeHostAssignment) SetRouteHostDefaulter(hd *HostDefaulter) {
	p.hostDefaulter = hd
}

func (p *routeHostAssignment) ValidateInitialization() error {
	if p.sarc == nil {
		return fmt.Errorf("%s plugin needs a subjectaccessreview client", pluginName)
	}
	if p.hostDefaulter == nil {
		return fmt.Errorf("%s plugin needs a host defaulter", pluginName)
	}
	return nil
}

func NewRouteHostAssignment() *routeHostAssignment {
	return &routeHostAssignment{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}
}
