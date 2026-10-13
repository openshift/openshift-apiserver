package requiredrouteannotations

import (
	"context"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/api/route"
	configv1listers "github.com/openshift/client-go/config/listers/config/v1"
	routev1listers "github.com/openshift/client-go/route/listers/route/v1"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

// TestValidate verifies that the RequiredRouteAnnotations plugin properly
// validates newly created routes' annotations.
func TestValidate(t *testing.T) {
	zero := int32(0)
	one := int32(1)
	ninetynine := int32(99)
	fivenines := int32(99999)

	emptyConfig := &configv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
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
	wildcardDomainConfig := &configv1.Ingress{
		Spec: configv1.IngressSpec{
			RequiredHSTSPolicies: []configv1.RequiredHSTSPolicy{{
				DomainPatterns: []string{
					"*.foo.com",
				},
				MaxAge: configv1.MaxAgePolicy{LargestMaxAge: &fivenines, SmallestMaxAge: &zero},
			}},
		},
	}
	wildcardDomainConfig2 := &configv1.Ingress{
		Spec: configv1.IngressSpec{
			RequiredHSTSPolicies: []configv1.RequiredHSTSPolicy{{
				DomainPatterns: []string{
					"*.foo.com",
					"foo.com",
				},
				MaxAge: configv1.MaxAgePolicy{LargestMaxAge: &fivenines, SmallestMaxAge: &zero},
			}},
		},
	}
	noPolicyConfig := &configv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
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
					MaxAge:                  configv1.MaxAgePolicy{LargestMaxAge: &ninetynine, SmallestMaxAge: &one},
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
		oldrouteAnnotations   map[string]string
		namespace             string
		name                  string
		spec                  *routeapi.RouteSpec
		oldspec               *routeapi.RouteSpec
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
			expectForbiddenClause: HSTSMaxAgeMissingOrWrongError,
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
			expectForbiddenClause: HSTSMaxAgeMissingOrWrongError,
			expectForbidden:       true,
		},
		{
			description: "unannotated route, with required annotations in ingress config, but no hsts in the old route",
			config:      nonemptyConfig,
			namespace:   "matchingDomainNamespace",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			oldspec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			name:            "config1.3",
			expectForbidden: false, // Okay because the old route also had no HSTS
		},
		{
			description: "unannotated route, with required annotations in ingress config, and hsts in the old route",
			config:      nonemptyConfig,
			namespace:   "matchingDomainNamespace",
			oldrouteAnnotations: map[string]string{
				hstsAnnotation: "max-age=9000",
			},
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			oldspec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			name:                  "config1.4",
			expectForbiddenClause: HSTSMaxAgeMissingOrWrongError,
			expectForbidden:       true, // Not okay because the old route had HSTS
		},
		{
			description: "unannotated route, non-TLS (nil), matching domain",
			config:      nonemptyConfig,
			namespace:   "matchingDomainNamespace",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
			},
			name:            "config1.5",
			expectForbidden: false,
		},
		{
			description: "unannotated route, non-TLS (passthrough), matching domain",
			config:      nonemptyConfig,
			namespace:   "matchingDomainNamespace",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationPassthrough},
			},
			name:            "config1.6",
			expectForbidden: false,
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
			expectForbiddenClause: HSTSPreloadMustError,
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
			expectForbiddenClause: HSTSPreloadMustNotError,
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
			expectForbiddenClause: HSTSIncludeSubDomainsMustNotError,
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
			expectForbiddenClause: HSTSPreloadMustError,
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
			description: "route in matching domain, but max-age exceeds",
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
			expectForbiddenClause: fmt.Sprintf(HSTSMaxAgeGreaterError, 99),
			expectForbidden:       true,
		},
		{
			description: "route in matching domain, but max-age too small",
			config:      multipleMatchConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "   max-age=0 ",
			},
			namespace: "unlabeledNamespace",
			name:      "config7.1",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbiddenClause: fmt.Sprintf(HSTSMaxAgeLessThanError, 1),
			expectForbidden:       true,
		},
		{
			description: "route in matching domain, but max-age missing",
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
			expectForbiddenClause: HSTSMaxAgeMissingOrWrongError,
			expectForbidden:       true,
		},
		{
			description: "route in matching domain, by wildcard",
			config:      wildcardDomainConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "   max-age= ",
			},
			namespace: "unlabeledNamespace",
			name:      "config9.1",
			spec: &routeapi.RouteSpec{
				Host: "abc.foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbiddenClause: HSTSMaxAgeMissingOrWrongError,
			expectForbidden:       true,
		},
		{
			description: "route not in matching domain, by wildcard",
			config:      wildcardDomainConfig,
			routeAnnotations: map[string]string{
				hstsAnnotation: "   max-age= ",
			},
			name:      "config9.2",
			namespace: "unlabeledNamespace",
			spec: &routeapi.RouteSpec{
				Host: "foo.com", // this doesn't match the configured *.foo.com
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbidden: false,
		},
		{
			description: "route in matching domain, by wildcard 2",
			config:      wildcardDomainConfig2,
			routeAnnotations: map[string]string{
				hstsAnnotation: "   max-age= ",
			},
			name:      "config9.3",
			namespace: "unlabeledNamespace",
			spec: &routeapi.RouteSpec{
				Host: "foo.com",
				TLS:  &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			},
			expectForbiddenClause: HSTSMaxAgeMissingOrWrongError,
			expectForbidden:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			admitter := NewRequiredRouteAnnotations()
			if admitter == nil {
				t.Fatal("Unexpected error: admitter is nil")
			}

			admitter.ingressLister = fakeIngressLister("cluster", *tc.config)
			admitter.routeLister = fakeRouteLister(map[string]map[string]string{"route1": {"default": ""}, "route2": {"foo": "bar"}, "route3": {"abc.foo.com": "bar"}}, tc.spec)
			admitter.nsLister = nsLister
			admitter.cachesToSync = append(admitter.cachesToSync, func() bool { return true })
			admitter.cachesToSync = append(admitter.cachesToSync, func() bool { return true })
			admitter.cachesToSync = append(admitter.cachesToSync, func() bool { return true })

			if err := admitter.ValidateInitialization(); err != nil {
				t.Fatalf("validation error: %v", err)
			}
			op := admission.Create
			if tc.oldspec != nil {
				op = admission.Update
			}
			a := admission.NewAttributesRecord(
				fakeRoute("test-route", tc.namespace, tc.spec, tc.routeAnnotations),
				fakeRoute("old-test-route", tc.namespace, tc.oldspec, tc.oldrouteAnnotations),
				route.Kind("Route").WithVersion("version"),
				tc.namespace,
				tc.name,
				route.Resource("routes").WithVersion("version"),
				"",
				op,
				nil,
				false,
				&user.DefaultInfo{Name: "test-user"},
			)
			err := admitter.Validate(context.TODO(), a, nil)
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
		})
	}
}

