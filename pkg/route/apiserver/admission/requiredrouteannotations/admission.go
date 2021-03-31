package requiredrouteannotations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrutil "k8s.io/apimachinery/pkg/util/errors"
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

// Validate ensures that routes specify required annotations.
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
	ingressConfig, err := o.configClient.Ingresses().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return admission.NewForbidden(a, err)
	}
	routeName := a.GetName()
	route := a.GetObject().(*routeapi.Route)
	var errs []error
	for _, requiredAnnotations := range ingressConfig.Spec.RequiredRouteAnnotations {
		// TODO: Check excludedNamespacesSelector.
		// TODO: Check domains.
		for k, v := range requiredAnnotations.RequiredAnnotations {
			if _, ok := route.Annotations[k]; ok {
				continue
			}
			err := fmt.Errorf("route %q is missing the %q annotation, which is required (suggested value: %q)", routeName, k, v)
			errs = append(errs, err)
		}
	}
	if err := kerrutil.NewAggregate(errs); err != nil {
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
		Handler: admission.NewHandler(admission.Create),
	}, nil
}
