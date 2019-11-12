package validation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	routev1 "github.com/openshift/api/route/v1"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

const (
	testCACertificate = `-----BEGIN CERTIFICATE-----
MIIClDCCAf2gAwIBAgIJAPU57OGhuqJtMA0GCSqGSIb3DQEBCwUAMGMxCzAJBgNV
BAYTAlVTMQswCQYDVQQIDAJDQTERMA8GA1UECgwIU2VjdXJpdHkxGzAZBgNVBAsM
Ek9wZW5TaGlmdDMgdGVzdCBDQTEXMBUGA1UEAwwOaGVhZGVyLnRlc3QgQ0EwHhcN
MTYwMzEyMDQyMTAzWhcNMzYwMzEyMDQyMTAzWjBjMQswCQYDVQQGEwJVUzELMAkG
A1UECAwCQ0ExETAPBgNVBAoMCFNlY3VyaXR5MRswGQYDVQQLDBJPcGVuU2hpZnQz
IHRlc3QgQ0ExFzAVBgNVBAMMDmhlYWRlci50ZXN0IENBMIGfMA0GCSqGSIb3DQEB
AQUAA4GNADCBiQKBgQCsdVIJ6GSrkFdE9LzsMItYGE4q3qqSqIbs/uwMoVsMT+33
pLeyzeecPuoQsdO6SEuqhUM1ivUN4GyXIR1+aW2baMwMXpjX9VIJu5d4FqtGi6SD
RfV+tbERWwifPJlN+ryuvqbbDxrjQeXhemeo7yrJdgJ1oyDmoM5pTiSUUmltvQID
AQABo1AwTjAdBgNVHQ4EFgQUOVuieqGfp2wnKo7lX2fQt+Yk1C4wHwYDVR0jBBgw
FoAUOVuieqGfp2wnKo7lX2fQt+Yk1C4wDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0B
AQsFAAOBgQA8VhmNeicRnKgXInVyYZDjL0P4WRbKJY7DkJxRMRWxikbEVHdySki6
jegpqgJqYbzU6EiuTS2sl2bAjIK9nGUtTDt1PJIC1Evn5Q6v5ylNflpv6GxtUbCt
bGvtpjWA4r9WASIDPFsxk/cDEEEO6iPxgMOf5MdpQC2y2MU0rzF/Gg==
-----END CERTIFICATE-----`

	testDestinationCACertificate = testCACertificate
)

func createRouteSpecTo(name string, kind string) routeapi.RouteTargetReference {
	svc := routeapi.RouteTargetReference{
		Name: name,
		Kind: kind,
	}
	return svc
}

// Context around testing hostname validation.
// Currently the host name validation checks along these lines
//
// DNS 1123 subdomain
// - host name ^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$
// - and not greater than 253 characters
//
// The additional test (which now aligns with the router hostname validation) is
// DNS 1123 label
// - host name label ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
// - and not greater than 63 characters
//
// The above check can be bypassed by setting the annotation for backwards compatibility
// - route.openshift.io/allow-non-dns-compliant-host: "true"

// N.B. For tests that have the AllowNonDNSCompliantHostAnnotation annotation set to true
// - All tests return the expected behavior of the current api-server
// - The ONLY exception is the test for labels greater than 63 i.e -> name: "Valid host (64 chars label annotation override)"
// - The behavior is as follows
//   - annotation set to false (default) test name: "Valid host (64 chars label annotation override)" has expectedErrors > 0
//   - annotation set to true            test name: "Valid host (64 chars label annotation override)" has expectedErrors == 0
//
// As mentioned this allows for the edge case where customers were using DNS labels greater than 64 chars and were not using the openshift router.

