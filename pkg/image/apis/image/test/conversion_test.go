package v1

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/apis/core"

	"github.com/openshift/api/image/v1"
	newer "github.com/openshift/openshift-apiserver/pkg/image/apis/image"

	_ "github.com/openshift/openshift-apiserver/pkg/image/apis/image/install"
)

func TestImageStreamStatusConversionPreservesTags(t *testing.T) {
	now := time.Now()

	in := &newer.ImageStreamStatus{
		Tags: map[string]newer.TagEventList{
			"v3.5.0": {
				Conditions: []newer.TagEventCondition{
					{
						Type:               newer.ImportSuccess,
						Status:             core.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						Reason:             "reason",
						Message:            "message",
						Generation:         2,
					},
					{
						Type:               newer.ImportSuccess,
						Status:             core.ConditionFalse,
						LastTransitionTime: metav1.NewTime(now),
						Reason:             "no-reason",
						Message:            "error",
						Generation:         4,
					},
				},
				Items: []newer.TagEvent{
					{
						Created:              metav1.NewTime(now),
						DockerImageReference: "xyz",
						Image:                "abc",
						Generation:           64,
					},
				},
			},
			"3.5.0": {},
		},
	}
	expOutVersioned := &v1.ImageStreamStatus{
		Tags: []v1.NamedTagEventList{
			{
				Tag: "3.5.0",
			},
			{
				Tag: "v3.5.0",
				Conditions: []v1.TagEventCondition{
					{
						Type:               v1.ImportSuccess,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						Reason:             "reason",
						Message:            "message",
						Generation:         2,
					},
					{
						Type:               v1.ImportSuccess,
						Status:             corev1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(now),
						Reason:             "no-reason",
						Message:            "error",
						Generation:         4,
					},
				},
				Items: []v1.TagEvent{
					{
						Created:              metav1.NewTime(now),
						DockerImageReference: "xyz",
						Image:                "abc",
						Generation:           64,
					},
				},
			},
		},
	}

	outVersioned := v1.ImageStreamStatus{Tags: []v1.NamedTagEventList{}}
	err := legacyscheme.Scheme.Convert(in, &outVersioned, nil)
	if err != nil {
		t.Fatalf("got unexpected error: %v", err)
	}
	if a, e := &outVersioned, expOutVersioned; !reflect.DeepEqual(a, e) {
		t.Fatalf("got unexpected output: %s", diff.ObjectDiff(a, e))
	}

	// convert back from v1 to internal scheme
	out := newer.ImageStreamStatus{}
	err = legacyscheme.Scheme.Convert(&outVersioned, &out, nil)
	if err != nil {
		t.Fatalf("got unexpected error: %v", err)
	}
	if a, e := &out, in; !reflect.DeepEqual(a, e) {
		t.Fatalf("got unexpected output: %s", diff.ObjectDiff(a, e))
	}
}
