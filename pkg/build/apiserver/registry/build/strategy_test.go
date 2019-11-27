package build

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	kapi "k8s.io/kubernetes/pkg/apis/core"

	buildapi "github.com/openshift/openshift-apiserver/pkg/build/apis/build"
)

func TestBuildStrategy(t *testing.T) {
	ctx := apirequest.NewDefaultContext()
	if !Strategy.NamespaceScoped() {
		t.Errorf("Build is namespace scoped")
	}
	if Strategy.AllowCreateOnUpdate() {
		t.Errorf("Build should not allow create on update")
	}
	build := &buildapi.Build{
		ObjectMeta: metav1.ObjectMeta{Name: "buildid", Namespace: "default"},
		Spec: buildapi.BuildSpec{
			CommonSpec: buildapi.CommonSpec{
				Source: buildapi.BuildSource{
					Git: &buildapi.GitBuildSource{
						URI: "http://github.com/my/repository",
					},
					ContextDir: "context",
				},
				Strategy: buildapi.BuildStrategy{
					DockerStrategy: &buildapi.DockerBuildStrategy{},
				},
				Output: buildapi.BuildOutput{
					To: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "repository/data",
					},
				},
			},
		},
	}
	Strategy.PrepareForCreate(ctx, build)
	if len(build.Status.Phase) == 0 || build.Status.Phase != buildapi.BuildPhaseNew {
		t.Errorf("Build phase is not New")
	}
	errs := Strategy.Validate(ctx, build)
	if len(errs) != 0 {
		t.Errorf("Unexpected error validating %v", errs)
	}

	build.ResourceVersion = "foo"
	errs = Strategy.ValidateUpdate(ctx, build, build)
	if len(errs) != 0 {
		t.Errorf("Unexpected error validating %v", errs)
	}
	invalidBuild := &buildapi.Build{}
	errs = Strategy.Validate(ctx, invalidBuild)
	if len(errs) == 0 {
		t.Errorf("Expected error validating")
	}
}

type test struct {
	input          *buildapi.Build
	expectedCreate *buildapi.Build
	expectedUpdate *buildapi.Build
}

