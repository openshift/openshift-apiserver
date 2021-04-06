package requiredrouteannotations

import (
	"context"
	"errors"
	"fmt"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/initializer"
	"k8s.io/client-go/informers"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"

	routev1 "github.com/openshift/api/route/v1"
	configtypedclient "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	routetypedclient "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	"github.com/openshift/library-go/pkg/apiserver/admission/admissionrestconfig"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

const (
	pluginName             = "route.openshift.io/RequiredRouteAnnotations"
	timeToWaitForCacheSync = 10 * time.Second
)

func Register(plugins *admission.Plugins) {
	plugins.Register(pluginName,
		func(_ io.Reader) (admission.Interface, error) {
			return NewRequiredRouteAnnotations()
		})
}

type requiredRouteAnnotations struct {
	*admission.Handler
	routeClient    routetypedclient.RoutesGetter
	configClient   configtypedclient.IngressesGetter
	nsLister       corev1listers.NamespaceLister
	nsListerSynced func() bool
}

// Ensure that the required OpenShift admission interfaces are implemented.
var _ = initializer.WantsExternalKubeInformerFactory(&requiredRouteAnnotations{})
var _ = admissionrestconfig.WantsRESTClientConfig(&requiredRouteAnnotations{})
var _ = admission.ValidationInterface(&requiredRouteAnnotations{})

const hstsAnnotation = "david doesn't know this value"

// Validate ensures that routes specify required annotations.
func (o *requiredRouteAnnotations) Validate(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) (err error) {
	if a.GetResource().GroupResource() != routev1.Resource("routes") {
		return nil
	}
	if _, isRoute := a.GetObject().(*routeapi.Route); !isRoute {
		return nil
	}
	newRoute := a.GetObject().(*routeapi.Route)
	oldRoute := a.GetOldObject().(*routeapi.Route)

	// if this is an update and the annotation in question is the same, then we know the update will be allowed
	if a.GetOperation() == admission.Update && newRoute.Annotations[hstsAnnotation] == oldRoute.Annotations[hstsAnnotation] {
		return nil
	}

	if !o.waitForSyncedStore(time.After(timeToWaitForCacheSync)) {
		return admission.NewForbidden(a, errors.New(pluginName+": caches not synchronized"))
	}
	_, err = o.configClient.Ingresses().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	namespace, err := o.nsLister.Get(newRoute.Namespace)
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	if err := isRouteHSTSAllowed(o.RequiredHSTS(), newRoute, namespace); err != nil {
		return admission.NewForbidden(a, err)
	}
	return nil
}

// isRouteHSTSAllowed returns nil if the route is allowed.  Otherwise, returns details and a suggestion in the error
func isRouteHSTSAllowed(requirements []RequiredHSTS, newRoute *routeapi.Route, namespace *corev1.Namespace) error {
	for _, requirement := range requirements {
		if matches, err := requiredHSTSMatchesRoute(requirement, newRoute, namespace); err != nil {
			return err
		} else if !matches {
			continue
		}

		routeHSTS, err := hstsConfigFromRoute(newRoute)
		if err != nil{
			return err
		}
		requirementErr := routeHSTS.meetsRequirements(requirement)
		if requirementErr != nil {
			return requirementErr
		}
		// if the rule matched, then the API can choose to say that only the first  rule applies.  This will make
		// allowlisting particular domains possible by enumerating them and saying they have no requirements
		// this looks really weird to reviewers, so be sure to reference this clearly and write a good set of unit
		// tests
		return nil
	}

	// this means none of the requirements matched this domain
	return nil
}

type hstsConfig struct {
	maxAge            int64
	preload           bool
	includeSubdomains bool
}

func hstsConfigFromRoute(route *routeapi.Route) (hstsConfig, error) {
	// TODO parse out the values from the annotation
	tokens := strings.Split(route.Annotations[hstsAnnotation], ";")
	ret := hstsConfig{}
	for _, token := range tokens{
		trimmed := strings.Trim(token, " ")
		if trimmed == "includeSubDomains"{
			ret.includeSubdomains = true
		}
		if trimmed == "preload"{
			ret.preload = true
		}
		if strings.HasPrefix(trimmed, "max-age="){
			age, err := strconv.ParseInt(trimmed[len("max-age="):], 10, 64)
			if err != nil{
				return hstsConfig{}, err
			}
			ret.maxAge = age
		}
	}
	return ret, nil
}

func (c hstsConfig) meetsRequirements(requirement RequiredHSTS) error {
	configHasMaxAge := c.maxAge != 0
	requirementHasMaxAge := requirement.MaxAge.LargestMaxAge != 0 || requirement.MaxAge.SmallestMaxAge != 0
	if requirementHasMaxAge && !configHasMaxAge {
		return fmt.Errorf("max age is required")
	}
	if requirement.MaxAge.LargestMaxAge != 0 && c.maxAge > requirement.MaxAge.LargestMaxAge {
		return fmt.Errorf("does not match max age")
	}
	if requirement.MaxAge.SmallestMaxAge != 0 && c.maxAge < requirement.MaxAge.SmallestMaxAge {
		return fmt.Errorf("does not match minimum age")
	}

	switch requirement.PreloadPolicy {
	case DefaultPreloadPolicy, NoOpinionPreloadPolicy:
		// anything is allowed, do nothing
	case RequirePreloadPolicy:
		if !c.preload {
			return fmt.Errorf("preload must be true")
		}
	case RequireNoPreloadPolicy:
		if c.preload {
			return fmt.Errorf("preload must be unspecified")
		}
	}

	switch requirement.IncludeSubdomainsPolicy {
	case DefaultIncludeSubdomains, NoOpinionIncludeSubdomains:
		// anything is allowed, do nothing
	case RequireIncludeSubdomains:
		if !c.includeSubdomains {
			return fmt.Errorf("includeSubdomains must be true")
		}
	case RequireNotIncludeSubdomains:
		if c.includeSubdomains {
			return fmt.Errorf("includeSubdomains must be unspecified")
		}
	}

	return nil
}

func requiredHSTSMatchesRoute(requirement RequiredHSTS, route *routeapi.Route, namespace *corev1.Namespace) (bool, error) {
	matchesNamespace, err := matchesNamespaceSelector(requirement.NamespaceSelector, namespace)
	if err != nil {
		return false, err
	}
	if !matchesNamespace {
		return false, nil
	}
	routeDomains := []string{route.Spec.Host}
	for _, ingress := range route.Status.Ingress {
		routeDomains = append(routeDomains, ingress.Host)
	}
	if matchesDomain(requirement.DomainPatterns, routeDomains) {
		return true, nil
	}

	return false, nil
}

func matchesDomain(domainMatchers []string, domains []string) bool {
	for _, routeDomain := range domains {
		for _, matcher := range domainMatchers {
			if !strings.HasPrefix(matcher, "*.") && routeDomain == matcher {
				return true
			}
			// this needs a unit test, it might work
			if strings.HasSuffix(routeDomain, matcher[1:]) {
				return true
			}
		}
	}

	return false
}

func matchesNamespaceSelector(nsSelector *metav1.LabelSelector, namespace *corev1.Namespace) (bool, error) {
	if nsSelector == nil {
		return true, nil
	}
	selector, err := getParsedNamespaceSelector(nsSelector)
	if err != nil {
		return false, err
	}
	return selector.Matches(labels.Set(namespace.Labels)), nil
}

func getParsedNamespaceSelector(nsSelector *metav1.LabelSelector) (labels.Selector, error) {
	// TODO cache this result to save time
	return metav1.LabelSelectorAsSelector(nsSelector)
}

// this is the actual API I'm proposing for Ingress.config.openshift.io
type RequiredHSTS struct {
	// This exactly matches the admission webhook struct
	// NamespaceSelector decides whether to run the webhook on an object based
	// on whether the namespace for that object matches the selector. If the
	// object itself is a namespace, the matching is performed on
	// object.metadata.labels. If the object is another cluster scoped resource,
	// it never skips the webhook.
	//
	// For example, to run the webhook on any objects whose namespace is not
	// associated with "runlevel" of "0" or "1";  you will set the selector as
	// follows:
	// "namespaceSelector": {
	//   "matchExpressions": [
	//     {
	//       "key": "runlevel",
	//       "operator": "NotIn",
	//       "values": [
	//         "0",
	//         "1"
	//       ]
	//     }
	//   ]
	// }
	//
	// If instead you want to only run the webhook on any objects whose
	// namespace is associated with the "environment" of "prod" or "staging";
	// you will set the selector as follows:
	// "namespaceSelector": {
	//   "matchExpressions": [
	//     {
	//       "key": "environment",
	//       "operator": "In",
	//       "values": [
	//         "prod",
	//         "staging"
	//       ]
	//     }
	//   ]
	// }
	//
	// See
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
	// for more examples of label selectors.
	//
	// Default to the empty LabelSelector, which matches everything.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty" protobuf:"bytes,5,opt,name=namespaceSelector"`

	// *.foo.com matches everything under foo.com.
	// foo.com only matches foo.com
	// to cover foo.com and everything under it, you must specify *both*
	DomainPatterns []string

	MaxAge                  MaxAgePolicy
	PreloadPolicy           PreloadPolicy
	IncludeSubdomainsPolicy IncludeSubdomainsPolicy
}

type MaxAgePolicy struct {
	// zero means no opinion
	// omitempty
	LargestMaxAge int64
	// zero means no opinion
	// omitempty
	SmallestMaxAge int64
}

type PreloadPolicy string

var (
	RequirePreloadPolicy   PreloadPolicy = "RequirePreload"
	RequireNoPreloadPolicy PreloadPolicy = "RequireNotPreload"
	NoOpinionPreloadPolicy PreloadPolicy = "NoOpinion"
	DefaultPreloadPolicy   PreloadPolicy = ""
)

type IncludeSubdomainsPolicy string

var (
	RequireIncludeSubdomains    IncludeSubdomainsPolicy = "RequireIncludeSubdomains"
	RequireNotIncludeSubdomains IncludeSubdomainsPolicy = "RequireNotIncludeSubdomains"
	NoOpinionIncludeSubdomains  IncludeSubdomainsPolicy = "NoOpinion"
	DefaultIncludeSubdomains    IncludeSubdomainsPolicy = ""
)

// this won't be necessary in the end
func (o *requiredRouteAnnotations) RequiredHSTS() []RequiredHSTS {
	return nil
}

func (o *requiredRouteAnnotations) SetRESTClientConfig(restClientConfig rest.Config) {
	var err error
	// TODO: Use an informer for config.
	o.configClient, err = configtypedclient.NewForConfig(&restClientConfig)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	o.routeClient, err = routetypedclient.NewForConfig(&restClientConfig)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
}

func (o *requiredRouteAnnotations) SetExternalKubeInformerFactory(kubeInformers informers.SharedInformerFactory) {
	o.nsLister = kubeInformers.Core().V1().Namespaces().Lister()
	o.nsListerSynced = kubeInformers.Core().V1().Namespaces().Informer().HasSynced
}

func (o *requiredRouteAnnotations) waitForSyncedStore(timeout <-chan time.Time) bool {
	for !o.nsListerSynced() {
		select {
		case <-time.After(100 * time.Millisecond):
		case <-timeout:
			return o.nsListerSynced()
		}
	}

	return true
}

func (o *requiredRouteAnnotations) ValidateInitialization() error {
	if o.configClient == nil {
		return fmt.Errorf(pluginName + " plugin needs a config client")
	}
	if o.routeClient == nil {
		return fmt.Errorf(pluginName + " plugin needs a route client")
	}
	if o.nsLister == nil {
		return fmt.Errorf(pluginName + " plugin needs a namespace lister")
	}
	if o.nsListerSynced == nil {
		return fmt.Errorf(pluginName + " plugin needs a namespace lister synced")
	}
	return nil
}

func NewRequiredRouteAnnotations() (admission.Interface, error) {
	return &requiredRouteAnnotations{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}, nil
}
