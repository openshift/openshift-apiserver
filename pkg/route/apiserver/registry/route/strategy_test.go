package route

import (
	"context"
	"reflect"
	"testing"

	authorizationapi "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/authentication/user"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	testclient "k8s.io/client-go/kubernetes/fake"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/library-go/pkg/route"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

type testAllocator struct {
}

func (t testAllocator) GenerateHostname(*routev1.Route) (string, error) {
	return "mygeneratedhost.com", nil
}

type testSAR struct {
	allow bool
	err   error
	sar   *authorizationapi.SubjectAccessReview
}

func (t *testSAR) Create(_ context.Context, subjectAccessReview *authorizationapi.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationapi.SubjectAccessReview, error) {
	t.sar = subjectAccessReview
	return &authorizationapi.SubjectAccessReview{
		Status: authorizationapi.SubjectAccessReviewStatus{
			Allowed: t.allow,
		},
	}, t.err
}

type testSecretGetter struct {
	namespace string
	secret    *corev1.Secret
}

func (t *testSecretGetter) Secrets(_ string) corev1client.SecretInterface {
	return testclient.NewSimpleClientset(t.secret).CoreV1().Secrets(t.namespace)
}

func TestEmptyHostDefaulting(t *testing.T) {
	ctx := apirequest.NewContext()
	strategy := NewStrategy(testAllocator{}, &testSAR{allow: true}, &testSecretGetter{}, true)

	hostlessCreatedRoute := &routeapi.Route{}
	strategy.Validate(ctx, hostlessCreatedRoute)
	if hostlessCreatedRoute.Spec.Host != "mygeneratedhost.com" {
		t.Fatalf("Expected host to be allocated, got %s", hostlessCreatedRoute.Spec.Host)
	}

	persistedRoute := &routeapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "foo",
			Name:            "myroute",
			UID:             types.UID("abc"),
			ResourceVersion: "1",
		},
		Spec: routeapi.RouteSpec{
			Host: "myhost.com",
		},
	}
	hostlessUpdatedRoute := persistedRoute.DeepCopy()
	hostlessUpdatedRoute.Spec.Host = ""
	strategy.PrepareForUpdate(ctx, hostlessUpdatedRoute, persistedRoute)
	if hostlessUpdatedRoute.Spec.Host != "myhost.com" {
		t.Fatalf("expected empty spec.host to default to existing spec.host, got %s", hostlessUpdatedRoute.Spec.Host)
	}
}

func TestEmptyHostDefaultingWhenSubdomainSet(t *testing.T) {
	ctx := apirequest.NewContext()
	strategy := NewStrategy(testAllocator{}, &testSAR{allow: true}, &testSecretGetter{}, true)

	hostlessCreatedRoute := &routeapi.Route{}
	strategy.Validate(ctx, hostlessCreatedRoute)
	if hostlessCreatedRoute.Spec.Host != "mygeneratedhost.com" {
		t.Fatalf("Expected host to be allocated, got %s", hostlessCreatedRoute.Spec.Host)
	}

	persistedRoute := &routeapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "foo",
			Name:            "myroute",
			UID:             types.UID("abc"),
			ResourceVersion: "1",
		},
		Spec: routeapi.RouteSpec{
			Subdomain: "myhost",
		},
	}
	hostlessUpdatedRoute := persistedRoute.DeepCopy()
	hostlessUpdatedRoute.Spec.Host = ""
	strategy.PrepareForUpdate(ctx, hostlessUpdatedRoute, persistedRoute)
	if hostlessUpdatedRoute.Spec.Host != "" {
		t.Fatalf("expected empty spec.host to remain unset, got %s", hostlessUpdatedRoute.Spec.Host)
	}
}

