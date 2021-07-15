package requiredrouteannotations

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
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
)

// TestValidate verifies that the RequiredRouteAnnotations plugin properly
// validates newly created routes' annotations.
func TestValidate(t *testing.T) {
	zero := int32(0)
	ninetynine := int32(99)
	fivenines := int32(99999)
	emptyConfig := &configv1.Ingress{}
	nonemptyConfig := &configv1.Ingress{
		Spec: configv1.IngressSpec{
			RequiredHSTSPolicies: []configv1.RequiredHSTSPolicy{{
				DomainPatterns: []string{
					"abc.foo.com",
					"www.foo.com",
				},
				MaxAge:                  configv1.MaxAgePolicy{LargestMaxAge: &fivenines, SmallestMaxAge: &zero},
				PreloadPolicy:           configv1.RequirePreloadPolicy,
				IncludeSubDomainsPolicy: configv1.RequireIncludeSubDomains,
			}},
		},
	}
	noPolicyConfig := &configv1.Ingress{
		Spec: configv1.IngressSpec{
			RequiredHSTSPolicies: []configv1.RequiredHSTSPolicy{{
				DomainPatterns: []string{
					"abc.foo.com",
					"www.foo.com",
				},
				MaxAge:                  configv1.MaxAgePolicy{},
				PreloadPolicy:           configv1.NoOpinionPreloadPolicy,
				IncludeSubDomainsPolicy: configv1.NoOpinionIncludeSubDomains,
			}},
		},
	}
	namespaceMatchConfig := &configv1.Ingress{
		Spec: configv1.IngressSpec{
			RequiredHSTSPolicies: []configv1.RequiredHSTSPolicy{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels:      map[string]string{"www.foo.com": "bar"},
					MatchExpressions: nil,
				},
				DomainPatterns: []string{
					"abc.foo.com",
					"www.foo.com",
				},
				MaxAge:                  configv1.MaxAgePolicy{LargestMaxAge: &fivenines, SmallestMaxAge: &zero},
				PreloadPolicy:           configv1.RequirePreloadPolicy,
				IncludeSubDomainsPolicy: configv1.NoOpinionIncludeSubDomains,
			}},
		},
	}
	multipleMatchConfig := &configv1.Ingress{
		Spec: configv1.IngressSpec{
			RequiredHSTSPolicies: []configv1.RequiredHSTSPolicy{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels:      map[string]string{"www.foo.com": "bar"},
						MatchExpressions: nil,
					},
					DomainPatterns: []string{
						"abc.foo.com",
						"www.foo.com",
					},
					MaxAge:                  configv1.MaxAgePolicy{LargestMaxAge: &fivenines, SmallestMaxAge: &zero},
					PreloadPolicy:           configv1.RequirePreloadPolicy,
					IncludeSubDomainsPolicy: configv1.NoOpinionIncludeSubDomains,
				},
				{
					// this requiredHSTSPolicy covers any namespace
					DomainPatterns: []string{
						"abc.foo.com",
					},
					MaxAge:                  configv1.MaxAgePolicy{LargestMaxAge: &ninetynine, SmallestMaxAge: &zero},
					PreloadPolicy:           configv1.RequireNoPreloadPolicy,
					IncludeSubDomainsPolicy: configv1.RequireNoIncludeSubDomains,
				},
			},
		},
	}

	nsLister := fakeNamespaceLister(map[string]map[string]string{
		"labeledNamespace":           {"www.foo.com": "bar"},
		"unlabeledNamespace":         {"default": ""},
		"matchingDomainNamespace":    {"abc.foo.com": ""},
		"nonmatchingDomainNamespace": {"abc.com": ""},
	})

	tests := []struct {
		description           string
		config                *configv1.Ingress
		routeAnnotations      map[string]string
		namespace             string
		name                  string
		spec                  *routeapi.RouteSpec
		expectForbiddenClause string
		expectForbidden       bool
	}{
		{
			description:     "unannotated route, no required annotations in ingress config",
			config:          emptyConfig,
			namespace:       "unlabeledNamespace",
			spec:            &routeapi.RouteSpec{TLS: &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt}},
			name:            "config1",
			expectForbidden: false,
		},
		{
			description: "annotated route, no policies in ingress config",
			config:      noPolicyConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "max-age=;preload ; includesubdomains",
			},
			namespace: "matchingDomainNamespace",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			name:                  "config1.1",
			expectForbiddenClause: "max-age must be set in HSTS annotation",
			expectForbidden:       true,
		},
		{
			description: "unannotated route, with required annotations in ingress config",
			config:      nonemptyConfig,
			namespace:   "matchingDomainNamespace",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			name:                  "config1.2",
			expectForbiddenClause: "max-age must be set in HSTS annotation",
			expectForbidden:       true,
		},
		{
			description: "appropriately annotated route for required annotations in ingress config",
			config:      nonemptyConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "max-age=9000;preload ; includesubdomains",
			},
			namespace: "matchingDomainNamespace",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			name:            "config2",
			expectForbidden: false,
		},
		{
			description: "route missing some required annotations that are in ingress config",
			config:      nonemptyConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "max-age=9000; misspeledpreload",
			},
			namespace: "matchingDomainNamespace",
			name:      "config3",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbiddenClause: "preload must be specified",
			expectForbidden:       true,
		},
		{
			description: "route has preload but should not",
			config:      multipleMatchConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "max-age=99; preload",
			},
			namespace: "nonmatchingDomainNamespace",
			name:      "config3.1",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbiddenClause: "preload must not be specified",
			expectForbidden:       true,
		},
		{
			description: "route has includeSubDomains but should not",
			config:      multipleMatchConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "max-age=99; includesubdomains",
			},
			namespace: "nonmatchingDomainNamespace",
			name:      "config3.2",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbiddenClause: "includeSubDomains must not be specified",
			expectForbidden:       true,
		},
		{
			description: "route not in matching domain, missing some required annotations that are in ingress config",
			config:      nonemptyConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "max-age=9000; misspeledpreload",
			},
			namespace: "nonmatchingDomainNamespace",
			name:      "config4",
			spec: &routeapi.RouteSpec{
				Host: "def.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbidden: false,
		},
		{
			description: "route in matching labeled namespace, missing some required annotations that are in ingress config",
			config:      namespaceMatchConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "max-age=9000; misspeledpreload",
			},
			namespace: "labeledNamespace",
			name:      "config5",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbiddenClause: "preload must be specified",
			expectForbidden:       true,
		},
		{
			description: "route in matching domain, matching requirements",
			config:      multipleMatchConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "max-age=90",
			},
			namespace: "unlabeledNamespace",
			name:      "config6",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbidden: false,
		},
		{
			description: "route in matching domain, non-matching requirements",
			config:      multipleMatchConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "   max-age=9999 ",
			},
			namespace: "unlabeledNamespace",
			name:      "config7",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbiddenClause: "does not match maximum age 99",
			expectForbidden:       true,
		},
		{
			description: "route in matching domain, non-matching requirements",
			config:      multipleMatchConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "   max-age= ",
			},
			namespace: "unlabeledNamespace",
			name:      "config8",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbiddenClause: "max-age must be set in HSTS annotation",
			expectForbidden:       true,
		},
	}

	for _, tc := range tests {
		routeClient := fakerouteclient.NewSimpleClientset()
		routeClient.PrependReactor("get", "routes",
			routeFn(map[string]map[string]string{"route1": {"default": ""}, "route2": {"foo": "bar"}, "route3": {"abc.foo.com": "bar"}}, tc.spec))
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
			fakeRoute("test-route", tc.namespace, tc.spec, tc.routeAnnotations),
			nil,
			route.Kind("Route").WithVersion("version"),
			tc.namespace,
			tc.name,
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
		case tc.expectForbidden:
			if err == nil {
				t.Errorf("%q: expecting forbidden error %s, got none", tc.description, tc.expectForbiddenClause)
			} else if !strings.Contains(err.Error(), tc.expectForbiddenClause) {
				t.Errorf("%q: expecting forbidden error %s, got %v", tc.description, tc.expectForbiddenClause, err)
			}
		}
	}
}

