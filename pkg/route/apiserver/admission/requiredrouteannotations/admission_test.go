package requiredrouteannotations

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"
	corev1listers "k8s.io/client-go/listers/core/v1"
	clientgotesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	configv1 "github.com/openshift/api/config/v1"
	route "github.com/openshift/api/route"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	fakerouteclient "github.com/openshift/client-go/route/clientset/versioned/fake"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// TestValidate verifies that the RequiredRouteAnnotations plugin properly
// validates newly created routes' annotations.
func TestValidate(t *testing.T) {
	emptyConfig := &configv1.Ingress{}
	nonemptyConfig := &configv1.Ingress{
		Spec: configv1.IngressSpec{
			RequiredRouteAnnotations: []configv1.RequiredRouteAnnotations{{
				RequiredAnnotations: map[string]string{
					"foo": "bar",
				},
			}},
		},
	}
	tests := []struct {
		description      string
		config           *configv1.Ingress
		routeAnnotations map[string]string
		expectForbidden  bool
	}{
		{
			description: "unannotated route, no required annotations",
			config:      emptyConfig,
		},
		{
			description: "appropriated annotated route for required annotations",
			config:      nonemptyConfig,
			routeAnnotations: map[string]string{
				"foo": "bar",
			},
		},
		{
			description:     "unannotated route, some required annotations",
			config:          nonemptyConfig,
			expectForbidden: true,
		},
	}

	for _, tc := range tests {
		nsLister := fakeNamespaceLister(map[string]map[string]string{
			"unlabeledNamespace": {},
			"labeledNamespace":   {"foo": "bar"},
		})

		routeClient := fakerouteclient.NewSimpleClientset()
		routeClient.PrependReactor("get", "routes", routeFn(map[string]map[string]string{
			"route1": {},
			"route2": {"foo": "bar"},
		}))
		configClient := fakeconfigclient.NewSimpleClientset()
		configClient.PrependReactor("get", "ingresses", configFn(tc.config))
		admitter, err := NewRequiredRouteAnnotations()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		admitter.(*requiredRouteAnnotations).configClient = configClient.ConfigV1()
		admitter.(*requiredRouteAnnotations).routeClient = routeClient.RouteV1()
		admitter.(*requiredRouteAnnotations).nsLister = nsLister
		admitter.(*requiredRouteAnnotations).nsListerSynced = func() bool { return true }
		if err = admitter.(admission.InitializationValidator).ValidateInitialization(); err != nil {
			t.Fatalf("validation error: %v", err)
		}
		a := admission.NewAttributesRecord(
			fakeRoute("test-route", tc.routeAnnotations),
			nil,
			route.Kind("Route").WithVersion("version"),
			"foo",
			"name",
			route.Resource("routes").WithVersion("version"),
			"",
			"CREATE",
			nil,
			false,
			&user.DefaultInfo{Name: "test-user"},
		)
		err = admitter.(admission.ValidationInterface).Validate(context.TODO(), a, nil)
		switch {
		case !tc.expectForbidden && err != nil:
			t.Errorf("%q: got unexpected error for: %v", tc.description, err)
		case tc.expectForbidden && !apierrors.IsForbidden(err):
			t.Errorf("%q: expecting forbidden error, got %v", tc.description, err)
		}
	}
}

func fakeNamespace(name string, labels map[string]string) *corev1.Namespace {
	ns := &corev1.Namespace{}
	ns.Name = name
	ns.Labels = labels
	return ns
}

func fakeRoute(name string, annotations map[string]string) *routeapi.Route {
	route := &routeapi.Route{}
	route.Name = name
	route.Annotations = annotations
	return route
}

func fakeNamespaceLister(namespacesAndLabels map[string]map[string]string) corev1listers.NamespaceLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for namespace, labels := range namespacesAndLabels {
		indexer.Add(fakeNamespace(namespace, labels))
	}
	return corev1listers.NewNamespaceLister(indexer)
}

func routeFn(routeAndAnnotations map[string]map[string]string) clientgotesting.ReactionFunc {
	return func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
		name := action.(clientgotesting.GetAction).GetName()
		return true, fakeRoute(name, map[string]string(routeAndAnnotations[name])), nil
	}
}

func configFn(config *configv1.Ingress) clientgotesting.ReactionFunc {
	return func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, config, nil
	}
}