func TestEmptyDefaultCACertificate(t *testing.T) {
	testCases := []struct {
		route *routeapi.Route
	}{
		{
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       "foo",
					Name:            "myroute",
					UID:             types.UID("abc"),
					ResourceVersion: "1",
				},
				Spec: routeapi.RouteSpec{
					Host: "myhost.com",
				},
			},
		},
	}
	for i, testCase := range testCases {
		copied := testCase.route.DeepCopy()
		if err := DecorateLegacyRouteWithEmptyDestinationCACertificates(copied); err != nil {
			t.Errorf("%d: unexpected error: %v", i, err)
			continue
		}
		routeStrategy{}.PrepareForCreate(nil, copied)
		if !reflect.DeepEqual(testCase.route, copied) {
			t.Errorf("%d: unexpected change: %#v", i, copied)
			continue
		}
		if err := DecorateLegacyRouteWithEmptyDestinationCACertificates(copied); err != nil {
			t.Errorf("%d: unexpected error: %v", i, err)
			continue
		}
		routeStrategy{}.PrepareForUpdate(nil, copied, &routeapi.Route{})
		if !reflect.DeepEqual(testCase.route, copied) {
			t.Errorf("%d: unexpected change: %#v", i, copied)
			continue
		}
	}
}

func TestHostWithWildcardPolicies(t *testing.T) {
	ctx := apirequest.NewContext()
	ctx = apirequest.WithUser(ctx, &user.DefaultInfo{Name: "bob"})
	testNamespace := "wildcard"

	tests := []struct {
		name          string
		host, oldHost string

		subdomain, oldSubdomain string

		wildcardPolicy routeapi.WildcardPolicyType
		tls, oldTLS    *routeapi.TLSConfig

		expected          string
		expectedSubdomain string

		// field for externalCertificate
		opts   route.RouteValidationOptions
		secret *corev1.Secret

		errs  int
		allow bool
	}{
		{
			name:     "no-host-empty-policy",
			expected: "mygeneratedhost.com",
			allow:    true,
		},
		{
			name:           "no-host-nopolicy",
			wildcardPolicy: routeapi.WildcardPolicyNone,
			expected:       "mygeneratedhost.com",
			allow:          true,
		},
		{
			name:           "no-host-wildcard-subdomain",
			wildcardPolicy: routeapi.WildcardPolicySubdomain,
			expected:       "",
			allow:          true,
			errs:           1,
		},
		{
			name:     "host-empty-policy",
			host:     "empty.policy.test",
			expected: "empty.policy.test",
			allow:    true,
		},
		{
			name:           "host-no-policy",
			host:           "no.policy.test",
			wildcardPolicy: routeapi.WildcardPolicyNone,
			expected:       "no.policy.test",
			allow:          true,
		},
		{
			name:           "host-wildcard-subdomain",
			host:           "wildcard.policy.test",
			wildcardPolicy: routeapi.WildcardPolicySubdomain,
			expected:       "wildcard.policy.test",
			allow:          true,
		},
		{
			name:           "custom-host-permission-denied",
			host:           "another.test",
			expected:       "another.test",
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "tls-permission-denied-destination",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt, DestinationCACertificate: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "tls-permission-denied-cert",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "tls-permission-denied-ca-cert",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, CACertificate: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "tls-permission-denied-key",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Key: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "no-host-but-allowed",
			expected:       "mygeneratedhost.com",
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
		},
		{
			name:           "update-changed-host-denied",
			host:           "new.host",
			expected:       "new.host",
			oldHost:        "original.host",
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "update-changed-host-allowed",
			host:           "new.host",
			expected:       "new.host",
			oldHost:        "original.host",
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           0,
		},
		{
			name:              "update-changed-subdomain-denied",
			subdomain:         "new.host",
			expectedSubdomain: "new.host",
			oldSubdomain:      "original.host",
			wildcardPolicy:    routeapi.WildcardPolicyNone,
			allow:             false,
			errs:              1,
		},
		{
			name:              "update-changed-subdomain-allowed",
			subdomain:         "new.host",
			expectedSubdomain: "new.host",
			oldSubdomain:      "original.host",
			wildcardPolicy:    routeapi.WildcardPolicyNone,
			allow:             true,
			errs:              0,
		},
		{
			name:           "key-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Key: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Key: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "key-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Key: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Key: "b"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "certificate-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "certificate-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "b"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "ca-certificate-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, CACertificate: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, CACertificate: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "ca-certificate-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, CACertificate: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, CACertificate: "b"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "key-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Key: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Key: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "key-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Key: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Key: "b"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "destination-ca-certificate-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt, DestinationCACertificate: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt, DestinationCACertificate: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "destination-ca-certificate-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt, DestinationCACertificate: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt, DestinationCACertificate: "b"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "set-to-edge-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge},
			oldTLS:         nil,
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "cleared-edge",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            nil,
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "removed-certificate",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt, Certificate: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "added-certificate-and-fails",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationReencrypt, Certificate: "a"},
			oldTLS:         nil,
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:     "create-with-external-certificate-denied",
			host:     "host",
			expected: "host",
			tls:      &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           4,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:     "create-with-external-certificate-allowed",
			host:     "host",
			expected: "host",
			tls:      &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "create-with-external-certificate-allowed-but-missing-secret",
			host:           "host",
			expected:       "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			secret:         &corev1.Secret{},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:     "create-with-external-certificate-allowed-but-incorrect-secret-type",
			host:     "host",
			expected: "host",
			tls:      &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeBasicAuth,
			},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-changed-to-certificate-denied",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           2,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-changed-to-certificate-allowed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:     "certificate-changed-to-external-certificate-denied",
			host:     "host",
			expected: "host",
			oldHost:  "host",
			tls:      &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			oldTLS:   &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "b",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           5,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:     "certificate-changed-to-external-certificate-allowed",
			host:     "host",
			expected: "host",
			oldHost:  "host",
			tls:      &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			oldTLS:   &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "b",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "certificate-changed-to-external-certificate-allowed-but-missing-secret",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			secret:         &corev1.Secret{},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:     "certificate-changed-to-external-certificate-allowed-but-incorrect-secret-type",
			host:     "host",
			expected: "host",
			oldHost:  "host",
			tls:      &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			oldTLS:   &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "b",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeBasicAuth,
			},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:     "external-certificate-unchanged-denied",
			host:     "host",
			expected: "host",
			oldHost:  "host",
			tls:      &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:   &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           4,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:     "external-certificate-unchanged-allowed",
			host:     "host",
			expected: "host",
			oldHost:  "host",
			tls:      &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:   &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-unchanged-allowed-but-missing-secret",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			secret:         &corev1.Secret{},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:     "external-certificate-unchanged-allowed-but-incorrect-secret-type",
			host:     "host",
			expected: "host",
			oldHost:  "host",
			tls:      &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:   &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeBasicAuth,
			},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-changed-denied",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			allow: false,
			errs:  4,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-changed-allowed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			allow: true,
			errs:  0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-changed-allowed-but-missing-secret",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret:         &corev1.Secret{},
			allow:          true,
			errs:           1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-changed-allowed-but-incorrect-secret-type",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "b"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeBasicAuth,
			},
			allow: true,
			errs:  1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-removed-denied",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			// removing certificate info is allowed
			errs: 0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-removed-allowed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-removed-and-set-nil-tls-denied",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            nil,
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			// removing certificate info is allowed
			errs: 0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-removed-and-set-nil-tls-allowed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            nil,
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-added-denied",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			allow: false,
			errs:  4,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-added-allowed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			allow: true,
			errs:  0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-added-allowed-but-missing-secret",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret:         &corev1.Secret{},
			allow:          true,
			errs:           1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-added-allowed-but-incorrect-secret-type",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeBasicAuth,
			},
			allow: true,
			errs:  1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-added-from-nil-tls-denied",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         nil,
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			allow: false,
			errs:  4,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-added-from-nil-tls-allowed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         nil,
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
			},
			allow: true,
			errs:  0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-added-from-nil-tls-allowed-but-missing-secret",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         nil,
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret:         &corev1.Secret{},
			allow:          true,
			errs:           1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:           "external-certificate-added-from-nil-tls-allowed-but-incorrect-secret-type",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: &routeapi.LocalObjectReference{Name: "a"}},
			oldTLS:         nil,
			wildcardPolicy: routeapi.WildcardPolicyNone,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeBasicAuth,
			},
			allow: true,
			errs:  1,

			opts: route.RouteValidationOptions{AllowExternalCertificates: true},
		},
		{
			name:     "certificate-changed-to-external-certificate-denied-and-featuregate-is-not-set",
			host:     "host",
			expected: "host",
			oldHost:  "host",
			// if the featuregate was disabled, and ExternalCertificate wasn't previously set, apiserver will strip ExternalCertificate field.
			// https://github.com/openshift/openshift-apiserver/blob/1fac5e7e3a6153efae875185af2dba48fbad41ab/pkg/route/apiserver/registry/route/strategy.go#L73-L93
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: nil},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          false,
			errs:           0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: false},
		},
		{
			name:     "certificate-changed-to-external-certificate-allowed-but-featuregate-is-not-set",
			host:     "host",
			expected: "host",
			oldHost:  "host",
			// if the featuregate was disabled, and ExternalCertificate wasn't previously set, apiserver will strip ExternalCertificate field.
			// https://github.com/openshift/openshift-apiserver/blob/1fac5e7e3a6153efae875185af2dba48fbad41ab/pkg/route/apiserver/registry/route/strategy.go#L73-L93
			tls:            &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, ExternalCertificate: nil},
			oldTLS:         &routeapi.TLSConfig{Termination: routeapi.TLSTerminationEdge, Certificate: "a"},
			wildcardPolicy: routeapi.WildcardPolicyNone,
			allow:          true,
			errs:           0,

			opts: route.RouteValidationOptions{AllowExternalCertificates: false},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			route := &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       testNamespace,
					Name:            tc.name,
					UID:             types.UID("wild"),
					ResourceVersion: "1",
				},
				Spec: routeapi.RouteSpec{
					Host:           tc.host,
					Subdomain:      tc.subdomain,
					WildcardPolicy: tc.wildcardPolicy,
					TLS:            tc.tls,
					To: routeapi.RouteTargetReference{
						Name: "test",
						Kind: "Service",
					},
				},
			}

			sar := &testSAR{allow: tc.allow}
			secretGetter := &testSecretGetter{namespace: testNamespace, secret: tc.secret}
			strategy := NewStrategy(testAllocator{}, sar, secretGetter, tc.opts.AllowExternalCertificates)

			var errs field.ErrorList
			if len(tc.oldHost) > 0 || len(tc.oldSubdomain) > 0 || tc.oldTLS != nil {
				oldRoute := &routeapi.Route{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       testNamespace,
						Name:            tc.name,
						UID:             types.UID("wild"),
						ResourceVersion: "1",
					},
					Spec: routeapi.RouteSpec{
						Host:           tc.oldHost,
						Subdomain:      tc.oldSubdomain,
						WildcardPolicy: tc.wildcardPolicy,
						TLS:            tc.oldTLS,
						To: routeapi.RouteTargetReference{
							Name: "test",
							Kind: "Service",
						},
					},
				}
				errs = strategy.ValidateUpdate(ctx, route, oldRoute)
			} else {
				errs = strategy.Validate(ctx, route)
			}

			if route.Spec.Host != tc.expected {
				t.Fatalf("expected host %s, got %s", tc.expected, route.Spec.Host)
			}
			if route.Spec.Subdomain != tc.expectedSubdomain {
				t.Fatalf("expected subdomain %s, got %s", tc.expectedSubdomain, route.Spec.Subdomain)
			}
			if len(errs) != tc.errs {
				t.Logf("wanted %d errors but got %d", tc.errs, len(errs))
				t.Fatalf("unexpected errors: %v %#v", errs, sar)
			}
		})
	}
}