func fakeNamespace(name string, labels map[string]string) *corev1.Namespace {
	ns := &corev1.Namespace{}
	ns.Name = name
	ns.Labels = labels
	return ns
}

func fakeRoute(name string, namespace string, spec *routeapi.RouteSpec, annotations map[string]string) *routeapi.Route {
	route := &routeapi.Route{}
	route.Name = name
	route.Namespace = namespace
	route.Annotations = annotations
	route.Spec.TLS = spec.TLS
	route.Spec.Host = spec.Host
	return route
}

func fakeNamespaceLister(namespacesAndLabels map[string]map[string]string) corev1listers.NamespaceLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for namespace, labels := range namespacesAndLabels {
		indexer.Add(fakeNamespace(namespace, labels))
	}
	return corev1listers.NewNamespaceLister(indexer)
}

func routeFn(routeAndAnnotations map[string]map[string]string, spec *routeapi.RouteSpec) clientgotesting.ReactionFunc {
	return func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
		name := action.(clientgotesting.GetAction).GetName()
		namespace := action.(clientgotesting.GetAction).GetNamespace()

		return true, fakeRoute(name, namespace, spec, map[string]string(routeAndAnnotations[name])), nil
	}
}

func configFn(config *configv1.Ingress) clientgotesting.ReactionFunc {
	return func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, config, nil
	}
}