// TestValidateRoute ensures not specifying a required field results in error and a fully specified
// route passes successfully
func TestValidateRoute(t *testing.T) {
	tests := []struct {
		name           string
		route          *routeapi.Route
		expectedErrors int
	}{
		{
			name: "No Name",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "host",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "No namespace",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name: "name",
				},
				Spec: routeapi.RouteSpec{
					Host: "host",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "No host",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					To: createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid host",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "**",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Valid single label host",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "test",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid host (start & end alpha)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid host (start & end numeric)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "1.test.com.2",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid host (trailing '.')",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.test.com.",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Invalid host ('*' not allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.*.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Invalid host ('%!&#@$^' not allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.%!&#@$^.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Invalid host ('A-Z' not allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "A.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Invalid host (trailing '-' not allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.test.com.-",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Valid host (many segements/labels allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "x.abc.y.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid host 63 chars label",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace-1234567890-1234567890-1234567890-1234567890-1234.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid host (64 chars label annotation override)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace-1234567890-1234567890-1234567890-1234567890-12345.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid host (253 chars)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace.a1234567890.b1234567890.c1234567890.d1234567890.e1234567890.f1234567890.g1234567890.h1234567890.i1234567890.j1234567890.k1234567890.l1234567890.m1234567890.n1234567890.o1234567890.p1234567890.q1234567890.r1234567890.s12345678.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid host (279 chars)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace.a1234567890.b1234567890.c1234567890.d1234567890.e1234567890.f1234567890.g1234567890.h1234567890.i1234567890.j1234567890.k1234567890.l1234567890.m1234567890.n1234567890.o1234567890.p1234567890.q1234567890.r1234567890.s1234567890.t1234567890.u1234567890.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Invalid host (does not conform DNS host name)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "**",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 2,
		},
		{
			name: "Valid single label host (conform DNS host name)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "test",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid host (conform DNS host name start & end alpha)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid host (conform DNS host name - start & end numeric)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "1.abc.test.com.2",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid host (does not conform DNS host name - trailing '.')",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.test.com.",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 2,
		},
		{
			name: "Invalid host (does not conform DNS host name - '*' not allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.*.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 2,
		},
		{
			name: "Invalid host (does not conform DNS host name - '%!&#@$^' not allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.%!&#@$^.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 2,
		},
		{
			name: "Invalid host (does not conform DNS host name - 'A-Z' not allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "A.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 2,
		},
		{
			name: "Invalid host (does not conform DNS host name - trailing '-' not allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.test.com.-",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 2,
		},
		{
			name: "Valid host (conform DNS host name - many segments/labels allowed)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "x.abc.y.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid host (conform DNS host name - 63 chars label)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace-1234567890-1234567890-1234567890-1234567890-1234.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid host (does not conform  DNS host name - 64 chars label)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace-1234567890-1234567890-1234567890-1234567890-12345.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Valid host (conform DNS host name - 253 chars)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace.a1234567890.b1234567890.c1234567890.d1234567890.e1234567890.f1234567890.g1234567890.h1234567890.i1234567890.j1234567890.k1234567890.l1234567890.m1234567890.n1234567890.o1234567890.p1234567890.q1234567890.r1234567890.s123456789.t1.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid host (does conform DNS host name - 254 chars)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "false",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace.a1234567890.b1234567890.c1234567890.d1234567890.e1234567890.f1234567890.g1234567890.h1234567890.i1234567890.j1234567890.k1234567890.l1234567890.m1234567890.n1234567890.o1234567890.p1234567890.q1234567890.r1234567890.s1234567890.t12.test.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Valid subdomain",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Subdomain: "api.ci",
					To:        createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid subdomain (253 chars)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Subdomain: "name-namespace.a1234567890.b1234567890.c1234567890.d1234567890.e1234567890.f1234567890.g1234567890.h1234567890.i1234567890.j1234567890.k1234567890.l1234567890.m1234567890.n1234567890.o1234567890.p1234567890.q1234567890.r1234567890.s12345678.test.com",
					To:        createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid subdomain (279 chars)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Subdomain: "name-namespace.a1234567890.b1234567890.c1234567890.d1234567890.e1234567890.f1234567890.g1234567890.h1234567890.i1234567890.j1234567890.k1234567890.l1234567890.m1234567890.n1234567890.o1234567890.p1234567890.q1234567890.r1234567890.s1234567890.t1234567890.u1234567890.test.com",
					To:        createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Invalid DNS 952 subdomain",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Subdomain: "**",
					To:        createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Valid subdomain (conform DNS host name - 253 chars)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Subdomain: "name-namespace.a1234567890.b1234567890.c1234567890.d1234567890.e1234567890.f1234567890.g1234567890.h1234567890.i1234567890.j1234567890.k1234567890.l1234567890.m1234567890.n1234567890.o1234567890.p1234567890.q1234567890.r1234567890.s123456789.t1.test.com",
					To:        createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid subdomain (does conform DNS host name - 254 chars)",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "false",
					},
				},
				Spec: routeapi.RouteSpec{
					Subdomain: "name-namespace.a1234567890.b1234567890.c1234567890.d1234567890.e1234567890.f1234567890.g1234567890.h1234567890.i1234567890.j1234567890.k1234567890.l1234567890.m1234567890.n1234567890.o1234567890.p1234567890.q1234567890.r1234567890.s1234567890.t12.test.com",
					To:        createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "No service name",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "host",
					To:   createRouteSpecTo("", "Service"),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "No service kind",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "host",
					To:   createRouteSpecTo("serviceName", ""),
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Zero port",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "www.example.com",
					To:   createRouteSpecTo("serviceName", "Service"),
					Port: &routeapi.RoutePort{
						TargetPort: intstr.FromInt(0),
					},
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Empty string port",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "www.example.com",
					To:   createRouteSpecTo("serviceName", "Service"),
					Port: &routeapi.RoutePort{
						TargetPort: intstr.FromString(""),
					},
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Valid route",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "www.example.com",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Valid route with path",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "www.example.com",
					To:   createRouteSpecTo("serviceName", "Service"),
					Path: "/test",
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid route with path",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "www.example.com",
					To:   createRouteSpecTo("serviceName", "Service"),
					Path: "test",
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Passthrough route with path",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "www.example.com",
					Path: "/test",
					To:   createRouteSpecTo("serviceName", "Service"),
					TLS: &routeapi.TLSConfig{
						Termination: routeapi.TLSTerminationPassthrough,
					},
				},
			},
			expectedErrors: 1,
		},
		{
			name: "No wildcard policy",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nowildcard",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host: "no.wildcard.test",
					To:   createRouteSpecTo("serviceName", "Service"),
				},
			},
			expectedErrors: 0,
		},
		{
			name: "wildcard policy none",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nowildcard2",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host:           "none.wildcard.test",
					To:             createRouteSpecTo("serviceName", "Service"),
					WildcardPolicy: routeapi.WildcardPolicyNone,
				},
			},
			expectedErrors: 0,
		},
		{
			name: "wildcard policy subdomain",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wildcardpolicy",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host:           "subdomain.wildcard.test",
					To:             createRouteSpecTo("serviceName", "Service"),
					WildcardPolicy: routeapi.WildcardPolicySubdomain,
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Invalid wildcard policy",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "badwildcard",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host:           "bad.wildcard.test",
					To:             createRouteSpecTo("serviceName", "Service"),
					WildcardPolicy: "bad-wolf",
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Invalid host for wildcard policy",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "badhost",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					To:             createRouteSpecTo("serviceName", "Service"),
					WildcardPolicy: routeapi.WildcardPolicySubdomain,
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Empty host for wildcard policy",
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "emptyhost",
					Namespace: "foo",
				},
				Spec: routeapi.RouteSpec{
					Host:           "",
					To:             createRouteSpecTo("serviceName", "Service"),
					WildcardPolicy: routeapi.WildcardPolicySubdomain,
				},
			},
			expectedErrors: 1,
		},
	}

	for _, tc := range tests {
		errs := ValidateRoute(tc.route)
		if len(errs) != tc.expectedErrors {
			t.Errorf("Test case %s expected %d error(s), got %d. %v", tc.name, tc.expectedErrors, len(errs), errs)
		}
	}
}

