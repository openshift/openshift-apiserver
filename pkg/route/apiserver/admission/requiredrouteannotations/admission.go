package requiredrouteannotations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/initializer"
	"k8s.io/client-go/informers"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	configv1 "github.com/openshift/api/config/v1"
	grouproute "github.com/openshift/api/route"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	configv1listers "github.com/openshift/client-go/config/listers/config/v1"
	routeinformers "github.com/openshift/client-go/route/informers/externalversions"
	routev1listers "github.com/openshift/client-go/route/listers/route/v1"
	openshiftapiserveradmission "github.com/openshift/openshift-apiserver/pkg/admission"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

const (
	pluginName = "route.openshift.io/RequiredRouteAnnotations"
	// To cover scenarios with 10,000 routes, need to wait up to 30 seconds for caches to sync
	timeToWaitForCacheSync = 30 * time.Second
	hstsAnnotation         = "haproxy.router.openshift.io/hsts_header"
)

func Register(plugins *admission.Plugins) {
	plugins.Register(pluginName,
		func(_ io.Reader) (admission.Interface, error) {
			return NewRequiredRouteAnnotations(), nil
		})
}

type requiredRouteAnnotations struct {
	*admission.Handler
	routeLister   routev1listers.RouteLister
	nsLister      corev1listers.NamespaceLister
	ingressLister configv1listers.IngressLister
	cachesSynced  bool
	cachesToSync  []cache.InformerSynced
}

// Ensure that the required OpenShift admission interfaces are implemented.
var _ = initializer.WantsExternalKubeInformerFactory(&requiredRouteAnnotations{})
var _ = admission.ValidationInterface(&requiredRouteAnnotations{})
var _ = openshiftapiserveradmission.WantsOpenShiftConfigInformers(&requiredRouteAnnotations{})
var _ = openshiftapiserveradmission.WantsOpenShiftRouteInformers(&requiredRouteAnnotations{})

var maxAgeRegExp = regexp.MustCompile(`max-age=(\d+)`)

