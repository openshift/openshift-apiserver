package requiredrouteannotations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/initializer"
	"k8s.io/client-go/informers"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	logger "k8s.io/klog/klogr"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	configtypedclient "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	routetypedclient "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	"github.com/openshift/library-go/pkg/apiserver/admission/admissionrestconfig"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

var Logger = logger.New()
var log = Logger.WithName("admission")

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

const hstsAnnotation = "haproxy.router.openshift.io/hsts_header"

// Validate ensures that routes specify required annotations, and returns nil if valid.
func (o *requiredRouteAnnotations) Validate(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) (err error) {
	if a.GetResource().GroupResource() != routev1.Resource("routes") {
		return nil
	}
	if _, isRoute := a.GetObject().(*routeapi.Route); !isRoute {
		return nil
	}

	if !o.waitForSyncedStore(time.After(timeToWaitForCacheSync)) {
		return admission.NewForbidden(a, errors.New(pluginName+": caches not synchronized"))
	}

	ingress, err := o.configClient.Ingresses().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	newRoute := a.GetObject().(*routeapi.Route)
	namespace, err := o.nsLister.Get(newRoute.Namespace)
	if err != nil {
		return admission.NewForbidden(a, err)
	}

	if err = isRouteHSTSAllowed(ingress, newRoute, namespace); err != nil {
		return admission.NewForbidden(a, err)
	}
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

// isRouteHSTSAllowed returns nil if the route is allowed.  Otherwise, returns details and a suggestion in the error
func isRouteHSTSAllowed(ingress *configv1.Ingress, newRoute *routeapi.Route, namespace *corev1.Namespace) error {
	// Invalid if a HSTS Policy is specified but this route is not TLS.  Just log a warning.
	if tls := newRoute.Spec.TLS; tls != nil {
		switch termination := tls.Termination; termination {
		case routeapi.TLSTerminationEdge, routeapi.TLSTerminationReencrypt:
		// Valid case
		default:
			// Non-tls routes will not get HSTS headers, but can still be valid
			log.Info("HSTS Policy not accepted for %s, termination type: %s", newRoute.Name, termination)
			return nil
		}
	} else {
		return fmt.Errorf("termination type is empty, must be %s or %s", routev1.TLSTerminationEdge, routev1.TLSTerminationReencrypt)
	}

	requirements := ingress.Spec.RequiredHSTSPolicies
	for _, requirement := range requirements {
		// Check if the required namespaceSelector (if any) and the domainPattern match
		if matchesReqNamespace, matchesReqDomain, err := requiredNamespaceDomainMatchesRoute(requirement, newRoute, namespace); err != nil {
			return err
		} else if !(matchesReqNamespace && matchesReqDomain) {
			// If one of either the namespaceSelector or domain didn't match, we will continue to look
			continue
		}

		routeHSTS, err := hstsConfigFromRoute(newRoute)
		if err != nil {
			return err
		}

		// If there is no annotation but there needs to be one, return error
		if routeHSTS != nil {
			requirementErr := routeHSTS.meetsRequirements(requirement)
			if requirementErr != nil {
				return requirementErr
			}
		}

		// Validation only checks the first matching required HSTS rule.
		return nil
	}

	// None of the requirements matched this route's domain/namespace, it is automatically allowed
	return nil
}

type hstsConfig struct {
	maxAge            int32
	preload           bool
	includeSubDomains bool
}

// Parse out the hstsConfig fields from the annotation
// Unrecognized fields are ignored
func hstsConfigFromRoute(route *routeapi.Route) (*hstsConfig, error) {
	var ret hstsConfig

	trimmed := strings.ToLower(strings.ReplaceAll(route.Annotations[hstsAnnotation], " ", ""))
	tokens := strings.Split(trimmed, ";")
	for _, token := range tokens {
		if strings.EqualFold(token, "includeSubDomains") {
			ret.includeSubDomains = true
		}
		if strings.EqualFold(token, "preload") {
			ret.preload = true
		}
		// unrecognized tokens are ignored
	}

	reg := regexp.MustCompile(`max-age=(\d+)`)
	if match := reg.FindStringSubmatch(trimmed); match != nil && len(match) > 1 {
		age, err := strconv.ParseInt(match[1], 10, 32)
		if err != nil {
			return nil, err
		}
		ret.maxAge = int32(age)
	} else {
		return nil, fmt.Errorf("max-age must be set in HSTS annotation")
	}

	return &ret, nil
}

// Make sure the given requirement meets the configured HSTS policy, validating:
// - range for maxAge (existence already established)
// - preloadPolicy
// - includeSubDomainsPolicy
func (c *hstsConfig) meetsRequirements(requirement configv1.RequiredHSTSPolicy) error {
	if requirement.MaxAge.LargestMaxAge != nil && *requirement.MaxAge.LargestMaxAge >= 0 && c.maxAge > *requirement.MaxAge.LargestMaxAge {
		return fmt.Errorf("does not match maximum age %d", *requirement.MaxAge.LargestMaxAge)
	}
	if requirement.MaxAge.SmallestMaxAge != nil && *requirement.MaxAge.SmallestMaxAge >= 0 && c.maxAge < *requirement.MaxAge.SmallestMaxAge {
		return fmt.Errorf("does not match minimum age %d", *requirement.MaxAge.SmallestMaxAge)
	}

	switch requirement.PreloadPolicy {
	case configv1.NoOpinionPreloadPolicy:
	// anything is allowed, do nothing
	case configv1.RequirePreloadPolicy:
		if !c.preload {
			return fmt.Errorf("preload must be specified")
		}
	case configv1.RequireNoPreloadPolicy:
		if c.preload {
			return fmt.Errorf("preload must not be specified")
		}
	}

	switch requirement.IncludeSubDomainsPolicy {
	case configv1.NoOpinionIncludeSubDomains:
	// anything is allowed, do nothing
	case configv1.RequireIncludeSubDomains:
		if !c.includeSubDomains {
			return fmt.Errorf("includeSubDomains must be specified")
		}
	case configv1.RequireNoIncludeSubDomains:
		if c.includeSubDomains {
			return fmt.Errorf("includeSubDomains must not be specified")
		}
	}

	return nil
}

// Check if the route matches the required domain/namespace in the HSTS Policy
func requiredNamespaceDomainMatchesRoute(requirement configv1.RequiredHSTSPolicy, route *routeapi.Route, namespace *corev1.Namespace) (bool, bool, error) {
	matchesNamespace, err := matchesNamespaceSelector(requirement.NamespaceSelector, namespace)
	if err != nil {
		return false, false, err
	}

	routeDomains := []string{route.Spec.Host}
	for _, ingress := range route.Status.Ingress {
		routeDomains = append(routeDomains, ingress.Host)
	}
	matchesDom := matchesDomain(requirement.DomainPatterns, routeDomains)

	return matchesNamespace, matchesDom, nil
}

// Check all of the required domainMatcher patterns against all provided domains,
// first match returns true.  If none match, return false.
func matchesDomain(domainMatchers []string, domains []string) bool {
	for _, matcher := range domainMatchers {
		match := regexp.MustCompile(fmt.Sprintf("^%s$", matcher))
		for _, candidate := range domains {
			if match.MatchString(candidate) {
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