func TestValidateTLS(t *testing.T) {
	tests := []struct {
		name           string
		route          *routeapi.Route
		expectedErrors int
	}{
		{
			name: "No TLS Termination",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination: "",
					},
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Passthrough termination OK",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination: routeapi.TLSTerminationPassthrough,
					},
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Reencrypt termination OK with certs",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination:              routeapi.TLSTerminationReencrypt,
						Certificate:              "def",
						Key:                      "ghi",
						CACertificate:            "jkl",
						DestinationCACertificate: "abc",
					},
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Reencrypt termination OK without certs",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination:              routeapi.TLSTerminationReencrypt,
						DestinationCACertificate: "abc",
					},
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Reencrypt termination no dest cert",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination:   routeapi.TLSTerminationReencrypt,
						Certificate:   "def",
						Key:           "ghi",
						CACertificate: "jkl",
					},
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Edge termination OK with certs",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination:   routeapi.TLSTerminationEdge,
						Certificate:   "abc",
						Key:           "abc",
						CACertificate: "abc",
					},
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Edge termination OK without certs",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination: routeapi.TLSTerminationEdge,
					},
				},
			},
			expectedErrors: 0,
		},
		{
			name: "Edge termination, dest cert",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination:              routeapi.TLSTerminationEdge,
						DestinationCACertificate: "abc",
					},
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Passthrough termination, cert",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{Termination: routeapi.TLSTerminationPassthrough, Certificate: "test"},
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Passthrough termination, key",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{Termination: routeapi.TLSTerminationPassthrough, Key: "test"},
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Passthrough termination, ca cert",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{Termination: routeapi.TLSTerminationPassthrough, CACertificate: "test"},
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Passthrough termination, dest ca cert",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{Termination: routeapi.TLSTerminationPassthrough, DestinationCACertificate: "test"},
				},
			},
			expectedErrors: 1,
		},
		{
			name: "Invalid termination type",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination: "invalid",
					},
				},
			},
			expectedErrors: 1,
		},
	}

	for _, tc := range tests {
		errs := validateTLS(tc.route, nil)

		if len(errs) != tc.expectedErrors {
			t.Errorf("Test case %s expected %d error(s), got %d. %v", tc.name, tc.expectedErrors, len(errs), errs)
		}
	}
}

