package routehostassignment

import (
	"context"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"

	routeinternal "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

type testAllocator struct {
}

func (t testAllocator) GenerateHostname(*routeinternal.Route) (string, error) {
	return "mygeneratedhost.com", nil
}

type testSAR struct {
	allow bool
	err   error
	sar   *authorizationv1.SubjectAccessReview
}

func (t *testSAR) Create(_ context.Context, subjectAccessReview *authorizationv1.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
	t.sar = subjectAccessReview
	return &authorizationv1.SubjectAccessReview{
		Status: authorizationv1.SubjectAccessReviewStatus{
			Allowed: t.allow,
		},
	}, t.err
}

func TestHostWithWildcardPolicies(t *testing.T) {
	ctx := request.NewContext()
	ctx = request.WithUser(ctx, &user.DefaultInfo{Name: "bob"})

	tests := []struct {
		name          string
		host, oldHost string

		subdomain, oldSubdomain string

		wildcardPolicy routeinternal.WildcardPolicyType
		tls, oldTLS    *routeinternal.TLSConfig

		expected          string
		expectedSubdomain string

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
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			expected:       "mygeneratedhost.com",
			allow:          true,
		},
		{
			name:           "no-host-wildcard-subdomain",
			wildcardPolicy: routeinternal.WildcardPolicySubdomain,
			expected:       "",
			allow:          true,
			errs:           0,
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
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			expected:       "no.policy.test",
			allow:          true,
		},
		{
			name:           "host-wildcard-subdomain",
			host:           "wildcard.policy.test",
			wildcardPolicy: routeinternal.WildcardPolicySubdomain,
			expected:       "wildcard.policy.test",
			allow:          true,
		},
		{
			name:           "custom-host-permission-denied",
			host:           "another.test",
			expected:       "another.test",
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "tls-permission-denied-destination",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationReencrypt, DestinationCACertificate: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "tls-permission-denied-cert",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Certificate: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "tls-permission-denied-ca-cert",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, CACertificate: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "tls-permission-denied-key",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Key: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "no-host-but-allowed",
			expected:       "mygeneratedhost.com",
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
		},
		{
			name:           "update-changed-host-denied",
			host:           "new.host",
			expected:       "new.host",
			oldHost:        "original.host",
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "update-changed-host-allowed",
			host:           "new.host",
			expected:       "new.host",
			oldHost:        "original.host",
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          true,
			errs:           0,
		},
		{
			name:              "update-changed-subdomain-denied",
			subdomain:         "new.host",
			expectedSubdomain: "new.host",
			oldSubdomain:      "original.host",
			wildcardPolicy:    routeinternal.WildcardPolicyNone,
			allow:             false,
			errs:              1,
		},
		{
			name:              "update-changed-subdomain-allowed",
			subdomain:         "new.host",
			expectedSubdomain: "new.host",
			oldSubdomain:      "original.host",
			wildcardPolicy:    routeinternal.WildcardPolicyNone,
			allow:             true,
			errs:              0,
		},
		{
			name:           "key-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Key: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Key: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "key-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Key: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Key: "b"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "certificate-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Certificate: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Certificate: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "certificate-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Certificate: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Certificate: "b"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "ca-certificate-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, CACertificate: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, CACertificate: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "ca-certificate-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, CACertificate: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, CACertificate: "b"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "key-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Key: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Key: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "key-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Key: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge, Key: "b"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "destination-ca-certificate-unchanged",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationReencrypt, DestinationCACertificate: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationReencrypt, DestinationCACertificate: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "destination-ca-certificate-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationReencrypt, DestinationCACertificate: "a"},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationReencrypt, DestinationCACertificate: "b"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
		{
			name:           "set-to-edge-changed",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge},
			oldTLS:         nil,
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "cleared-edge",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            nil,
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationEdge},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "removed-certificate",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationReencrypt},
			oldTLS:         &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationReencrypt, Certificate: "a"},
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           0,
		},
		{
			name:           "added-certificate-and-fails",
			host:           "host",
			expected:       "host",
			oldHost:        "host",
			tls:            &routeinternal.TLSConfig{Termination: routeinternal.TLSTerminationReencrypt, Certificate: "a"},
			oldTLS:         nil,
			wildcardPolicy: routeinternal.WildcardPolicyNone,
			allow:          false,
			errs:           1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			route := &routeinternal.Route{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       "wildcard",
					Name:            tc.name,
					UID:             types.UID("wild"),
					ResourceVersion: "1",
				},
				Spec: routeinternal.RouteSpec{
					Host:           tc.host,
					Subdomain:      tc.subdomain,
					WildcardPolicy: tc.wildcardPolicy,
					TLS:            tc.tls,
					To: routeinternal.RouteTargetReference{
						Name: "test",
						Kind: "Service",
					},
				},
			}

			var errs field.ErrorList
			if len(tc.oldHost) > 0 || len(tc.oldSubdomain) > 0 || tc.oldTLS != nil {
				oldRoute := &routeinternal.Route{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "wildcard",
						Name:            tc.name,
						UID:             types.UID("wild"),
						ResourceVersion: "1",
					},
					Spec: routeinternal.RouteSpec{
						Host:           tc.oldHost,
						Subdomain:      tc.oldSubdomain,
						WildcardPolicy: tc.wildcardPolicy,
						TLS:            tc.oldTLS,
						To: routeinternal.RouteTargetReference{
							Name: "test",
							Kind: "Service",
						},
					},
				}
				errs = ValidateHostUpdate(ctx, route, oldRoute, &testSAR{allow: tc.allow})
			} else {
				errs = AllocateHost(ctx, route, &testSAR{allow: tc.allow}, testAllocator{})
			}

			if route.Spec.Host != tc.expected {
				t.Fatalf("expected host %s, got %s", tc.expected, route.Spec.Host)
			}
			if route.Spec.Subdomain != tc.expectedSubdomain {
				t.Fatalf("expected subdomain %s, got %s", tc.expectedSubdomain, route.Spec.Subdomain)
			}
			if len(errs) != tc.errs {
				t.Fatalf("expected %d errors, got %d: %v", tc.errs, len(errs), errs)
			}
		})
	}
}