// Validate ensures that routes specify required annotations, and returns nil if valid.
func (o *requiredRouteAnnotations) Validate(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) (err error) {
	if a.GetResource().GroupResource() != grouproute.Resource("routes") {
		return nil
	}
	newObject, isRoute := a.GetObject().(*routeapi.Route)
	if !isRoute {
		return nil
	}

	// Skip the validation if we're not updating/creating
	switch a.GetOperation() {
	case admission.Update, admission.Create:
	default:
		return nil
	}

	// Determine if there are HSTS changes in this update
	if a.GetOperation() == admission.Update {
		wants, has := false, false
		var oldHSTS, newHSTS string

		newMetaData, err := meta.Accessor(newObject)
		if err != nil {
			return err
		}
		newAnnotations := newMetaData.GetAnnotations()
		newHSTS, wants = newAnnotations[hstsAnnotation]

		oldObject := a.GetOldObject()
		if oldObject != nil {
			oldMetaData, err := meta.Accessor(oldObject)
			if err != nil {
				return err
			}
			oldAnnotations := oldMetaData.GetAnnotations()
			if oldAnnotations != nil {
				oldHSTS, has = oldAnnotations[hstsAnnotation]
			}
		}

		// Skip the validation if we're not making a change to HSTS at this time
		if wants == has && newHSTS == oldHSTS {
			return nil
		}
	}

	// Wait up to 30 seconds for all caches to sync.  This is needed only once.
	if !o.cachesSynced {
		if synced := o.waitForSyncedStore(ctx); !synced {
			return admission.NewForbidden(a, errors.New(pluginName+": caches not synchronized"))
		}
		o.cachesSynced = true
	}

	ingress, err := o.ingressLister.Get("cluster")
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

func (o *requiredRouteAnnotations) SetExternalKubeInformerFactory(kubeInformers informers.SharedInformerFactory) {
	o.nsLister = kubeInformers.Core().V1().Namespaces().Lister()
	o.cachesToSync = append(o.cachesToSync, kubeInformers.Core().V1().Namespaces().Informer().HasSynced)
}

func (o *requiredRouteAnnotations) waitForSyncedStore(ctx context.Context) bool {
	syncCtx, cancelFn := context.WithTimeout(ctx, timeToWaitForCacheSync)
	defer cancelFn()
	return cache.WaitForNamedCacheSync(pluginName, syncCtx.Done(), o.cachesToSync...)
}

func (o *requiredRouteAnnotations) ValidateInitialization() error {
	if o.ingressLister == nil {
		return fmt.Errorf(pluginName + " plugin needs an ingress lister")
	}
	if o.routeLister == nil {
		return fmt.Errorf(pluginName + " plugin needs a route lister")
	}
	if o.nsLister == nil {
		return fmt.Errorf(pluginName + " plugin needs a namespace lister")
	}
	if len(o.cachesToSync) < 3 {
		return fmt.Errorf(pluginName + " plugin missing informer synced functions")
	}
	return nil
}

func NewRequiredRouteAnnotations() *requiredRouteAnnotations {
	return &requiredRouteAnnotations{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}
}

func (o *requiredRouteAnnotations) SetOpenShiftRouteInformers(informers routeinformers.SharedInformerFactory) {
	o.cachesToSync = append(o.cachesToSync, informers.Route().V1().Routes().Informer().HasSynced)
	o.routeLister = informers.Route().V1().Routes().Lister()
}

func (o *requiredRouteAnnotations) SetOpenShiftConfigInformers(informers configinformers.SharedInformerFactory) {
	o.cachesToSync = append(o.cachesToSync, informers.Config().V1().Ingresses().Informer().HasSynced)
	o.ingressLister = informers.Config().V1().Ingresses().Lister()
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
			klog.Warningf("HSTS Policy not added for %s, wrong termination type: %s", newRoute.Name, termination)
			return nil
		}
	}

	requirements := ingress.Spec.RequiredHSTSPolicies
	for _, requirement := range requirements {
		// Check if the required namespaceSelector (if any) and the domainPattern match
		if matches, err := requiredNamespaceDomainMatchesRoute(requirement, newRoute, namespace); err != nil {
			return err
		} else if !matches {
			// If one of either the namespaceSelector or domain didn't match, we will continue to look
			continue
		}

		routeHSTS, err := hstsConfigFromRoute(newRoute)
		if err != nil {
			return err
		}

		// If there is no annotation but there needs to be one, return error
		if routeHSTS != nil {
			if err = routeHSTS.meetsRequirements(requirement); err != nil {
				return err
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

	if match := maxAgeRegExp.FindStringSubmatch(trimmed); match != nil && len(match) > 1 {
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
	if requirement.MaxAge.LargestMaxAge != nil && c.maxAge > *requirement.MaxAge.LargestMaxAge {
		return fmt.Errorf("is greater than maximum age (%d)", *requirement.MaxAge.LargestMaxAge)
	}
	if requirement.MaxAge.SmallestMaxAge != nil && c.maxAge < *requirement.MaxAge.SmallestMaxAge {
		return fmt.Errorf("is less than minimum age (%d)", *requirement.MaxAge.SmallestMaxAge)
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
func requiredNamespaceDomainMatchesRoute(requirement configv1.RequiredHSTSPolicy, route *routeapi.Route, namespace *corev1.Namespace) (bool, error) {
	matchesNamespace, err := matchesNamespaceSelector(requirement.NamespaceSelector, namespace)
	if err != nil {
		return false, err
	}

	routeDomains := []string{route.Spec.Host}
	for _, ingress := range route.Status.Ingress {
		routeDomains = append(routeDomains, ingress.Host)
	}
	matchesDom := matchesDomain(requirement.DomainPatterns, routeDomains)

	return matchesNamespace && matchesDom, nil
}

// Check all of the required domainMatcher patterns against all provided domains,
// first match returns true.  If none match, return false.
func matchesDomain(domainMatchers []string, domains []string) bool {
	for _, pattern := range domainMatchers {
		for _, candidate := range domains {
			matched, err := filepath.Match(pattern, candidate)
			if err != nil {
				klog.Warningf("Ignoring HSTS Policy domain match for %s, error parsing: %v", candidate, err)
				continue
			}
			if matched {
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
		klog.Warningf("Ignoring HSTS Policy namespace match for %s, error parsing: %v", namespace, err)
		return false, err
	}
	return selector.Matches(labels.Set(namespace.Labels)), nil
}

func getParsedNamespaceSelector(nsSelector *metav1.LabelSelector) (labels.Selector, error) {
	// TODO cache this result to save time
	return metav1.LabelSelectorAsSelector(nsSelector)
}