func TestValidatePassthroughInsecureEdgeTerminationPolicy(t *testing.T) {

	insecureTypes := map[routeapi.InsecureEdgeTerminationPolicyType]bool{
		"": false,
		routeapi.InsecureEdgeTerminationPolicyNone:     false,
		routeapi.InsecureEdgeTerminationPolicyAllow:    true,
		routeapi.InsecureEdgeTerminationPolicyRedirect: false,
		"support HTTPsec": true,
		"or maybe HSTS":   true,
	}

	for key, expected := range insecureTypes {
		route := &routeapi.Route{
			Spec: routeapi.RouteSpec{
				TLS: &routeapi.TLSConfig{
					Termination:                   routeapi.TLSTerminationPassthrough,
					InsecureEdgeTerminationPolicy: key,
				},
			},
		}
		route.Spec.TLS.InsecureEdgeTerminationPolicy = key
		errs := validateTLS(route, nil)
		if !expected && len(errs) != 0 {
			t.Errorf("Test case for Passthrough termination with insecure=%s got %d errors where none where expected. %v",
				key, len(errs), errs)
		}
		if expected && len(errs) == 0 {
			t.Errorf("Test case for Passthrough termination with insecure=%s got no errors where some where expected.", key)
		}
	}
}

// TestValidateRouteBad ensures not specifying a required field results in error and a fully specified
// route passes successfully
func TestValidateRouteUpdate(t *testing.T) {
	tests := []struct {
		name           string
		route          *routeapi.Route
		change         func(route *routeapi.Route)
		expectedErrors int
	}{
		{
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar",
					Namespace:       "foo",
					ResourceVersion: "1",
				},
				Spec: routeapi.RouteSpec{
					Host: "host",
					To: routeapi.RouteTargetReference{
						Name: "serviceName",
						Kind: "Service",
					},
				},
			},
			change:         func(route *routeapi.Route) { route.Spec.Host = "" },
			expectedErrors: 0, // now controlled by rbac
		},
		{
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar",
					Namespace:       "foo",
					ResourceVersion: "1",
				},
				Spec: routeapi.RouteSpec{
					Host: "host",
					To: routeapi.RouteTargetReference{
						Name: "serviceName",
						Kind: "Service",
					},
				},
			},
			change:         func(route *routeapi.Route) { route.Spec.Host = "other" },
			expectedErrors: 0, // now controlled by rbac
		},
		{
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar",
					Namespace:       "foo",
					ResourceVersion: "1",
				},
				Spec: routeapi.RouteSpec{
					Host: "host",
					To: routeapi.RouteTargetReference{
						Name: "serviceName",
						Kind: "Service",
					},
				},
			},
			change:         func(route *routeapi.Route) { route.Name = "baz" },
			expectedErrors: 1,
		},
		{
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar",
					Namespace:       "foo",
					ResourceVersion: "1",
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace-1234567890-1234567890-1234567890-1234567890-12345.test.com",
					To: routeapi.RouteTargetReference{
						Name: "serviceName",
						Kind: "Service",
					},
				},
			},
			change: func(route *routeapi.Route) {
				route.Spec.Host = "abc.test.com"
			}, // old route was invalid - ignore validation check
			expectedErrors: 0,
		},
		{
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar",
					Namespace:       "foo",
					ResourceVersion: "1",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "name-namespace-1234567890-1234567890-1234567890-1234567890-12345.test.com",
					To: routeapi.RouteTargetReference{
						Name: "serviceName",
						Kind: "Service",
					},
				},
			},
			change: func(route *routeapi.Route) {
				route.Spec.Host = "abc.test.com"
			}, // old route was invalid - ignore validation check even if annoatation is set
			expectedErrors: 0,
		},
		{
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar",
					Namespace:       "foo",
					ResourceVersion: "1",
					Annotations: map[string]string{
						routev1.AllowNonDNSCompliantHostAnnotation: "true",
					},
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.test.com",
					To: routeapi.RouteTargetReference{
						Name: "serviceName",
						Kind: "Service",
					},
				},
			},
			change: func(route *routeapi.Route) {
				route.Spec.Host = "name-namespace-1234567890-1234567890-1234567890-1234567890-12345.test.com"
			}, // new route is invalid - skip check as annotation is set
			expectedErrors: 0,
		},
		{
			route: &routeapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar",
					Namespace:       "foo",
					ResourceVersion: "1",
				},
				Spec: routeapi.RouteSpec{
					Host: "abc.test.com",
					To: routeapi.RouteTargetReference{
						Name: "serviceName",
						Kind: "Service",
					},
				},
			},
			change: func(route *routeapi.Route) {
				route.Spec.Host = "name-namespace-1234567890-1234567890-1234567890-1234567890-12345.test.com"
			}, // new route is invalid - do labels check
			expectedErrors: 1,
		},
	}

	for i, tc := range tests {
		newRoute := tc.route.DeepCopy()
		tc.change(newRoute)
		errs := ValidateRouteUpdate(newRoute, tc.route)
		if len(errs) != tc.expectedErrors {
			t.Errorf("%d: expected %d error(s), got %d. %v", i, tc.expectedErrors, len(errs), errs)
		}
	}
}

