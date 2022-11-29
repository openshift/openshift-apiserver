package v1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/pointer"

	v1 "github.com/openshift/api/route/v1"
)

func TestDefaults(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(RegisterDefaults(scheme))

	for _, tc := range []struct {
		name     string
		original v1.Route
		expected v1.Route
	}{
		{
			name: "empty destination ca certificate",
			original: v1.Route{
				Spec: v1.RouteSpec{
					To:  v1.RouteTargetReference{},
					TLS: &v1.TLSConfig{},
				},
				Status: v1.RouteStatus{
					Ingress: []v1.RouteIngress{
						{},
					},
				},
			},
			expected: v1.Route{
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Kind:   "Service",
						Weight: pointer.Int32(100),
					},
					TLS: &v1.TLSConfig{
						Termination: v1.TLSTerminationEdge,
					},
					WildcardPolicy: v1.WildcardPolicyNone,
				},
				Status: v1.RouteStatus{
					Ingress: []v1.RouteIngress{
						{
							WildcardPolicy: v1.WildcardPolicyNone,
						},
					},
				},
			},
		},
		{
			name: "nonempty destination ca certificate",
			original: v1.Route{
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Kind:   "Service",
						Weight: pointer.Int32(100),
					},
					TLS: &v1.TLSConfig{
						DestinationCACertificate: "nonempty",
					},
					WildcardPolicy: v1.WildcardPolicyNone,
				},
			},
			expected: v1.Route{
				Spec: v1.RouteSpec{
					To: v1.RouteTargetReference{
						Kind:   "Service",
						Weight: pointer.Int32(100),
					},
					TLS: &v1.TLSConfig{
						DestinationCACertificate: "nonempty",
					},
					WildcardPolicy: v1.WildcardPolicyNone,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clone := tc.original.DeepCopy()
			scheme.Default(clone)
			if !apiequality.Semantic.DeepEqual(&tc.expected, clone) {
				t.Errorf("expected vs got:\n%s", cmp.Diff(&tc.expected, clone))
			}
		})
	}
}