func TestManageConditions(t *testing.T) {
	now := metav1.Now()
	tests := []test{
		// 0 - empty build, prepare for create should add condition new
		{
			input: &buildapi.Build{},
			expectedCreate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "",
							Message:            "",
						},
					},
				},
			},
			expectedUpdate: &buildapi.Build{},
		},
		// 1 - empty condition array populated w/ current phase
		{
			input: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Phase: buildapi.BuildPhaseNew,
				},
			},
			expectedCreate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "",
							Message:            "",
						},
					},
				},
			},
			expectedUpdate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "",
							Message:            "",
						},
					},
				},
			},
		},
		// 2 - empty condition array populated w/ current phase, reason, message
		{
			input: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Phase:   buildapi.BuildPhaseRunning,
					Reason:  "reason",
					Message: "message",
				},
			},
			expectedCreate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "reason",
							Message:            "message",
						},
					},
				},
			},
			expectedUpdate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "reason",
							Message:            "message",
						},
					},
				},
			},
		},
		// 3 - existing (false) condition untouched, existing true condition transitioned to false,
		// new phase added to conditions.
		{
			input: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Phase:   buildapi.BuildPhaseComplete,
					Reason:  "creason",
					Message: "cmessage",
					Conditions: []buildapi.BuildCondition{
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:  kapi.ConditionFalse,
							Reason:  "nreason",
							Message: "nmessage",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "rreason",
							Message:            "rmessage",
						},
					},
				},
			},
			expectedCreate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:  kapi.ConditionFalse,
							Reason:  "nreason",
							Message: "nmessage",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:             kapi.ConditionFalse,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "",
							Message:            "",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseComplete),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "creason",
							Message:            "cmessage",
						},
					},
				},
			},
			expectedUpdate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:  kapi.ConditionFalse,
							Reason:  "nreason",
							Message: "nmessage",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:             kapi.ConditionFalse,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "",
							Message:            "",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseComplete),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "creason",
							Message:            "cmessage",
						},
					},
				},
			},
		},
		// 4 - existing (false) condition untouched, existing true condition transitioned to false,
		// existing false condition transitioned to true.
		{
			input: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Phase:   buildapi.BuildPhaseComplete,
					Reason:  "creason",
					Message: "cmessage",
					Conditions: []buildapi.BuildCondition{
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:  kapi.ConditionFalse,
							Reason:  "nreason",
							Message: "nmessage",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "rreason",
							Message:            "rmessage",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseComplete),
							Status:             kapi.ConditionFalse,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "creason",
							Message:            "cmessage",
						},
					},
				},
			},
			expectedCreate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:  kapi.ConditionFalse,
							Reason:  "nreason",
							Message: "nmessage",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:             kapi.ConditionFalse,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "",
							Message:            "",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseComplete),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "creason",
							Message:            "cmessage",
						},
					},
				},
			},
			expectedUpdate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:  kapi.ConditionFalse,
							Reason:  "nreason",
							Message: "nmessage",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:             kapi.ConditionFalse,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "",
							Message:            "",
						},
						{
							Type:               buildapi.BuildConditionType(buildapi.BuildPhaseComplete),
							Status:             kapi.ConditionTrue,
							LastUpdateTime:     now,
							LastTransitionTime: now,
							Reason:             "creason",
							Message:            "cmessage",
						},
					},
				},
			},
		},
		// 5 - all existing conditions untouched
		{
			input: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Phase:   buildapi.BuildPhaseComplete,
					Reason:  "creason",
					Message: "cmessage",
					Conditions: []buildapi.BuildCondition{
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:  kapi.ConditionFalse,
							Reason:  "nreason",
							Message: "nmessage",
						},
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:  kapi.ConditionFalse,
							Reason:  "rreason",
							Message: "rmessage",
						},
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseComplete),
							Status:  kapi.ConditionTrue,
							Reason:  "creason",
							Message: "cmessage",
						},
					},
				},
			},
			expectedCreate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:  kapi.ConditionFalse,
							Reason:  "nreason",
							Message: "nmessage",
						},
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:  kapi.ConditionFalse,
							Reason:  "rreason",
							Message: "rmessage",
						},
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseComplete),
							Status:  kapi.ConditionTrue,
							Reason:  "creason",
							Message: "cmessage",
						},
					},
				},
			},
			expectedUpdate: &buildapi.Build{
				Status: buildapi.BuildStatus{
					Conditions: []buildapi.BuildCondition{
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseNew),
							Status:  kapi.ConditionFalse,
							Reason:  "nreason",
							Message: "nmessage",
						},
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseRunning),
							Status:  kapi.ConditionFalse,
							Reason:  "rreason",
							Message: "rmessage",
						},
						{
							Type:    buildapi.BuildConditionType(buildapi.BuildPhaseComplete),
							Status:  kapi.ConditionTrue,
							Reason:  "creason",
							Message: "cmessage",
						},
					},
				},
			},
		},
	}

	for n, test := range tests {
		build := test.input.DeepCopy()
		Strategy.PrepareForCreate(apirequest.NewDefaultContext(), build)

		if len(build.Status.Conditions) != len(test.expectedCreate.Status.Conditions) {
			t.Fatalf("Test[%d]PrepareForCreate] differing number of conditions.  Got:\n%v\nexpected\n%v", n, build.Status.Conditions, test.expectedCreate.Status.Conditions)
		}
		for i, result := range build.Status.Conditions {
			expectedCreate := test.expectedCreate.Status.Conditions[i]
			if result.Type != expectedCreate.Type ||
				result.Status != expectedCreate.Status ||
				result.Reason != expectedCreate.Reason ||
				result.Message != expectedCreate.Message ||
				(!result.LastUpdateTime.IsZero() && expectedCreate.LastUpdateTime.IsZero()) ||
				(!result.LastTransitionTime.IsZero() && expectedCreate.LastTransitionTime.IsZero()) ||
				(!result.LastUpdateTime.IsZero() && !result.LastUpdateTime.After(expectedCreate.LastUpdateTime.Time)) ||
				(!result.LastTransitionTime.IsZero() && !result.LastTransitionTime.After(expectedCreate.LastTransitionTime.Time)) {
				t.Errorf("Test[%d][PrepateForCreate] conditions differed from expected.  Got:\n%v\nexpected\n%v", n, result, expectedCreate)
			}
		}
	}

	for n, test := range tests {
		build := test.input.DeepCopy()
		Strategy.PrepareForUpdate(apirequest.NewDefaultContext(), build, build)

		if len(build.Status.Conditions) != len(test.expectedUpdate.Status.Conditions) {
			t.Fatalf("[%d]PrepareForUpdate] differing number of conditions.  Got:\n%v\nexpected\n%v", n, build.Status.Conditions, test.expectedUpdate.Status.Conditions)
		}
		for i, result := range build.Status.Conditions {
			expectedUpdate := test.expectedUpdate.Status.Conditions[i]
			if result.Type != expectedUpdate.Type ||
				result.Status != expectedUpdate.Status ||
				result.Reason != expectedUpdate.Reason ||
				result.Message != expectedUpdate.Message ||
				(!result.LastUpdateTime.IsZero() && expectedUpdate.LastUpdateTime.IsZero()) ||
				(!result.LastTransitionTime.IsZero() && expectedUpdate.LastTransitionTime.IsZero()) ||
				(!result.LastUpdateTime.IsZero() && !result.LastUpdateTime.After(expectedUpdate.LastUpdateTime.Time)) ||
				(!result.LastTransitionTime.IsZero() && !result.LastTransitionTime.After(expectedUpdate.LastTransitionTime.Time)) {
				t.Errorf("Test[%d][PrepateForUpdate] conditions differed from expected.  Got:\n%v\nexpected\n%v", n, result, expectedUpdate)
			}
		}
	}

}