func TestValidateInsecureEdgeTerminationPolicy(t *testing.T) {
	tests := []struct {
		name           string
		insecure       routeapi.InsecureEdgeTerminationPolicyType
		expectedErrors int
	}{
		{
			name:           "empty insecure option",
			insecure:       "",
			expectedErrors: 0,
		},
		{
			name:           "foobar insecure option",
			insecure:       "foobar",
			expectedErrors: 1,
		},
		{
			name:           "insecure option none",
			insecure:       routeapi.InsecureEdgeTerminationPolicyNone,
			expectedErrors: 0,
		},
		{
			name:           "insecure option allow",
			insecure:       routeapi.InsecureEdgeTerminationPolicyAllow,
			expectedErrors: 0,
		},
		{
			name:           "insecure option redirect",
			insecure:       routeapi.InsecureEdgeTerminationPolicyRedirect,
			expectedErrors: 0,
		},
		{
			name:           "insecure option other",
			insecure:       "something else",
			expectedErrors: 1,
		},
	}

	for _, tc := range tests {
		route := &routeapi.Route{
			Spec: routeapi.RouteSpec{
				TLS: &routeapi.TLSConfig{
					Termination:                   routeapi.TLSTerminationEdge,
					InsecureEdgeTerminationPolicy: tc.insecure,
				},
			},
		}
		errs := validateTLS(route, nil)

		if len(errs) != tc.expectedErrors {
			t.Errorf("Test case %s expected %d error(s), got %d. %v", tc.name, tc.expectedErrors, len(errs), errs)
		}
	}
}

func TestValidateEdgeReencryptInsecureEdgeTerminationPolicy(t *testing.T) {
	tests := []struct {
		name  string
		route *routeapi.Route
	}{
		{
			name: "Reencrypt termination",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination:              routeapi.TLSTerminationReencrypt,
						DestinationCACertificate: "dca",
					},
				},
			},
		},
		{
			name: "Reencrypt termination DestCACert",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination:              routeapi.TLSTerminationReencrypt,
						DestinationCACertificate: testDestinationCACertificate,
					},
				},
			},
		},
		{
			name: "Edge termination",
			route: &routeapi.Route{
				Spec: routeapi.RouteSpec{
					TLS: &routeapi.TLSConfig{
						Termination: routeapi.TLSTerminationEdge,
					},
				},
			},
		},
	}

	insecureTypes := map[routeapi.InsecureEdgeTerminationPolicyType]bool{
		routeapi.InsecureEdgeTerminationPolicyNone:     false,
		routeapi.InsecureEdgeTerminationPolicyAllow:    false,
		routeapi.InsecureEdgeTerminationPolicyRedirect: false,
		"support HTTPsec": true,
		"or maybe HSTS":   true,
	}

	for _, tc := range tests {
		for key, expected := range insecureTypes {
			tc.route.Spec.TLS.InsecureEdgeTerminationPolicy = key
			errs := validateTLS(tc.route, nil)
			if !expected && len(errs) != 0 {
				t.Errorf("Test case %s with insecure=%s got %d errors where none were expected. %v",
					tc.name, key, len(errs), errs)
			}
			if expected && len(errs) == 0 {
				t.Errorf("Test case %s  with insecure=%s got no errors where some were expected.", tc.name, key)
			}
		}
	}
}
