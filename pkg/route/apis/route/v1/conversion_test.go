package v1

import (
	"testing"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	v1 "github.com/openshift/api/route/v1"
	"github.com/openshift/openshift-apiserver/pkg/api/apihelpers/apitesting"
	"github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

var scheme = runtime.NewScheme()
var convert = scheme.Convert

func init() {
	Install(scheme)
}

func TestFieldSelectorConversions(t *testing.T) {
	apitesting.FieldKeyCheck{
		SchemeBuilder: []func(*runtime.Scheme) error{Install},
		Kind:          v1.GroupVersion.WithKind("Route"),
		// Ensure previously supported labels have conversions. DO NOT REMOVE THINGS FROM THIS LIST
		AllowedExternalFieldKeys: []string{"spec.host", "spec.path", "spec.to.name"},
		FieldKeyEvaluatorFn:      route.RouteFieldSelector,
	}.Check(t)
}

func TestSupportingCamelConstants(t *testing.T) {
	for k, v := range map[v1.TLSTerminationType]v1.TLSTerminationType{
		"Reencrypt":   v1.TLSTerminationReencrypt,
		"Edge":        v1.TLSTerminationEdge,
		"Passthrough": v1.TLSTerminationPassthrough,
	} {
		obj := &v1.Route{
			Spec: v1.RouteSpec{
				TLS: &v1.TLSConfig{Termination: k},
			},
		}
		scheme.Default(obj)
		if obj.Spec.TLS.Termination != v {
			t.Errorf("%s: did not default termination: %#v", k, spew.Sdump(obj))
		}
	}
}

func setOrDeleteHeadersForVersioned(headers *v1.RouteHTTPHeaders) *v1.RouteSpec {
	serviceName := "TestService"
	serviceWeight := int32(0)
	versionedRouteSpec := &v1.RouteSpec{
		Host: "host",
		Path: "path",
		Port: &v1.RoutePort{
			TargetPort: intstr.FromInt(8080),
		},
		To: v1.RouteTargetReference{
			Name:   serviceName,
			Weight: &serviceWeight,
		},
		TLS: &v1.TLSConfig{
			Termination:              v1.TLSTerminationEdge,
			Certificate:              "abc",
			Key:                      "def",
			CACertificate:            "ghi",
			DestinationCACertificate: "jkl",
		},
	}

	versionedRouteSpec.HTTPHeaders = headers

	return versionedRouteSpec
}

func setOrDeleteHeadersForUnversioned(headers *route.RouteHTTPHeaders) *route.RouteSpec {
	serviceName := "TestService"
	serviceWeight := int32(0)
	unversionedRouteSpec := &route.RouteSpec{
		Host: "host",
		Path: "path",
		Port: &route.RoutePort{
			TargetPort: intstr.FromInt(8080),
		},
		To: route.RouteTargetReference{
			Name:   serviceName,
			Weight: &serviceWeight,
		},
		TLS: &route.TLSConfig{
			Termination:              route.TLSTerminationEdge,
			Certificate:              "abc",
			Key:                      "def",
			CACertificate:            "ghi",
			DestinationCACertificate: "jkl",
		},
	}

	unversionedRouteSpec.HTTPHeaders = headers

	return unversionedRouteSpec
}

func TestV1RouteSpecConversion(t *testing.T) {
	headerNameXFrame := "X-Frame-Options"
	headerNameAccept := "Accept"

	versionedRouteSpecResponseSetHeader := &v1.RouteHTTPHeaders{
		Actions: v1.RouteHTTPHeaderActions{
			Response: []v1.RouteHTTPHeader{
				{
					Name: headerNameXFrame,
					Action: v1.RouteHTTPHeaderActionUnion{
						Type: v1.Set,
						Set: &v1.RouteSetHTTPHeader{
							Value: "DENY",
						},
					},
				},
			},
		},
	}

	versionedRouteSpecResponseDeleteHeader := &v1.RouteHTTPHeaders{
		Actions: v1.RouteHTTPHeaderActions{
			Response: []v1.RouteHTTPHeader{
				{
					Name: headerNameXFrame,
					Action: v1.RouteHTTPHeaderActionUnion{
						Type: v1.Delete,
					},
				},
			},
		},
	}

	unversionedRouteSpecResponseSetHeader := &route.RouteHTTPHeaders{
		Actions: route.RouteHTTPHeaderActions{
			Response: []route.RouteHTTPHeader{
				{
					Name: headerNameXFrame,
					Action: route.RouteHTTPHeaderActionUnion{
						Type: route.Set,
						Set: &route.RouteSetHTTPHeader{
							Value: "DENY",
						},
					},
				},
			},
		},
	}

	unversionedRouteSpecResponseDeleteHeader := &route.RouteHTTPHeaders{
		Actions: route.RouteHTTPHeaderActions{
			Response: []route.RouteHTTPHeader{
				{
					Name: headerNameXFrame,
					Action: route.RouteHTTPHeaderActionUnion{
						Type: route.Delete,
					},
				},
			},
		},
	}

	versionedRouteSpecRequestSetHeader := &v1.RouteHTTPHeaders{
		Actions: v1.RouteHTTPHeaderActions{
			Request: []v1.RouteHTTPHeader{
				{
					Name: headerNameAccept,
					Action: v1.RouteHTTPHeaderActionUnion{
						Type: v1.Set,
						Set: &v1.RouteSetHTTPHeader{
							Value: "text/plain,text/html",
						},
					},
				},
			},
		},
	}

	unversionedRouteSpecRequestSetHeader := &route.RouteHTTPHeaders{
		Actions: route.RouteHTTPHeaderActions{
			Request: []route.RouteHTTPHeader{
				{
					Name: headerNameAccept,
					Action: route.RouteHTTPHeaderActionUnion{
						Type: route.Set,
						Set: &route.RouteSetHTTPHeader{
							Value: "text/plain,text/html",
						},
					},
				},
			},
		},
	}

	versionedRouteSpecRequestDeleteHeader := &v1.RouteHTTPHeaders{
		Actions: v1.RouteHTTPHeaderActions{
			Request: []v1.RouteHTTPHeader{
				{
					Name: headerNameAccept,
					Action: v1.RouteHTTPHeaderActionUnion{
						Type: v1.Delete,
					},
				},
			},
		},
	}

	unversionedRouteSpecRequestDeleteHeader := &route.RouteHTTPHeaders{
		Actions: route.RouteHTTPHeaderActions{
			Request: []route.RouteHTTPHeader{
				{
					Name: headerNameAccept,
					Action: route.RouteHTTPHeaderActionUnion{
						Type: route.Delete,
					},
				},
			},
		},
	}

	testcases := map[string]struct {
		versionedRouteSpec1 *v1.RouteSpec
		internalRouteSpec2  *route.RouteSpec
	}{
		"RouteSpec Conversion 1 when header is a HTTP response and action is Set": {
			versionedRouteSpec1: setOrDeleteHeadersForVersioned(versionedRouteSpecResponseSetHeader),
			internalRouteSpec2:  setOrDeleteHeadersForUnversioned(unversionedRouteSpecResponseSetHeader),
		},
		"RouteSpec Conversion 2 when header is a HTTP request and action is Set": {
			versionedRouteSpec1: setOrDeleteHeadersForVersioned(versionedRouteSpecRequestSetHeader),
			internalRouteSpec2:  setOrDeleteHeadersForUnversioned(unversionedRouteSpecRequestSetHeader),
		},
		"RouteSpec Conversion 3 when header is a HTTP response and action is Delete": {
			versionedRouteSpec1: setOrDeleteHeadersForVersioned(versionedRouteSpecResponseDeleteHeader),
			internalRouteSpec2:  setOrDeleteHeadersForUnversioned(unversionedRouteSpecResponseDeleteHeader),
		},
		"RouteSpec Conversion 4 when header is a HTTP request and action is Delete": {
			versionedRouteSpec1: setOrDeleteHeadersForVersioned(versionedRouteSpecRequestDeleteHeader),
			internalRouteSpec2:  setOrDeleteHeadersForUnversioned(unversionedRouteSpecRequestDeleteHeader),
		},
		"RouteSpec Conversion 1 when header is a HTTP response is nil": {
			versionedRouteSpec1: setOrDeleteHeadersForVersioned(nil),
			internalRouteSpec2:  setOrDeleteHeadersForUnversioned(nil),
		},
	}

	for k, tc := range testcases {
		// un-versioned -> versioned
		internal1 := &v1.RouteSpec{}
		if err := convert(tc.internalRouteSpec2, internal1, nil); err != nil {
			t.Errorf("%q - %q: unexpected error: %v", k, "from route to routev1", err)
		}
		if !apiequality.Semantic.DeepEqual(internal1, tc.versionedRouteSpec1) {
			t.Errorf("%q - %q: diff: %v", k, "from route to routev1", cmp.Diff(tc.versionedRouteSpec1, internal1))
		}

		// versioned -> un-versioned
		internal2 := &route.RouteSpec{}
		if err := convert(tc.versionedRouteSpec1, internal2, nil); err != nil {
			t.Errorf("%q - %q: unexpected error: %v", k, "from routev1 to route", err)
		}
		if !apiequality.Semantic.DeepEqual(internal2, tc.internalRouteSpec2) {
			t.Errorf("%q- %q: diff: %v", k, "from routev1 to route", cmp.Diff(tc.internalRouteSpec2, internal2))
		}
	}
}