func TestExternalCertRemoval(t *testing.T) {
	ctx := apirequest.NewContext()
	ctx = apirequest.WithUser(ctx, &user.DefaultInfo{Name: "bob"})

	withExternalCert := &routeapi.Route{
		Spec: routeapi.RouteSpec{
			TLS: &routeapi.TLSConfig{
				ExternalCertificate: &routeapi.LocalObjectReference{
					Name: "serving-cert",
				},
			},
		},
	}

	{
		noExternalCertificates := NewStrategy(nil, nil, nil, false)

		freshRoute := withExternalCert.DeepCopy()
		noExternalCertificates.PrepareForCreate(ctx, freshRoute)
		if freshRoute.Spec.TLS.ExternalCertificate != nil {
			t.Errorf("still has external cert")
		}

		// cannot add external certs to routes that don't have them.
		freshNoCertRoute := &routeapi.Route{}
		freshRoute = withExternalCert.DeepCopy()
		noExternalCertificates.PrepareForUpdate(ctx, freshRoute, freshNoCertRoute)
		if freshRoute.Spec.TLS.ExternalCertificate != nil {
			t.Errorf("still has external cert")
		}

		// routes with existing external certificates are allowed to keep them
		freshRoute = withExternalCert.DeepCopy()
		noExternalCertificates.PrepareForUpdate(ctx, freshRoute, freshRoute)
		if freshRoute.Spec.TLS.ExternalCertificate == nil {
			t.Errorf("should have external cert")
		}
	}

	{
		allowExternalCertificates := NewStrategy(nil, nil, nil, true)

		freshRoute := withExternalCert.DeepCopy()
		allowExternalCertificates.PrepareForCreate(ctx, freshRoute)
		if freshRoute.Spec.TLS.ExternalCertificate == nil {
			t.Errorf("should have external cert")
		}

		freshRoute = withExternalCert.DeepCopy()
		allowExternalCertificates.PrepareForUpdate(ctx, freshRoute, freshRoute)
		if freshRoute.Spec.TLS.ExternalCertificate == nil {
			t.Errorf("should have external cert")
		}
	}
}