func fakeNamespace(name string, labels map[string]string) *corev1.Namespace {
	ns := &corev1.Namespace{}
	ns.Name = name
	ns.Labels = labels
	return ns
}

func fakeRoute(name string, namespace string, spec *routeapi.RouteSpec, annotations map[string]string) *routeapi.Route {
	r := &routeapi.Route{}
	r.Name = name
	r.Namespace = namespace
	if annotations != nil {
		r.Annotations = annotations
	}
	if spec != nil {
		r.Spec.TLS = spec.TLS
		r.Spec.Host = spec.Host
	}
	return r
}

func fakeIngress(name string, config configv1.Ingress) *configv1.Ingress {
	ingress := &configv1.Ingress{}
	ingress.ObjectMeta.Name = name
	ingress.Spec = config.Spec
	return ingress
}

func fakeNamespaceLister(namespacesAndLabels map[string]map[string]string) corev1listers.NamespaceLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for namespace, labels := range namespacesAndLabels {
		if err := indexer.Add(fakeNamespace(namespace, labels)); err != nil {
			panic(err.Error())
		}
	}
	return corev1listers.NewNamespaceLister(indexer)
}

func fakeIngressLister(name string, config configv1.Ingress) configv1listers.IngressLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := indexer.Add(fakeIngress(name, config)); err != nil {
		panic(err.Error())
	}
	return configv1listers.NewIngressLister(indexer)
}

func fakeRouteLister(routeDetails map[string]map[string]string, spec *routeapi.RouteSpec) routev1listers.RouteLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for rt, namespaceAndLabels := range routeDetails {
		for ns := range namespaceAndLabels {
			if err := indexer.Add(fakeRoute(rt, ns, spec, namespaceAndLabels)); err != nil {
				panic(err.Error())
			}
		}
	}
	return routev1listers.NewRouteLister(indexer)
}
