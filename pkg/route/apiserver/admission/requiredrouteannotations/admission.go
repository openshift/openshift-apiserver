package requiredrouteannotations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"k8s.io/apimachinery/pkg/api/meta"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/initializer"
	"k8s.io/client-go/informers"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	configv1listers "github.com/openshift/client-go/config/listers/config/v1"
	routeinformers "github.com/openshift/client-go/route/informers/externalversions"
	routev1listers "github.com/openshift/client-go/route/listers/route/v1"
	openshiftapiserveradmission "github.com/openshift/openshift-apiserver/pkg/admission"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

const (
	pluginName             = "route.openshift.io/RequiredRouteAnnotations"
	timeToWaitForCacheSync = 10 * time.Second
)

func Register(plugins *admission.Plugins) {
	plugins.Register(pluginName,
		func(_ io.Reader) (admission.Interface, error) {
			klog.Infof("%s registered", pluginName)
			return NewRequiredRouteAnnotations()
		})
}

type requiredRouteAnnotations struct {
	*admission.Handler
	routeLister       routev1listers.RouteLister
	nsLister          corev1listers.NamespaceLister
	ingressLister     configv1listers.IngressLister
	cachesToSync      []cache.InformerSynced
	cachesToSyncNames []string
}

// Ensure that the required OpenShift admission interfaces are implemented.
var _ = initializer.WantsExternalKubeInformerFactory(&requiredRouteAnnotations{})
var _ = admission.ValidationInterface(&requiredRouteAnnotations{})
var _ = openshiftapiserveradmission.WantsOpenShiftConfigInformers(&requiredRouteAnnotations{})
var _ = openshiftapiserveradmission.WantsOpenShiftRouteInformers(&requiredRouteAnnotations{})

const hstsAnnotation = "haproxy.router.openshift.io/hsts_header"

// Validate ensures that routes specify required annotations, and returns nil if valid.
func (o *requiredRouteAnnotations) Validate(ctx context.Context, a admission.Attributes, _ admission.ObjectInterfaces) (err error) {
	if a.GetResource().GroupResource() != routev1.Resource("routes") {
		return nil
	}
	newobject, isRoute := a.GetObject().(*routeapi.Route)
	if !isRoute {
		return nil
	}

	// Skip the validation if we're not updating/creating
	switch a.GetOperation() {
	case admission.Update, admission.Create:
	default:
		klog.Infof("ignoring %v on %s/%s", a.GetOperation(), a.GetNamespace(), a.GetName())
		return nil
	}

	// Determine if there are HSTS changes in this update
	if a.GetOperation() == admission.Update {
		wants, has := false, false
		var oldhsts, newhsts string

		newmetadata, err := meta.Accessor(newobject)
		if err != nil {
			return err
		}
		newannotations := newmetadata.GetAnnotations()
		if newannotations != nil {
			newhsts, wants = newannotations[hstsAnnotation]
		}

		oldObject := a.GetOldObject()
		if oldObject != nil {
			oldmetadata, err := meta.Accessor(oldObject)
			if err != nil {
				return err
			}
			oldannotations := oldmetadata.GetAnnotations()
			if oldannotations != nil {
				oldhsts, has = oldannotations[hstsAnnotation]
			}
		}

		// Skip the validation if we're not making a change to HSTS  this  update
		if wants == has && newhsts == oldhsts {
			klog.Infof("ignoring %v on %s/%s that doesn't change HSTS policy (old=%v, new=%v)", a.GetOperation(), a.GetNamespace(), a.GetName(), oldhsts, newhsts)
			return nil
		}
	}

	// Since we are modifying route HSTS, wait for all caches to sync
	if !o.waitForSyncedStore(ctx) {
		synced := []string{}
		for i := range o.cachesToSync {
			synced = append(synced, fmt.Sprintf("%v=%v", o.cachesToSyncNames[i], o.cachesToSync[i]()))
		}
		return admission.NewForbidden(a, errors.New(pluginName+": caches not synchronized: "+strings.Join(synced, ", ")))
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
	klog.Infof("allowing %v on %s/%s: %v", a.GetOperation(), a.GetNamespace(), a.GetName(), newRoute)
	return nil
}

var setExternalKubeInformerFactoryOnce sync.Once

func (o *requiredRouteAnnotations) SetExternalKubeInformerFactory(kubeInformers informers.SharedInformerFactory) {
	klog.Info("SetExternalKubeInformerFactory was called")
	setExternalKubeInformerFactoryOnce.Do(func() {
		klog.Info("SetExternalKubeInformerFactory was called (once)")
		o.nsLister = kubeInformers.Core().V1().Namespaces().Lister()
		o.cachesToSync = append(o.cachesToSync, kubeInformers.Core().V1().Namespaces().Informer().HasSynced)
		o.cachesToSyncNames = append(o.cachesToSyncNames, "namespace")
	})
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

func NewRequiredRouteAnnotations() (*requiredRouteAnnotations, error) {
	klog.Info("NewRequiredRouteAnnotations was called")
	return &requiredRouteAnnotations{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}, nil
}

func (o *requiredRouteAnnotations) SetOpenShiftRouteInformers(informers routeinformers.SharedInformerFactory) {
	klog.Info("SetOpenShiftRouteInformers was called")
	o.cachesToSync = append(o.cachesToSync, informers.Route().V1().Routes().Informer().HasSynced)
	o.cachesToSyncNames = append(o.cachesToSyncNames, "route")
	o.routeLister = informers.Route().V1().Routes().Lister()
}

func (o *requiredRouteAnnotations) SetOpenShiftConfigInformers(informers configinformers.SharedInformerFactory) {
	klog.Info("SetOpenShiftConfigInformers was called")
	o.cachesToSync = append(o.cachesToSync, informers.Config().V1().Ingresses().Informer().HasSynced)
	o.cachesToSyncNames = append(o.cachesToSyncNames, "config")
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