func TestRouteGenerationManagement(t *testing.T) {
	ctx := apirequest.NewContext()
	strategy := NewStrategy(testAllocator{}, &testSAR{allow: true}, &testSecretGetter{}, true)

	simpleRoute := &routeapi.Route{}
	strategy.Validate(ctx, simpleRoute)
	if simpleRoute.Spec.Host != "mygeneratedhost.com" {
		t.Fatalf("Expected host to be allocated, got %s", simpleRoute.Spec.Host)
	}

	if simpleRoute.Generation > 0 {
		t.Fatalf("Expected generation to not be allocated yet, got %d", simpleRoute.Generation)
	}

	// PrepareForCreate should set a generation 1
	strategy.PrepareForCreate(ctx, simpleRoute)
	if simpleRoute.Generation != 1 {
		t.Fatalf("Expected generation after create to be 1, got %d", simpleRoute.Generation)
	}

	newRoute := simpleRoute.DeepCopy()
	// Changing annotations and labels should not bump the generation
	newRoute.Annotations = map[string]string{
		"someannotation": "novalue",
	}
	newRoute.Labels = map[string]string{
		"somelabel": "novalue",
	}

	strategy.PrepareForUpdate(ctx, newRoute, simpleRoute)
	if newRoute.Generation != 1 {
		t.Fatalf("Expected generation after metadata update to still be 1, got %d", simpleRoute.Generation)
	}

	// Updating the spec should bump the generation
	newRoute.Spec.Path = "/xpto"
	strategy.PrepareForUpdate(ctx, newRoute, simpleRoute)
	if newRoute.Generation != 2 {
		t.Fatalf("Expected generation after spec update to be 2, got %d", simpleRoute.Generation)
	}
}
