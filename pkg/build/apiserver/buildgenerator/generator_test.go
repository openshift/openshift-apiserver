package buildgenerator

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/apitesting"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	"github.com/openshift/library-go/pkg/build/buildutil"
	"github.com/openshift/openshift-apiserver/pkg/bootstrappolicy"
	buildapi "github.com/openshift/openshift-apiserver/pkg/build/apis/build"
	buildconversionsv1 "github.com/openshift/openshift-apiserver/pkg/build/apis/build/v1"
	"github.com/openshift/openshift-apiserver/pkg/build/apis/build/validation"
	"github.com/openshift/openshift-apiserver/pkg/build/apiserver/apiserverbuildutil"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	scheme, _ = apitesting.SchemeForOrDie(buildconversionsv1.Install)
}

const (
	originalImage = "originalimage"
	newImage      = originalImage + ":" + newTag

	tagName = "test"

	// immutable imageid associated w/ test tag
	newTag = "123"

	imageRepoName      = "testRepo"
	imageRepoNamespace = "testns"

	dockerReference       = "dockerReference"
	latestDockerReference = "latestDockerReference"
)

func TestInstantiate(t *testing.T) {
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	_, err := generator.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
}

func TestInstantiateBinary(t *testing.T) {
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	build, err := generator.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{Binary: &buildv1.BinaryBuildSource{}}, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	if build.Spec.Source.Binary == nil {
		t.Errorf("build should have a binary source value, has nil")
	}
	build, err = generator.Clone(apirequest.NewDefaultContext(), &buildv1.BuildRequest{Binary: &buildv1.BinaryBuildSource{}})
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	// TODO: we should enable this flow.
	if build.Spec.Source.Binary != nil {
		t.Errorf("build should not have a binary source value, has %v", build.Spec.Source.Binary)
	}
}

// TODO(agoldste): I'm not sure the intent of this test. Using the previous logic for
// the generator, which would try to update the build config before creating
// the build, I can see why the UpdateBuildConfigFunc is set up to return an
// error, but nothing is checking the value of instantiationCalls. We could
// update this test to fail sooner, when the build is created, but that's
// already handled by TestCreateBuildCreateError. We may just want to delete
// this test.
/*
func TestInstantiateRetry(t *testing.T) {
	instantiationCalls := 0
	fakeSecrets := []runtime.Object{}
	for _, s := range MockBuilderSecrets() {
		fakeSecrets = append(fakeSecrets, s)
	}
	generator := BuildGenerator{
		Secrets:         testclient.NewSimpleFake(fakeSecrets...),
		ServiceAccounts: MockBuilderServiceAccount(MockBuilderSecrets()),
		TestingClient: TestingClient{
			GetBuildConfigFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return MockBuildConfig(MockSource(), MockSourceStrategyForImageRepository(), MockOutput()), nil
			},
			UpdateBuildConfigFunc: func(ctx context.Context, buildConfig *buildv1.BuildConfig) error {
				instantiationCalls++
				return fmt.Errorf("update-error")
			},
		}}

	_, err := generator.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	if err == nil || !strings.Contains(err.Error(), "update-error") {
		t.Errorf("Expected update-error, got different %v", err)
	}
}
*/

func TestInstantiateDeletingError(t *testing.T) {
	source := MockSource()
	generator := BuildGenerator{Client: TestingClient{
		GetBuildConfigFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
			bc := &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						buildv1.BuildConfigPausedAnnotation: "true",
					},
				},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Source: source,
						Revision: &buildv1.SourceRevision{
							Git: &buildv1.GitSourceRevision{
								Commit: "1234",
							},
						},
					},
				},
			}
			return bc, nil
		},
		GetBuildFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.Build, error) {
			build := &buildv1.Build{
				Spec: buildv1.BuildSpec{
					CommonSpec: buildv1.CommonSpec{
						Source: source,
						Revision: &buildv1.SourceRevision{
							Git: &buildv1.GitSourceRevision{
								Commit: "1234",
							},
						},
					},
				},
				Status: buildv1.BuildStatus{
					Config: &corev1.ObjectReference{
						Name: "buildconfig",
					},
				},
			}
			return build, nil
		},
	}}
	_, err := generator.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	if err == nil || !strings.Contains(err.Error(), "BuildConfig is paused") {
		t.Errorf("Expected error, got different %v", err)
	}
	_, err = generator.Clone(apirequest.NewDefaultContext(), &buildv1.BuildRequest{})
	if err == nil || !strings.Contains(err.Error(), "BuildConfig is paused") {
		t.Errorf("Expected error, got different %v", err)
	}
}

// TestInstantiateBinaryClear ensures that when instantiating or cloning from a buildconfig/build
// that has a binary source value, the resulting build does not have a binary source value
// (because the request did not include one)
func TestInstantiateBinaryRemoved(t *testing.T) {
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	client := generator.Client.(TestingClient)
	client.GetBuildConfigFunc = func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
		bc := &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Source: buildv1.BuildSource{
						Binary: &buildv1.BinaryBuildSource{},
					},
				},
			},
		}
		return bc, nil
	}
	client.GetBuildFunc = func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.Build, error) {
		build := &buildv1.Build{
			Spec: buildv1.BuildSpec{
				CommonSpec: buildv1.CommonSpec{
					Source: buildv1.BuildSource{
						Binary: &buildv1.BinaryBuildSource{},
					},
				},
			},
			Status: buildv1.BuildStatus{
				Config: &corev1.ObjectReference{
					Name: "buildconfig",
				},
			},
		}
		return build, nil
	}

	build, err := generator.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	if build.Spec.Source.Binary != nil {
		t.Errorf("build should not have a binary source value, has %v", build.Spec.Source.Binary)
	}
	build, err = generator.Clone(apirequest.NewDefaultContext(), &buildv1.BuildRequest{})
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	if build.Spec.Source.Binary != nil {
		t.Errorf("build should not have a binary source value, has %v", build.Spec.Source.Binary)
	}
}

func TestInstantiateGetBuildConfigError(t *testing.T) {
	generator := BuildGenerator{Client: TestingClient{
		GetBuildConfigFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
			return nil, fmt.Errorf("get-error")
		},
		GetImageStreamFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error) {
			return nil, fmt.Errorf("get-error")
		},
		GetImageStreamImageFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamImage, error) {
			return nil, fmt.Errorf("get-error")
		},
		GetImageStreamTagFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamTag, error) {
			return nil, fmt.Errorf("get-error")
		},
	}}

	_, err := generator.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	if err == nil || !strings.Contains(err.Error(), "get-error") {
		t.Errorf("Expected get-error, got different %v", err)
	}
}

func TestInstantiateGenerateBuildError(t *testing.T) {
	fakeSecrets := []runtime.Object{}
	for _, s := range MockBuilderSecrets() {
		fakeSecrets = append(fakeSecrets, s)
	}
	generator := BuildGenerator{
		Secrets:         fake.NewSimpleClientset(fakeSecrets...).CoreV1(),
		ServiceAccounts: MockBuilderServiceAccount(MockBuilderSecrets()),
		Client: TestingClient{
			GetBuildConfigFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return nil, fmt.Errorf("get-error")
			},
		}}

	_, err := generator.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	if err == nil || !strings.Contains(err.Error(), "get-error") {
		t.Errorf("Expected get-error, got different %v", err)
	}
}

func TestInstantiateWithImageTrigger(t *testing.T) {
	imageID := "the-imagev1-id-12345"
	defaultTriggers := func() []buildv1.BuildTriggerPolicy {
		return []buildv1.BuildTriggerPolicy{
			{
				Type: buildv1.GenericWebHookBuildTriggerType,
			},
			{
				Type:        buildv1.ImageChangeBuildTriggerType,
				ImageChange: &buildv1.ImageChangeTrigger{},
			},
			{
				Type: buildv1.ImageChangeBuildTriggerType,
				ImageChange: &buildv1.ImageChangeTrigger{
					From: &corev1.ObjectReference{
						Name: "image1:tag1",
						Kind: "ImageStreamTag",
					},
				},
			},
			{
				Type: buildv1.ImageChangeBuildTriggerType,
				ImageChange: &buildv1.ImageChangeTrigger{
					From: &corev1.ObjectReference{
						Name:      "image2:tag2",
						Namespace: "image2ns",
						Kind:      "ImageStreamTag",
					},
				},
			},
		}
	}
	pre48Trigger := func() []buildv1.BuildTriggerPolicy {
		return []buildv1.BuildTriggerPolicy{
			{
				Type: buildv1.ImageChangeBuildTriggerType,
				ImageChange: &buildv1.ImageChangeTrigger{
					From: &corev1.ObjectReference{
						Name: "image1:tag1",
						Kind: "ImageStreamTag",
					},
					LastTriggeredImageID: "ref/image1:tag1",
				},
			},
		}
	}
	tests := []struct {
		name    string
		reqFrom *corev1.ObjectReference
		// the spec LastTriggeredImageID is deprecated in 4.8 but still populated; in 4.9 it is not longer populated
		specTriggerIndex      int // indes of trigger in spec that will be updated with the imagev1id, if -1, no update expected
		statusTriggerIndex    int // index of trigger in status that will be updated with the imagev1 id, if -1, no update expected
		triggers              []buildv1.BuildTriggerPolicy
		errorExpected         bool
		lastTriggeredIDInSpec bool
	}{
		{
			name: "default trigger",
			reqFrom: &corev1.ObjectReference{
				Kind: "ImageStreamTag",
				Name: "image3:tag3",
			},
			specTriggerIndex:   1,
			statusTriggerIndex: 0,
			triggers:           defaultTriggers(),
		},
		{
			name: "trigger with from",
			reqFrom: &corev1.ObjectReference{
				Kind: "ImageStreamTag",
				Name: "image1:tag1",
			},
			specTriggerIndex:   2,
			statusTriggerIndex: 1,
			triggers:           defaultTriggers(),
		},
		{
			name: "trigger with from and namespace",
			reqFrom: &corev1.ObjectReference{
				Kind:      "ImageStreamTag",
				Name:      "image2:tag2",
				Namespace: "image2ns",
			},
			specTriggerIndex:   3,
			statusTriggerIndex: 2,
			triggers:           defaultTriggers(),
		},
		{
			name: "pre 4.7 trigger",
			reqFrom: &corev1.ObjectReference{
				Kind: "ImageStreamTag",
				Name: "image1:tag1",
			},
			specTriggerIndex:      0,
			statusTriggerIndex:    0,
			triggers:              pre48Trigger(),
			lastTriggeredIDInSpec: true,
		},
	}

	source := MockSource()
	for _, tc := range tests {
		bc := &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						SourceStrategy: &buildv1.SourceBuildStrategy{
							From: corev1.ObjectReference{
								Name: "image3:tag3",
								Kind: "ImageStreamTag",
							},
						},
					},
					Source: source,
					Revision: &buildv1.SourceRevision{
						Git: &buildv1.GitSourceRevision{
							Commit: "1234",
						},
					},
				},
				Triggers: tc.triggers,
			},
		}
		imageStreamTagFunc := func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamTag, error) {
			return &imagev1.ImageStreamTag{
				Image: imagev1.Image{
					ObjectMeta:           metav1.ObjectMeta{Name: imageRepoName + ":" + newTag},
					DockerImageReference: "ref@" + name,
				},
			}, nil
		}

		generator := mockBuildGenerator(nil, nil, nil, nil, nil, imageStreamTagFunc, nil)
		client := generator.Client.(TestingClient)
		client.GetBuildConfigFunc =
			func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return bc, nil
			}
		client.UpdateBuildConfigFunc =
			func(ctx context.Context, buildConfig *buildv1.BuildConfig, _ metav1.UpdateOptions) error {
				bc = buildConfig
				return nil
			}
		client.GetImageStreamFunc =
			func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error) {
				return &imagev1.ImageStream{
					ObjectMeta: metav1.ObjectMeta{Name: name},
					Status: imagev1.ImageStreamStatus{
						DockerImageRepository: originalImage,
						Tags: []imagev1.NamedTagEventList{
							{
								Tag: "tag1",
								Items: []imagev1.TagEvent{
									{
										DockerImageReference: "ref/" + name + ":tag1",
									},
								},
							},
							{
								Tag: "tag2",
								Items: []imagev1.TagEvent{
									{
										DockerImageReference: "ref/" + name + ":tag2",
									},
								},
							},
							{
								Tag: "tag3",
								Items: []imagev1.TagEvent{
									{
										DockerImageReference: "ref/" + name + ":tag3",
									},
								},
							},
						},
					},
				}, nil
			}
		generator.Client = client

		req := &buildv1.BuildRequest{
			TriggeredByImage: &corev1.ObjectReference{
				Kind: "DockerImage",
				Name: imageID,
			},
			From: tc.reqFrom,
		}
		_, err := generator.Instantiate(apirequest.NewDefaultContext(), req, metav1.CreateOptions{})
		if err != nil && !tc.errorExpected {
			t.Errorf("%s: unexpected error %v", tc.name, err)
			continue
		}
		if err == nil && tc.errorExpected {
			t.Errorf("%s: expected error but didn't get one", tc.name)
			continue
		}
		if tc.errorExpected {
			continue
		}
		// In 4.9 LastTriggeredImageID is no longer populated in spec.  However, BuildConfigs from clusters prior to 4.9
		// may have this field populated.
		if !tc.lastTriggeredIDInSpec {
			for i := range bc.Spec.Triggers {
				if i == tc.specTriggerIndex {
					// Verify that the trigger in spec is empty
					if bc.Spec.Triggers[i].ImageChange.LastTriggeredImageID != "" {
						t.Errorf("%s: expected trigger at index %d to NOT contain imageID %s", tc.name, i, imageID)
					}
					continue
				}
				// Ensure that other triggers are NOT updated with the latest container imagev1 ref
				if bc.Spec.Triggers[i].Type == buildv1.ImageChangeBuildTriggerType {
					from := bc.Spec.Triggers[i].ImageChange.From
					if from == nil {
						from = buildutil.GetInputReference(bc.Spec.Strategy)
					}
					if bc.Spec.Triggers[i].ImageChange.LastTriggeredImageID != "" {
						t.Errorf("%s: expected LastTriggeredImageID for trigger at %d (%+v) to be %s. Got: %s", tc.name, i, bc.Spec.Triggers[i].ImageChange.From, "ref/"+from.Name, bc.Spec.Triggers[i].ImageChange.LastTriggeredImageID)
					}
				}

			}
		}

		for i := range bc.Status.ImageChangeTriggers {
			if i == tc.statusTriggerIndex {
				// Verify that the trigger got updated
				if bc.Status.ImageChangeTriggers[i].LastTriggeredImageID != imageID {
					t.Errorf("%s: expected trigger at index %d to contain imageID %s", tc.name, i, imageID)
				}
				continue
			}
			// Ensure that other triggers are updated with the latest container imagev1 ref
			from := bc.Status.ImageChangeTriggers[i].From
			if bc.Status.ImageChangeTriggers[i].LastTriggeredImageID != ("ref/" + from.Name) {
				t.Errorf("%s: expected LastTriggeredImageID for trigger at %d (%+v) to be %s. Got: %s", tc.name, i, bc.Status.ImageChangeTriggers[i].From, "ref/"+from.Name, bc.Status.ImageChangeTriggers[i].LastTriggeredImageID)
			}

		}
	}
}

func TestInstantiateWithBuildRequestEnvs(t *testing.T) {
	buildRequestWithEnv := buildv1.BuildRequest{
		Env: []corev1.EnvVar{{Name: "FOO", Value: "BAR"}},
	}
	buildRequestWithoutEnv := buildv1.BuildRequest{}

	tests := []struct {
		bcfunc           func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error)
		req              buildv1.BuildRequest
		expectedEnvValue string
	}{
		{
			bcfunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return MockBuildConfig(MockSource(), MockSourceStrategyForEnvs(), MockOutput()), nil
			},
			req:              buildRequestWithEnv,
			expectedEnvValue: "BAR",
		},
		{
			bcfunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return MockBuildConfig(MockSource(), MockDockerStrategyForEnvs(), MockOutput()), nil
			},
			req:              buildRequestWithEnv,
			expectedEnvValue: "BAR",
		},
		{
			bcfunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return MockBuildConfig(MockSource(), MockCustomStrategyForEnvs(), MockOutput()), nil
			},
			req:              buildRequestWithEnv,
			expectedEnvValue: "BAR",
		},
		{
			bcfunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return MockBuildConfig(MockSource(), MockJenkinsStrategyForEnvs(), MockOutput()), nil
			},
			req:              buildRequestWithEnv,
			expectedEnvValue: "BAR",
		},
		{
			bcfunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return MockBuildConfig(MockSource(), MockSourceStrategyForEnvs(), MockOutput()), nil
			},
			req:              buildRequestWithoutEnv,
			expectedEnvValue: "VAR",
		},
		{
			bcfunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return MockBuildConfig(MockSource(), MockDockerStrategyForEnvs(), MockOutput()), nil
			},
			req:              buildRequestWithoutEnv,
			expectedEnvValue: "VAR",
		},
		{
			bcfunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return MockBuildConfig(MockSource(), MockCustomStrategyForEnvs(), MockOutput()), nil
			},
			req:              buildRequestWithoutEnv,
			expectedEnvValue: "VAR",
		},
		{
			bcfunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
				return MockBuildConfig(MockSource(), MockJenkinsStrategyForEnvs(), MockOutput()), nil
			},
			req:              buildRequestWithoutEnv,
			expectedEnvValue: "VAR",
		},
	}

	for _, tc := range tests {
		generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
		client := generator.Client.(TestingClient)
		client.GetBuildConfigFunc = tc.bcfunc
		generator.Client = client
		build, err := generator.Instantiate(apirequest.NewDefaultContext(), &tc.req, metav1.CreateOptions{})
		if err != nil {
			t.Errorf("unexpected error %v", err)
		} else {
			switch {
			case build.Spec.Strategy.SourceStrategy != nil:
				if len(build.Spec.Strategy.SourceStrategy.Env) == 0 {
					t.Errorf("no envs set for src bc and req %#v, expected %s", tc.req, tc.expectedEnvValue)
				} else if build.Spec.Strategy.SourceStrategy.Env[0].Value != tc.expectedEnvValue {
					t.Errorf("unexpected value %s for src bc and req %#v, expected %s", build.Spec.Strategy.SourceStrategy.Env[0].Value, tc.req, tc.expectedEnvValue)
				}
			case build.Spec.Strategy.DockerStrategy != nil:
				if len(build.Spec.Strategy.DockerStrategy.Env) == 0 {
					t.Errorf("no envs set for dock bc and req %#v, expected %s", tc.req, tc.expectedEnvValue)
				} else if build.Spec.Strategy.DockerStrategy.Env[0].Value != tc.expectedEnvValue {
					t.Errorf("unexpected value %s for dock bc and req %#v, expected %s", build.Spec.Strategy.DockerStrategy.Env[0].Value, tc.req, tc.expectedEnvValue)
				}
			case build.Spec.Strategy.CustomStrategy != nil:
				if len(build.Spec.Strategy.CustomStrategy.Env) == 0 {
					t.Errorf("no envs set for cust bc and req %#v, expected %s", tc.req, tc.expectedEnvValue)
				} else {
					// custom strategy will also have OPENSHIFT_CUSTOM_BUILD_BASE_IMAGE injected, could be in either order
					found := false
					for _, env := range build.Spec.Strategy.CustomStrategy.Env {
						if env.Value == tc.expectedEnvValue {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("unexpected values %#v for cust bc and req %#v, expected %s", build.Spec.Strategy.CustomStrategy.Env, tc.req, tc.expectedEnvValue)
					}
				}
			case build.Spec.Strategy.JenkinsPipelineStrategy != nil:
				if len(build.Spec.Strategy.JenkinsPipelineStrategy.Env) == 0 {
					t.Errorf("no envs set for jenk bc and req %#v, expected %s", tc.req, tc.expectedEnvValue)
				} else if build.Spec.Strategy.JenkinsPipelineStrategy.Env[0].Value != tc.expectedEnvValue {
					t.Errorf("unexpected value %s for jenk bc and req %#v, expected %s", build.Spec.Strategy.JenkinsPipelineStrategy.Env[0].Value, tc.req, tc.expectedEnvValue)
				}
			}
		}
	}
}

func TestInstantiateWithLastVersion(t *testing.T) {
	g := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	c := g.Client.(TestingClient)
	c.GetBuildConfigFunc = func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
		bc := MockBuildConfig(MockSource(), MockSourceStrategyForImageRepository(), MockOutput())
		bc.Status.LastVersion = 1
		return bc, nil
	}
	g.Client = c

	// Version not specified
	_, err := g.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}

	// Version specified and it matches
	lastVersion := int64(1)
	_, err = g.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{LastVersion: &lastVersion}, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}

	// Version specified, but doesn't match
	lastVersion = 0
	_, err = g.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{LastVersion: &lastVersion}, metav1.CreateOptions{})
	if err == nil {
		t.Errorf("Expected an error and did not get one")
	}
}

func TestInstantiateWithMissingImageStream(t *testing.T) {
	g := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	c := g.Client.(TestingClient)
	c.GetImageStreamFunc = func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error) {
		return nil, errors.NewNotFound(imagev1.Resource("imagestreams"), "testRepo")
	}
	g.Client = c

	_, err := g.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	se, ok := err.(*errors.StatusError)

	if !ok {
		t.Fatalf("Expected errors.StatusError, got %T", err)
	}

	if se.ErrStatus.Code != http.StatusUnprocessableEntity {
		t.Errorf("Expected status 422, got %d", se.ErrStatus.Code)
	}

	if !strings.Contains(se.ErrStatus.Message, "testns") {
		t.Errorf("Error message does not contain namespace: %q", se.ErrStatus.Message)
	}
}

func TestInstantiateWithLabelsAndAnnotations(t *testing.T) {
	g := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	c := g.Client.(TestingClient)
	c.GetBuildConfigFunc = func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
		bc := MockBuildConfig(MockSource(), MockSourceStrategyForImageRepository(), MockOutput())
		bc.Status.LastVersion = 1
		return bc, nil
	}
	g.Client = c

	req := &buildv1.BuildRequest{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"a_1": "a_value1",
				// build number is set as an annotation on the generated build, so we
				// shouldn't be able to ovewrite it here.
				buildv1.BuildNumberAnnotation: "bad_annotation",
			},
			Labels: map[string]string{
				"l_1": "l_value1",
				// testbclabel is defined as a label on the mockBuildConfig so we shouldn't
				// be able to overwrite it here.
				"testbclabel": "bad_label",
			},
		},
	}

	build, err := g.Instantiate(apirequest.NewDefaultContext(), req, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	if build.Annotations["a_1"] != "a_value1" || build.Annotations[buildv1.BuildNumberAnnotation] == "bad_annotation" {
		t.Errorf("Build annotations were merged incorrectly: %v", build.Annotations)
	}
	if build.Labels["l_1"] != "l_value1" || build.Labels[buildv1.BuildLabel] == "bad_label" {
		t.Errorf("Build labels were merged incorrectly: %v", build.Labels)
	}
}

func TestFindImageTrigger(t *testing.T) {
	defaultTrigger := &buildv1.ImageChangeTrigger{}
	defaultTriggerResp := buildv1.ImageChangeTriggerStatus{
		From: buildv1.ImageStreamTagReference{
			Name: "image3:tag3",
		},
	}
	image1Trigger := &buildv1.ImageChangeTrigger{
		From: &corev1.ObjectReference{
			Name: "image1:tag1",
		},
	}
	image1TriggerResp := buildv1.ImageChangeTriggerStatus{
		From: buildv1.ImageStreamTagReference{
			Name: "image1:tag1",
		},
	}
	image2Trigger := &buildv1.ImageChangeTrigger{
		From: &corev1.ObjectReference{
			Name:      "image2:tag2",
			Namespace: "image2ns",
		},
	}
	image2TriggerResp := buildv1.ImageChangeTriggerStatus{
		From: buildv1.ImageStreamTagReference{
			Name:      "image2:tag2",
			Namespace: "image2ns",
		},
	}
	image4Trigger := &buildv1.ImageChangeTrigger{
		From: &corev1.ObjectReference{
			Name: "image4:tag4",
		},
	}
	image4TriggerResp := buildv1.ImageChangeTriggerStatus{
		From: buildv1.ImageStreamTagReference{
			Name: "image4:tag4",
		},
	}
	image5Trigger := &buildv1.ImageChangeTrigger{
		From: &corev1.ObjectReference{
			Name:      "image5:tag5",
			Namespace: "bcnamespace",
		},
	}
	image5TriggerResp := buildv1.ImageChangeTriggerStatus{
		From: buildv1.ImageStreamTagReference{
			Name:      "image5:tag5",
			Namespace: "bcnamespace",
		},
	}
	bc := &buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testbc",
			Namespace: "bcnamespace",
		},
		Spec: buildv1.BuildConfigSpec{
			CommonSpec: buildv1.CommonSpec{
				Strategy: buildv1.BuildStrategy{
					SourceStrategy: &buildv1.SourceBuildStrategy{
						From: corev1.ObjectReference{
							Name: "image3:tag3",
							Kind: "ImageStreamTag",
						},
					},
				},
			},
			Triggers: []buildv1.BuildTriggerPolicy{
				{
					Type: buildv1.GenericWebHookBuildTriggerType,
				},
				{
					Type:        buildv1.ImageChangeBuildTriggerType,
					ImageChange: defaultTrigger,
				},
				{
					Type:        buildv1.ImageChangeBuildTriggerType,
					ImageChange: image1Trigger,
				},
				{
					Type:        buildv1.ImageChangeBuildTriggerType,
					ImageChange: image2Trigger,
				},
				{
					Type:        buildv1.ImageChangeBuildTriggerType,
					ImageChange: image4Trigger,
				},
				{
					Type:        buildv1.ImageChangeBuildTriggerType,
					ImageChange: image5Trigger,
				},
			},
		},
		Status: buildv1.BuildConfigStatus{
			ImageChangeTriggers: []buildv1.ImageChangeTriggerStatus{
				defaultTriggerResp,
				image1TriggerResp,
				image2TriggerResp,
				image4TriggerResp,
				image5TriggerResp,
			},
		},
	}

	tests := []struct {
		name      string
		input     *corev1.ObjectReference
		expectReq *buildv1.ImageChangeTrigger
		expectRsp *buildv1.ImageChangeTriggerStatus
	}{
		{
			name:      "nil reference",
			input:     nil,
			expectReq: nil,
			expectRsp: nil,
		},
		{
			name: "match name",
			input: &corev1.ObjectReference{
				Name: "image1:tag1",
			},
			expectReq: image1Trigger,
			expectRsp: &image1TriggerResp,
		},
		{
			name: "mismatched namespace",
			input: &corev1.ObjectReference{
				Name:      "image1:tag1",
				Namespace: "otherns",
			},
			expectReq: nil,
			expectRsp: nil,
		},
		{
			name: "match name and namespace",
			input: &corev1.ObjectReference{
				Name:      "image2:tag2",
				Namespace: "image2ns",
			},
			expectReq: image2Trigger,
			expectRsp: &image2TriggerResp,
		},
		{
			name: "match default trigger",
			input: &corev1.ObjectReference{
				Name: "image3:tag3",
			},
			expectReq: defaultTrigger,
			expectRsp: &defaultTriggerResp,
		},
		{
			name: "input includes bc namespace",
			input: &corev1.ObjectReference{
				Name:      "image4:tag4",
				Namespace: "bcnamespace",
			},
			expectReq: image4Trigger,
			expectRsp: &image4TriggerResp,
		},
		{
			name: "implied namespace in trigger input",
			input: &corev1.ObjectReference{
				Name: "image5:tag5",
			},
			expectReq: image5Trigger,
			expectRsp: &image5TriggerResp,
		},
	}

	for _, tc := range tests {
		result, response := findImageChangeTrigger(bc, tc.input)
		if result != tc.expectReq {
			t.Errorf("%s: unexpected trigger for %#v: %#v", tc.name, tc.input, result)
			continue
		}
		if response == nil && tc.expectRsp == nil {
			continue
		}
		if response != nil && tc.expectRsp == nil {
			t.Errorf("%s: unexpected trigger for %#v: %#v", tc.name, tc.input, result)
			continue
		}
		if response == nil && tc.expectRsp != nil {
			t.Errorf("%s: unexpected trigger for %#v: %#v", tc.name, tc.input, result)
			continue
		}
		if len(response.From.Name) == 0 && len(tc.expectRsp.From.Name) == 0 {
			continue
		}
		if len(response.From.Name) > 0 && len(tc.expectRsp.From.Name) == 0 {
			t.Errorf("%s: unexpected trigger for %#v: %#v", tc.name, tc.input, result)
			continue
		}
		if len(response.From.Name) == 0 && len(tc.expectRsp.From.Name) > 0 {
			t.Errorf("%s: unexpected trigger for %#v: %#v", tc.name, tc.input, result)
			continue
		}
		if response.From.Namespace != tc.expectRsp.From.Namespace || response.From.Name != tc.expectRsp.From.Name {
			t.Errorf("%s: unexpected trigger for %#v: %#v", tc.name, tc.input, result)
		}
	}

}

func TestClone(t *testing.T) {
	generator := BuildGenerator{Client: TestingClient{
		CreateBuildFunc: func(ctx context.Context, build *buildv1.Build, _ metav1.CreateOptions) error {
			return nil
		},
		GetBuildFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.Build, error) {
			return &buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-build-1",
					Namespace: metav1.NamespaceDefault,
				},
			}, nil
		},
	}}

	_, err := generator.Clone(apirequest.NewDefaultContext(), &buildv1.BuildRequest{})
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
}

func TestCloneError(t *testing.T) {
	generator := BuildGenerator{Client: TestingClient{
		GetBuildFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.Build, error) {
			return nil, fmt.Errorf("get-error")
		},
	}}

	_, err := generator.Clone(apirequest.NewContext(), &buildv1.BuildRequest{})
	if err == nil || !strings.Contains(err.Error(), "get-error") {
		t.Errorf("Expected get-error, got different %v", err)
	}
}

func TestCreateBuild(t *testing.T) {
	build := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-build",
			Namespace: metav1.NamespaceDefault,
		},
	}
	generator := BuildGenerator{Client: TestingClient{
		CreateBuildFunc: func(ctx context.Context, build *buildv1.Build, _ metav1.CreateOptions) error {
			return nil
		},
		GetBuildFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.Build, error) {
			return build, nil
		},
	}}

	build, err := generator.createBuild(apirequest.NewDefaultContext(), build, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if build.CreationTimestamp.IsZero() || len(build.UID) == 0 {
		t.Error("Expected meta fields being filled in!")
	}
}

func TestCreateBuildNamespaceError(t *testing.T) {
	build := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-build",
		},
	}
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)

	_, err := generator.createBuild(apirequest.NewContext(), build, metav1.CreateOptions{})
	if err == nil || !strings.Contains(err.Error(), "namespace") {
		t.Errorf("Expected namespace error, got different %v", err)
	}
}

func TestCreateBuildCreateError(t *testing.T) {
	build := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-build",
			Namespace: metav1.NamespaceDefault,
		},
	}
	generator := BuildGenerator{Client: TestingClient{
		CreateBuildFunc: func(ctx context.Context, build *buildv1.Build, _ metav1.CreateOptions) error {
			return fmt.Errorf("create-error")
		},
	}}

	_, err := generator.createBuild(apirequest.NewDefaultContext(), build, metav1.CreateOptions{})
	if err == nil || !strings.Contains(err.Error(), "create-error") {
		t.Errorf("Expected create-error, got different %v", err)
	}
}

func TestGenerateBuildFromConfig(t *testing.T) {
	source := MockSource()
	strategy := mockDockerStrategyForDockerImage(originalImage, metav1.GetOptions{})
	output := MockOutput()
	resources := mockResources()
	expectedLabel := "test-build-config-4.3.0.ipv6-2019-11-27-0001-build"
	mountTrustedCA := true
	bc := &buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			UID: "test-uid",
			// Specify a name here that we cannot use as-is for a k8s ObjectMeta label
			Name:      "test-build-config-4.3.0.ipv6-2019-11-27-0001-build",
			Namespace: metav1.NamespaceDefault,
			Labels:    map[string]string{"testlabel": "testvalue"},
		},
		Spec: buildv1.BuildConfigSpec{
			CommonSpec: buildv1.CommonSpec{
				Source: source,
				Revision: &buildv1.SourceRevision{
					Git: &buildv1.GitSourceRevision{
						Commit: "1234",
					},
				},
				Strategy:       strategy,
				Output:         output,
				Resources:      resources,
				MountTrustedCA: &mountTrustedCA,
			},
		},
		Status: buildv1.BuildConfigStatus{
			LastVersion: 12,
		},
	}
	revision := &buildv1.SourceRevision{
		Git: &buildv1.GitSourceRevision{
			Commit: "abcd",
		},
	}
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)

	build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, revision, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if !reflect.DeepEqual(source, build.Spec.Source) {
		t.Errorf("Build source does not match BuildConfig source")
	}
	// FIXME: This is disabled because the strategies does not match since we plug the
	//        pullSecret into the build strategy.
	/*
		if !reflect.DeepEqual(strategy, build.Spec.Strategy) {
			t.Errorf("Build strategy does not match BuildConfig strategy %+v != %+v", strategy.DockerStrategy, build.Spec.Strategy.DockerStrategy)
		}
	*/
	if !reflect.DeepEqual(output, build.Spec.Output) {
		t.Errorf("Build output does not match BuildConfig output")
	}
	if !reflect.DeepEqual(revision, build.Spec.Revision) {
		t.Errorf("Build revision does not match passed in revision")
	}
	if !reflect.DeepEqual(resources, build.Spec.Resources) {
		t.Errorf("Build resources does not match passed in resources")
	}
	if build.Labels["testlabel"] != bc.Labels["testlabel"] {
		t.Errorf("Build does not contain labels from BuildConfig")
	}
	if build.Annotations[buildv1.BuildConfigAnnotation] != bc.Name {
		t.Errorf("Build does not contain annotation from BuildConfig")
	}
	if build.Labels[buildv1.BuildConfigLabel] != expectedLabel {
		t.Errorf("Build does not contain labels from BuildConfig")
	}
	if build.Labels[buildv1.BuildConfigLabelDeprecated] != expectedLabel {
		t.Errorf("Build does not contain labels from BuildConfig")
	}
	if build.Status.Config.Name != bc.Name || build.Status.Config.Namespace != bc.Namespace || build.Status.Config.Kind != "BuildConfig" {
		t.Errorf("Build does not contain correct BuildConfig reference: %v", build.Status.Config)
	}
	if build.Annotations[buildv1.BuildNumberAnnotation] != "13" {
		t.Errorf("Build number annotation value %s does not match expected value 13", build.Annotations[buildv1.BuildNumberAnnotation])
	}
	if len(build.OwnerReferences) == 0 || build.OwnerReferences[0].Kind != "BuildConfig" || build.OwnerReferences[0].Name != bc.Name {
		t.Errorf("generated build does not have OwnerReference to parent BuildConfig")
	}
	if build.OwnerReferences[0].Controller == nil || !*build.OwnerReferences[0].Controller {
		t.Errorf("generated build does not have OwnerReference to parent BuildConfig marked as a controller relationship")
	}
	if !reflect.DeepEqual(build.Spec.MountTrustedCA, bc.Spec.MountTrustedCA) {
		t.Error("generated build does not have MountTrustedCA copied")
	}
	// Test long name
	bc.Name = strings.Repeat("a", 100)
	build, err = generator.generateBuildFromConfig(apirequest.NewContext(), bc, revision, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	build.Namespace = "test-namespace"

	// TODO: We have to convert this to internal as the validation/apiserver is still using internal build...
	internalBuild := &buildapi.Build{}
	if err := scheme.Convert(build, internalBuild, nil); err != nil {
		t.Fatalf("unable to convert to internal build: %v", err)
	}
	validateErrors := validation.ValidateBuild(internalBuild)
	if len(validateErrors) > 0 {
		t.Fatalf("Unexpected validation errors %v", validateErrors)
	}

}

func TestGenerateBuildWithImageTagForSourceStrategyImageRepository(t *testing.T) {
	source := MockSource()
	strategy := MockSourceStrategyForImageRepository()
	output := MockOutput()
	bc := &buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-build-config",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: buildv1.BuildConfigSpec{
			CommonSpec: buildv1.CommonSpec{
				Source: source,
				Revision: &buildv1.SourceRevision{
					Git: &buildv1.GitSourceRevision{
						Commit: "1234",
					},
				},
				Strategy: strategy,
				Output:   output,
			},
		},
	}
	fakeSecrets := []runtime.Object{}
	for _, s := range MockBuilderSecrets() {
		fakeSecrets = append(fakeSecrets, s)
	}
	is := MockImageStream("", originalImage, map[string]string{tagName: newTag})
	generator := BuildGenerator{
		Secrets:         fake.NewSimpleClientset(fakeSecrets...).CoreV1(),
		ServiceAccounts: MockBuilderServiceAccount(MockBuilderSecrets()),
		Client: TestingClient{
			GetImageStreamFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error) {
				return &imagev1.ImageStream{
					ObjectMeta: metav1.ObjectMeta{Name: imageRepoName},
					Status: imagev1.ImageStreamStatus{
						DockerImageRepository: originalImage,
						Tags:                  is.Status.Tags,
					},
				}, nil
			},
			GetImageStreamTagFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamTag, error) {
				return &imagev1.ImageStreamTag{
					Image: imagev1.Image{
						ObjectMeta:           metav1.ObjectMeta{Name: imageRepoName + ":" + newTag},
						DockerImageReference: originalImage + ":" + newTag,
					},
				}, nil
			},
			GetImageStreamImageFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamImage, error) {
				return &imagev1.ImageStreamImage{
					Image: imagev1.Image{
						ObjectMeta:           metav1.ObjectMeta{Name: imageRepoName + ":@id"},
						DockerImageReference: originalImage + ":" + newTag,
					},
				}, nil
			},

			UpdateBuildConfigFunc: func(ctx context.Context, buildConfig *buildv1.BuildConfig, _ metav1.UpdateOptions) error {
				return nil
			},
		}}

	build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if build.Spec.Strategy.SourceStrategy.From.Name != newImage {
		t.Errorf("source-to-image base image value %s does not match expected value %s", build.Spec.Strategy.SourceStrategy.From.Name, newImage)
	}
}

func TestGenerateBuildWithImageTagForDockerStrategyImageRepository(t *testing.T) {
	source := MockSource()
	strategy := mockDockerStrategyForImageRepository()
	output := MockOutput()
	bc := &buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-build-config",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: buildv1.BuildConfigSpec{
			CommonSpec: buildv1.CommonSpec{
				Source: source,
				Revision: &buildv1.SourceRevision{
					Git: &buildv1.GitSourceRevision{
						Commit: "1234",
					},
				},
				Strategy: strategy,
				Output:   output,
			},
		},
	}
	fakeSecrets := []runtime.Object{}
	for _, s := range MockBuilderSecrets() {
		fakeSecrets = append(fakeSecrets, s)
	}
	is := MockImageStream("", originalImage, map[string]string{tagName: newTag})
	generator := BuildGenerator{
		Secrets:         fake.NewSimpleClientset(fakeSecrets...).CoreV1(),
		ServiceAccounts: MockBuilderServiceAccount(MockBuilderSecrets()),
		Client: TestingClient{
			GetImageStreamFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error) {
				return &imagev1.ImageStream{
					ObjectMeta: metav1.ObjectMeta{Name: imageRepoName},
					Status: imagev1.ImageStreamStatus{
						DockerImageRepository: originalImage,
						Tags:                  is.Status.Tags,
					},
				}, nil
			},
			GetImageStreamTagFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamTag, error) {
				return &imagev1.ImageStreamTag{
					Image: imagev1.Image{
						ObjectMeta:           metav1.ObjectMeta{Name: imageRepoName + ":" + newTag},
						DockerImageReference: originalImage + ":" + newTag,
					},
				}, nil
			},
			GetImageStreamImageFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamImage, error) {
				return &imagev1.ImageStreamImage{
					Image: imagev1.Image{
						ObjectMeta:           metav1.ObjectMeta{Name: imageRepoName + ":@id"},
						DockerImageReference: originalImage + ":" + newTag,
					},
				}, nil
			},
			UpdateBuildConfigFunc: func(ctx context.Context, buildConfig *buildv1.BuildConfig, _ metav1.UpdateOptions) error {
				return nil
			},
		}}

	build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if build.Spec.Strategy.DockerStrategy.From.Name != newImage {
		t.Errorf("Docker base image value %s does not match expected value %s", build.Spec.Strategy.DockerStrategy.From.Name, newImage)
	}
}

func TestGenerateBuildWithImageTagForCustomStrategyImageRepository(t *testing.T) {
	source := MockSource()
	strategy := mockCustomStrategyForImageRepository()
	output := MockOutput()
	bc := &buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-build-config",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: buildv1.BuildConfigSpec{
			CommonSpec: buildv1.CommonSpec{
				Source: source,
				Revision: &buildv1.SourceRevision{
					Git: &buildv1.GitSourceRevision{
						Commit: "1234",
					},
				},
				Strategy: strategy,
				Output:   output,
			},
		},
	}
	fakeSecrets := []runtime.Object{}
	for _, s := range MockBuilderSecrets() {
		fakeSecrets = append(fakeSecrets, s)
	}
	is := MockImageStream("", originalImage, map[string]string{tagName: newTag})
	generator := BuildGenerator{
		Secrets:         fake.NewSimpleClientset(fakeSecrets...).CoreV1(),
		ServiceAccounts: MockBuilderServiceAccount(MockBuilderSecrets()),
		Client: TestingClient{
			GetImageStreamFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error) {
				return &imagev1.ImageStream{
					ObjectMeta: metav1.ObjectMeta{Name: imageRepoName},
					Status: imagev1.ImageStreamStatus{
						DockerImageRepository: originalImage,
						Tags:                  is.Status.Tags,
					},
				}, nil
			},
			GetImageStreamTagFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamTag, error) {
				return &imagev1.ImageStreamTag{
					Image: imagev1.Image{
						ObjectMeta:           metav1.ObjectMeta{Name: imageRepoName + ":" + newTag},
						DockerImageReference: originalImage + ":" + newTag,
					},
				}, nil
			},
			GetImageStreamImageFunc: func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamImage, error) {
				return &imagev1.ImageStreamImage{
					Image: imagev1.Image{
						ObjectMeta:           metav1.ObjectMeta{Name: imageRepoName + ":@id"},
						DockerImageReference: originalImage + ":" + newTag,
					},
				}, nil
			},
			UpdateBuildConfigFunc: func(ctx context.Context, buildConfig *buildv1.BuildConfig, _ metav1.UpdateOptions) error {
				return nil
			},
		}}

	build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if build.Spec.Strategy.CustomStrategy.From.Name != newImage {
		t.Errorf("Custom base image value %s does not match expected value %s", build.Spec.Strategy.CustomStrategy.From.Name, newImage)
	}
}

func TestGenerateBuildFromBuild(t *testing.T) {
	source := MockSource()
	strategy := mockDockerStrategyForImageRepository()
	output := MockOutput()
	build := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-build",
			Annotations: map[string]string{
				buildv1.BuildJenkinsStatusJSONAnnotation:      "foo",
				buildv1.BuildJenkinsLogURLAnnotation:          "bar",
				buildv1.BuildJenkinsConsoleLogURLAnnotation:   "bar",
				buildv1.BuildJenkinsBlueOceanLogURLAnnotation: "bar",
				buildv1.BuildJenkinsBuildURIAnnotation:        "baz",
				buildv1.BuildPodNameAnnotation:                "ruby-sample-build-1-build",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       "test-owner",
					Kind:       "BuildConfig",
					APIVersion: "v1",
					UID:        "foo",
				},
			},
		},
		Spec: buildv1.BuildSpec{
			CommonSpec: buildv1.CommonSpec{
				Source: source,
				Revision: &buildv1.SourceRevision{
					Git: &buildv1.GitSourceRevision{
						Commit: "1234",
					},
				},
				Strategy: strategy,
				Output:   output,
			},
		},
	}

	newBuild := generateBuildFromBuild(build, nil)
	if !reflect.DeepEqual(build.Spec, newBuild.Spec) {
		t.Errorf("Build parameters does not match the original Build parameters")
	}
	if !reflect.DeepEqual(build.ObjectMeta.Labels, newBuild.ObjectMeta.Labels) {
		t.Errorf("Build labels does not match the original Build labels")
	}
	if _, ok := newBuild.ObjectMeta.Annotations[buildv1.BuildJenkinsStatusJSONAnnotation]; ok {
		t.Errorf("%s annotation exists, expected it not to", buildv1.BuildJenkinsStatusJSONAnnotation)
	}
	if _, ok := newBuild.ObjectMeta.Annotations[buildv1.BuildJenkinsLogURLAnnotation]; ok {
		t.Errorf("%s annotation exists, expected it not to", buildv1.BuildJenkinsLogURLAnnotation)
	}
	if _, ok := newBuild.ObjectMeta.Annotations[buildv1.BuildJenkinsConsoleLogURLAnnotation]; ok {
		t.Errorf("%s annotation exists, expected it not to", buildv1.BuildJenkinsConsoleLogURLAnnotation)
	}
	if _, ok := newBuild.ObjectMeta.Annotations[buildv1.BuildJenkinsBlueOceanLogURLAnnotation]; ok {
		t.Errorf("%s annotation exists, expected it not to", buildv1.BuildJenkinsBlueOceanLogURLAnnotation)
	}
	if _, ok := newBuild.ObjectMeta.Annotations[buildv1.BuildJenkinsBuildURIAnnotation]; ok {
		t.Errorf("%s annotation exists, expected it not to", buildv1.BuildJenkinsBuildURIAnnotation)
	}
	if _, ok := newBuild.ObjectMeta.Annotations[buildv1.BuildPodNameAnnotation]; ok {
		t.Errorf("%s annotation exists, expected it not to", buildv1.BuildPodNameAnnotation)
	}
	if !reflect.DeepEqual(build.ObjectMeta.OwnerReferences, newBuild.ObjectMeta.OwnerReferences) {
		t.Errorf("Build OwnerReferences does not match the original Build OwnerReferences")
	}

}

func TestGenerateBuildFromBuildWithBuildConfig(t *testing.T) {
	source := MockSource()
	strategy := mockDockerStrategyForImageRepository()
	output := MockOutput()
	annotatedBuild := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name: "annotatedBuild",
			Annotations: map[string]string{
				buildv1.BuildCloneAnnotation: "sourceOfBuild",
			},
		},
		Spec: buildv1.BuildSpec{
			CommonSpec: buildv1.CommonSpec{
				Source: source,
				Revision: &buildv1.SourceRevision{
					Git: &buildv1.GitSourceRevision{
						Commit: "1234",
					},
				},
				Strategy: strategy,
				Output:   output,
			},
		},
	}
	nonAnnotatedBuild := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nonAnnotatedBuild",
		},
		Spec: buildv1.BuildSpec{
			CommonSpec: buildv1.CommonSpec{
				Source: source,
				Revision: &buildv1.SourceRevision{
					Git: &buildv1.GitSourceRevision{
						Commit: "1234",
					},
				},
				Strategy: strategy,
				Output:   output,
			},
		},
	}

	buildConfig := &buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "buildConfigName",
		},
		Status: buildv1.BuildConfigStatus{
			LastVersion: 5,
		},
	}

	newBuild := generateBuildFromBuild(annotatedBuild, buildConfig)
	if !reflect.DeepEqual(annotatedBuild.Spec, newBuild.Spec) {
		t.Errorf("Build parameters does not match the original Build parameters")
	}
	if !reflect.DeepEqual(annotatedBuild.ObjectMeta.Labels, newBuild.ObjectMeta.Labels) {
		t.Errorf("Build labels does not match the original Build labels")
	}
	if newBuild.Annotations[buildv1.BuildNumberAnnotation] != "6" {
		t.Errorf("Build number annotation is %s expected %s", newBuild.Annotations[buildv1.BuildNumberAnnotation], "6")
	}
	if newBuild.Annotations[buildv1.BuildCloneAnnotation] != "annotatedBuild" {
		t.Errorf("Build number annotation is %s expected %s", newBuild.Annotations[buildv1.BuildCloneAnnotation], "annotatedBuild")
	}

	newBuild = generateBuildFromBuild(nonAnnotatedBuild, buildConfig)
	if !reflect.DeepEqual(nonAnnotatedBuild.Spec, newBuild.Spec) {
		t.Errorf("Build parameters does not match the original Build parameters")
	}
	if !reflect.DeepEqual(nonAnnotatedBuild.ObjectMeta.Labels, newBuild.ObjectMeta.Labels) {
		t.Errorf("Build labels does not match the original Build labels")
	}
	// was incremented by previous test, so expectReq 7 now.
	if newBuild.Annotations[buildv1.BuildNumberAnnotation] != "7" {
		t.Errorf("Build number annotation is %s expected %s", newBuild.Annotations[buildv1.BuildNumberAnnotation], "7")
	}
	if newBuild.Annotations[buildv1.BuildCloneAnnotation] != "nonAnnotatedBuild" {
		t.Errorf("Build number annotation is %s expected %s", newBuild.Annotations[buildv1.BuildCloneAnnotation], "nonAnnotatedBuild")
	}

}

func TestSubstituteImageCustomAllMatch(t *testing.T) {
	source := MockSource()
	strategy := mockCustomStrategyForDockerImage(originalImage, metav1.GetOptions{})
	output := MockOutput()
	bc := MockBuildConfig(source, strategy, output)
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// Full custom build with a Image and a well defined environment variable imagev1 value,
	// both should be replaced.  Additional environment variables should not be touched.
	build.Spec.Strategy.CustomStrategy.Env = make([]corev1.EnvVar, 2)
	build.Spec.Strategy.CustomStrategy.Env[0] = corev1.EnvVar{Name: "someImage", Value: originalImage}
	build.Spec.Strategy.CustomStrategy.Env[1] = corev1.EnvVar{Name: buildv1.CustomBuildStrategyBaseImageKey, Value: originalImage}
	updateCustomImageEnv(build.Spec.Strategy.CustomStrategy, newImage)
	if build.Spec.Strategy.CustomStrategy.Env[0].Value != originalImage {
		t.Errorf("Random env variable %s was improperly substituted in custom strategy", build.Spec.Strategy.CustomStrategy.Env[0].Name)
	}
	if build.Spec.Strategy.CustomStrategy.Env[1].Value != newImage {
		t.Errorf("Image env variable was not properly substituted in custom strategy")
	}
	if c := len(build.Spec.Strategy.CustomStrategy.Env); c != 2 {
		t.Errorf("Expected %d, found %d environment variables", 2, c)
	}
	if bc.Spec.Strategy.CustomStrategy.From.Name != originalImage {
		t.Errorf("Custom BuildConfig Image was updated when Build was modified %s!=%s", bc.Spec.Strategy.CustomStrategy.From.Name, originalImage)
	}
	if len(bc.Spec.Strategy.CustomStrategy.Env) != 0 {
		t.Errorf("Custom BuildConfig Env was updated when Build was modified")
	}
}

func TestSubstituteImageCustomAllMismatch(t *testing.T) {
	source := MockSource()
	strategy := mockCustomStrategyForDockerImage(originalImage, metav1.GetOptions{})
	output := MockOutput()
	bc := MockBuildConfig(source, strategy, output)
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// Full custom build with base imagev1 that is not matched
	// Base imagev1 name should be unchanged
	updateCustomImageEnv(build.Spec.Strategy.CustomStrategy, "dummy")
	if build.Spec.Strategy.CustomStrategy.From.Name != originalImage {
		t.Errorf("Base image name was improperly substituted in custom strategy %s %s", build.Spec.Strategy.CustomStrategy.From.Name, originalImage)
	}
}

func TestSubstituteImageCustomBaseMatchEnvMismatch(t *testing.T) {
	source := MockSource()
	strategy := mockCustomStrategyForImageRepository()
	output := MockOutput()
	bc := MockBuildConfig(source, strategy, output)
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// Full custom build with a Image and a well defined environment variable image value that does not match the new image
	// Environment variables should not be updated.
	build.Spec.Strategy.CustomStrategy.Env = make([]corev1.EnvVar, 2)
	build.Spec.Strategy.CustomStrategy.Env[0] = corev1.EnvVar{Name: "someEnvVar", Value: originalImage}
	build.Spec.Strategy.CustomStrategy.Env[1] = corev1.EnvVar{Name: buildv1.CustomBuildStrategyBaseImageKey, Value: "dummy"}
	updateCustomImageEnv(build.Spec.Strategy.CustomStrategy, newImage)
	if build.Spec.Strategy.CustomStrategy.Env[0].Value != originalImage {
		t.Errorf("Random env variable %s was improperly substituted in custom strategy", build.Spec.Strategy.CustomStrategy.Env[0].Name)
	}
	if build.Spec.Strategy.CustomStrategy.Env[1].Value != newImage {
		t.Errorf("Image env variable was not substituted in custom strategy")
	}
	if c := len(build.Spec.Strategy.CustomStrategy.Env); c != 2 {
		t.Errorf("Expected %d, found %d environment variables", 2, c)
	}
}

func TestSubstituteImageCustomBaseMatchEnvMissing(t *testing.T) {
	source := MockSource()
	strategy := mockCustomStrategyForImageRepository()
	output := MockOutput()
	bc := MockBuildConfig(source, strategy, output)
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// Custom build with a base Image but no image environment variable.
	// base image should be replaced, new image environment variable should be added,
	// existing environment variable should be untouched
	build.Spec.Strategy.CustomStrategy.Env = make([]corev1.EnvVar, 1)
	build.Spec.Strategy.CustomStrategy.Env[0] = corev1.EnvVar{Name: "someImage", Value: originalImage}
	updateCustomImageEnv(build.Spec.Strategy.CustomStrategy, newImage)
	if build.Spec.Strategy.CustomStrategy.Env[0].Value != originalImage {
		t.Errorf("Random env variable was improperly substituted in custom strategy")
	}
	if build.Spec.Strategy.CustomStrategy.Env[1].Name != buildv1.CustomBuildStrategyBaseImageKey || build.Spec.Strategy.CustomStrategy.Env[1].
		Value != newImage {
		t.Errorf("Image env variable was not added in custom strategy %s %s |", build.Spec.Strategy.CustomStrategy.Env[1].Name, build.Spec.Strategy.CustomStrategy.Env[1].Value)
	}
	if c := len(build.Spec.Strategy.CustomStrategy.Env); c != 2 {
		t.Errorf("Expected %d, found %d environment variables", 2, c)
	}
}

func TestSubstituteImageCustomBaseMatchEnvNil(t *testing.T) {
	source := MockSource()
	strategy := mockCustomStrategyForImageRepository()
	output := MockOutput()
	bc := MockBuildConfig(source, strategy, output)
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// Custom build with a base Image but no environment variables
	// base image should be replaced, new image environment variable should be added
	updateCustomImageEnv(build.Spec.Strategy.CustomStrategy, newImage)
	if build.Spec.Strategy.CustomStrategy.Env[0].Name != buildv1.CustomBuildStrategyBaseImageKey || build.Spec.Strategy.CustomStrategy.Env[0].
		Value != newImage {
		t.Errorf("New image name variable was not added to environment list in custom strategy")
	}
	if c := len(build.Spec.Strategy.CustomStrategy.Env); c != 1 {
		t.Errorf("Expected %d, found %d environment variables", 1, c)
	}
}

func TestGetNextBuildName(t *testing.T) {
	bc := MockBuildConfig(MockSource(), MockSourceStrategyForImageRepository(), MockOutput())
	if expected, actual := bc.Name+"-1", getNextBuildName(bc); expected != actual {
		t.Errorf("Wrong buildName, expected %s, got %s", expected, actual)
	}
	if expected, actual := int64(1), bc.Status.LastVersion; expected != actual {
		t.Errorf("Wrong version, expected %d, got %d", expected, actual)
	}
}

func TestGetNextBuildNameFromBuild(t *testing.T) {
	testCases := []struct {
		value    string
		expected string
	}{
		// 0
		{"mybuild-1", `^mybuild-1-\d+$`},
		// 1
		{"mybuild-1-1426794070", `^mybuild-1-\d+$`},
		// 2
		{"mybuild-1-1426794070-1-1426794070", `^mybuild-1-1426794070-1-\d+$`},
		// 3
		{"my-build-1", `^my-build-1-\d+$`},
		// 4
		{"mybuild-10-1426794070", `^mybuild-10-\d+$`},
	}

	for i, tc := range testCases {
		buildName := getNextBuildNameFromBuild(&buildv1.Build{ObjectMeta: metav1.ObjectMeta{Name: tc.value}}, nil)
		if matched, err := regexp.MatchString(tc.expected, buildName); !matched || err != nil {
			t.Errorf("(%d) Unexpected build name, got %s expected %s", i, buildName, tc.expected)
		}
	}
}

func TestGetNextBuildNameFromBuildWithBuildConfig(t *testing.T) {
	testCases := []struct {
		value       string
		buildConfig *buildv1.BuildConfig
		expected    string
	}{
		// 0
		{
			"mybuild-1",
			&buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "buildConfigName",
				},
				Status: buildv1.BuildConfigStatus{
					LastVersion: 5,
				},
			},
			`^buildConfigName-6$`,
		},
		// 1
		{
			"mybuild-1-1426794070",
			&buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "buildConfigName",
				},
				Status: buildv1.BuildConfigStatus{
					LastVersion: 5,
				},
			},
			`^buildConfigName-6$`,
		},
	}

	for i, tc := range testCases {
		buildName := getNextBuildNameFromBuild(&buildv1.Build{ObjectMeta: metav1.ObjectMeta{Name: tc.value}}, tc.buildConfig)
		if matched, err := regexp.MatchString(tc.expected, buildName); !matched || err != nil {
			t.Errorf("(%d) Unexpected build name, got %s expected %s", i, buildName, tc.expected)
		}
	}
}

func TestResolveImageStreamRef(t *testing.T) {
	type resolveTest struct {
		streamRef         corev1.ObjectReference
		tag               string
		expectedSuccess   bool
		expectedDockerRef string
	}
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)

	tests := []resolveTest{
		{
			streamRef: corev1.ObjectReference{
				Name: imageRepoName,
			},
			tag:               tagName,
			expectedSuccess:   false,
			expectedDockerRef: dockerReference,
		},
		{
			streamRef: corev1.ObjectReference{
				Kind: "ImageStreamTag",
				Name: imageRepoName + ":" + tagName,
			},
			expectedSuccess:   true,
			expectedDockerRef: dockerReference,
		},
		{
			streamRef: corev1.ObjectReference{
				Kind: "ImageStreamImage",
				Name: imageRepoName + "@myid",
			},
			expectedSuccess: true,
			// until we default to the "real" pull by id logic,
			// the @id is applied as a :tag when resolving the repository.
			expectedDockerRef: latestDockerReference,
		},
	}
	for i, test := range tests {
		ref, err := generator.resolveImageStreamReference(apirequest.NewDefaultContext(), test.streamRef, "")
		if err != nil {
			if test.expectedSuccess {
				t.Errorf("Scenario %d: Unexpected error %v", i, err)
			}
			continue
		} else if !test.expectedSuccess {
			t.Errorf("Scenario %d: did not get expected error", i)
		}
		if ref != test.expectedDockerRef {
			t.Errorf("Scenario %d: Resolved reference %q did not match expected value %q", i, ref, test.expectedDockerRef)
		}
	}
}

func mockResources() corev1.ResourceRequirements {
	res := corev1.ResourceRequirements{}
	res.Limits = corev1.ResourceList{}
	res.Limits[corev1.ResourceCPU] = resource.MustParse("100m")
	res.Limits[corev1.ResourceMemory] = resource.MustParse("100Mi")
	return res
}

func mockDockerStrategyForDockerImage(name string, options metav1.GetOptions) buildv1.BuildStrategy {
	return buildv1.BuildStrategy{
		DockerStrategy: &buildv1.DockerBuildStrategy{
			NoCache: true,
			From: &corev1.ObjectReference{
				Kind: "DockerImage",
				Name: name,
			},
		},
	}
}

func mockDockerStrategyForImageRepository() buildv1.BuildStrategy {
	return buildv1.BuildStrategy{
		DockerStrategy: &buildv1.DockerBuildStrategy{
			NoCache: true,
			From: &corev1.ObjectReference{
				Kind:      "ImageStreamTag",
				Name:      imageRepoName + ":" + tagName,
				Namespace: imageRepoNamespace,
			},
		},
	}
}

func mockCustomStrategyForDockerImage(name string, options metav1.GetOptions) buildv1.BuildStrategy {
	return buildv1.BuildStrategy{
		CustomStrategy: &buildv1.CustomBuildStrategy{
			From: corev1.ObjectReference{
				Kind: "DockerImage",
				Name: originalImage,
			},
		},
	}
}

func mockCustomStrategyForImageRepository() buildv1.BuildStrategy {
	return buildv1.BuildStrategy{
		CustomStrategy: &buildv1.CustomBuildStrategy{
			From: corev1.ObjectReference{
				Kind:      "ImageStreamTag",
				Name:      imageRepoName + ":" + tagName,
				Namespace: imageRepoNamespace,
			},
		},
	}
}

func mockOutputWithImageName(name string, options metav1.GetOptions) buildv1.BuildOutput {
	return buildv1.BuildOutput{
		To: &corev1.ObjectReference{
			Kind: "DockerImage",
			Name: name,
		},
	}
}

func getBuildConfigFunc(buildConfigFunc func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error)) func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
	if buildConfigFunc == nil {
		return func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
			return MockBuildConfig(MockSource(), MockSourceStrategyForImageRepository(), MockOutput()), nil
		}
	}
	return buildConfigFunc
}

func getUpdateBuildConfigFunc(updateBuildConfigFunc func(ctx context.Context, buildConfig *buildv1.BuildConfig, _ metav1.UpdateOptions) error) func(ctx context.Context, buildConfig *buildv1.BuildConfig, _ metav1.UpdateOptions) error {
	if updateBuildConfigFunc == nil {
		return func(ctx context.Context, buildConfig *buildv1.BuildConfig, _ metav1.UpdateOptions) error {
			return nil
		}
	}
	return updateBuildConfigFunc
}

func getCreateBuildFunc(createBuildConfigFunc func(ctx context.Context, build *buildv1.Build, _ metav1.CreateOptions) error, b *buildv1.Build) func(ctx context.Context, build *buildv1.Build, _ metav1.CreateOptions) error {
	if createBuildConfigFunc == nil {
		return func(ctx context.Context, build *buildv1.Build, _ metav1.CreateOptions) error {
			*b = *build
			return nil
		}
	}
	return createBuildConfigFunc
}

func getGetBuildFunc(getBuildFunc func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.Build, error), b *buildv1.Build) func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.Build, error) {
	if getBuildFunc == nil {
		return func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.Build, error) {
			if b == nil {
				return &buildv1.Build{}, nil
			}
			return b, nil
		}
	}
	return getBuildFunc
}

func getGetImageStreamFunc(getImageStreamFunc func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error)) func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error) {
	if getImageStreamFunc == nil {
		return func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error) {
			if name != imageRepoName {
				return &imagev1.ImageStream{}, nil
			}
			return &imagev1.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Name:      imageRepoName,
					Namespace: imageRepoNamespace,
				},
				Status: imagev1.ImageStreamStatus{
					DockerImageRepository: "repo/namespace/image",
					Tags: []imagev1.NamedTagEventList{
						{
							Tag: tagName,
							Items: []imagev1.TagEvent{
								{DockerImageReference: dockerReference},
							},
						},
						{
							Tag: "latest",
							Items: []imagev1.TagEvent{
								{DockerImageReference: latestDockerReference, Image: "myid"},
							},
						},
					},
				},
			}, nil
		}
	}
	return getImageStreamFunc
}

func getGetImageStreamTagFunc(getImageStreamTagFunc func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamTag, error)) func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamTag, error) {
	if getImageStreamTagFunc == nil {
		return func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamTag, error) {
			return &imagev1.ImageStreamTag{
				Image: imagev1.Image{
					ObjectMeta:           metav1.ObjectMeta{Name: imageRepoName + ":" + newTag},
					DockerImageReference: latestDockerReference,
				},
			}, nil
		}
	}
	return getImageStreamTagFunc
}

func getGetImageStreamImageFunc(getImageStreamImageFunc func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamImage, error)) func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamImage, error) {
	if getImageStreamImageFunc == nil {
		return func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamImage, error) {
			return &imagev1.ImageStreamImage{
				Image: imagev1.Image{
					ObjectMeta:           metav1.ObjectMeta{Name: imageRepoName + ":@id"},
					DockerImageReference: latestDockerReference,
				},
			}, nil
		}
	}
	return getImageStreamImageFunc
}

func mockBuildGenerator(buildConfigFunc func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error),
	updateBuildConfigFunc func(ctx context.Context, buildConfig *buildv1.BuildConfig, _ metav1.UpdateOptions) error,
	createBuildFunc func(ctx context.Context, build *buildv1.Build, _ metav1.CreateOptions) error,
	getBuildFunc func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.Build, error),
	getImageStreamFunc func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStream, error),
	getImageStreamTagFunc func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamTag, error),
	getImageStreamImageFunc func(ctx context.Context, name string, options metav1.GetOptions) (*imagev1.ImageStreamImage, error),
) *BuildGenerator {
	fakeSecrets := []runtime.Object{}
	for _, s := range MockBuilderSecrets() {
		fakeSecrets = append(fakeSecrets, s)
	}
	b := buildv1.Build{}
	return &BuildGenerator{
		Secrets:         fake.NewSimpleClientset(fakeSecrets...).CoreV1(),
		ServiceAccounts: MockBuilderServiceAccount(MockBuilderSecrets()),
		Client: TestingClient{
			GetBuildConfigFunc:      getBuildConfigFunc(buildConfigFunc),
			UpdateBuildConfigFunc:   getUpdateBuildConfigFunc(updateBuildConfigFunc),
			CreateBuildFunc:         getCreateBuildFunc(createBuildFunc, &b),
			GetBuildFunc:            getGetBuildFunc(getBuildFunc, &b),
			GetImageStreamFunc:      getGetImageStreamFunc(getImageStreamFunc),
			GetImageStreamTagFunc:   getGetImageStreamTagFunc(getImageStreamTagFunc),
			GetImageStreamImageFunc: getGetImageStreamImageFunc(getImageStreamImageFunc),
		}}
}

func TestGenerateBuildFromConfigWithSecrets(t *testing.T) {
	source := MockSource()
	revision := &buildv1.SourceRevision{
		Git: &buildv1.GitSourceRevision{
			Commit: "abcd",
		},
	}
	dockerCfgTable := map[string]map[string][]byte{
		// FIXME: This imagev1 pull spec does not return ANY registry, but it should
		// return the hub.
		//"docker.io/secret2/image":     {".dockercfg": sampleDockerConfigs["hub"]},
		"secret1/image":               {".dockercfg": SampleDockerConfigs["hub"]},
		"1.1.1.1:5000/secret3/image":  {".dockercfg": SampleDockerConfigs["ipv4"]},
		"registry.host/secret4/image": {".dockercfg": SampleDockerConfigs["host"]},
	}
	for imageName := range dockerCfgTable {
		// Setup the BuildGenerator
		strategy := mockDockerStrategyForDockerImage(imageName, metav1.GetOptions{})
		output := mockOutputWithImageName(imageName, metav1.GetOptions{})
		generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
		bc := MockBuildConfig(source, strategy, output)
		build, err := generator.generateBuildFromConfig(apirequest.NewContext(), bc, revision, nil)

		if build.Spec.Strategy.DockerStrategy.PullSecret == nil {
			t.Errorf("Expected PullSecret for image '%s' to be set, got nil", imageName)
			continue
		}
		if len(build.Spec.Strategy.DockerStrategy.PullSecret.Name) == 0 {
			t.Errorf("Expected PullSecret for image %s to be set not empty", imageName)
		}
		if err != nil {
			t.Fatalf("Unexpected error %v", err)
		}
	}
}

func TestInstantiateBuildTriggerCauseConfigChange(t *testing.T) {
	changeMessage := "Build configuration change"

	buildTriggerCauses := []buildv1.BuildTriggerCause{}
	buildRequest := &buildv1.BuildRequest{
		TriggeredBy: append(buildTriggerCauses,
			buildv1.BuildTriggerCause{
				Message: changeMessage,
			},
		),
	}
	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	buildObject, err := generator.Instantiate(apirequest.NewDefaultContext(), buildRequest, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Expected error to be nil, got %v", err)
	}

	for _, cause := range buildObject.Spec.TriggeredBy {
		if cause.Message != changeMessage {
			t.Errorf("Expected reason %s, got %s", changeMessage, cause.Message)
		}
	}
}

func TestInstantiateBuildTriggerCauseImageChange(t *testing.T) {
	buildTriggerCauses := []buildv1.BuildTriggerCause{}
	changeMessage := "Image change"
	imageID := "centos@sha256:b3da5267165b"
	refName := "centos:7"
	refKind := "ImageStreamTag"

	buildRequest := &buildv1.BuildRequest{
		TriggeredBy: append(buildTriggerCauses,
			buildv1.BuildTriggerCause{
				Message: changeMessage,
				ImageChangeBuild: &buildv1.ImageChangeCause{
					ImageID: imageID,
					FromRef: &corev1.ObjectReference{
						Name: refName,
						Kind: refKind,
					},
				},
			},
		),
	}

	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	buildObject, err := generator.Instantiate(apirequest.NewDefaultContext(), buildRequest, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Expected error to be nil, got %v", err)
	}
	for _, cause := range buildObject.Spec.TriggeredBy {
		if cause.Message != "Image change" {
			t.Errorf("Expected reason %s, got %s", changeMessage, cause.Message)
		}
		if cause.ImageChangeBuild.ImageID != imageID {
			t.Errorf("Expected imageID: %s, got: %s", imageID, cause.ImageChangeBuild.ImageID)
		}
		if cause.ImageChangeBuild.FromRef.Name != refName {
			t.Errorf("Expected image name to be %s, got %s", refName, cause.ImageChangeBuild.FromRef.Name)
		}
		if cause.ImageChangeBuild.FromRef.Kind != refKind {
			t.Errorf("Expected image kind to be %s, got %s", refKind, cause.ImageChangeBuild.FromRef.Kind)
		}
	}
}

func TestInstantiateBuildTriggerCauseGenericWebHook(t *testing.T) {
	buildTriggerCauses := []buildv1.BuildTriggerCause{}
	changeMessage := "Generic WebHook"
	webHookSecret := "<secret>"

	gitRevision := &buildv1.SourceRevision{
		Git: &buildv1.GitSourceRevision{
			Author: buildv1.SourceControlUser{
				Name:  "John Doe",
				Email: "johndoe@test.com",
			},
			Message: "A random act of kindness",
		},
	}

	buildRequest := &buildv1.BuildRequest{
		TriggeredBy: append(buildTriggerCauses,
			buildv1.BuildTriggerCause{
				Message: changeMessage,
				GenericWebHook: &buildv1.GenericWebHookCause{
					Secret:   "<secret>",
					Revision: gitRevision,
				},
			},
		),
	}

	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	buildObject, err := generator.Instantiate(apirequest.NewDefaultContext(), buildRequest, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Expected error to be nil, got %v", err)
	}
	for _, cause := range buildObject.Spec.TriggeredBy {
		if cause.Message != changeMessage {
			t.Errorf("Expected reason %s, got %s", changeMessage, cause.Message)
		}
		if cause.GenericWebHook.Secret != webHookSecret {
			t.Errorf("Expected WebHook secret %s, got %s", webHookSecret, cause.GenericWebHook.Secret)
		}
		if !reflect.DeepEqual(gitRevision, cause.GenericWebHook.Revision) {
			t.Errorf("Expected return revision to match")
		}
	}
}

func TestInstantiateBuildTriggerCauseGitHubWebHook(t *testing.T) {
	buildTriggerCauses := []buildv1.BuildTriggerCause{}
	changeMessage := apiserverbuildutil.BuildTriggerCauseGithubMsg
	webHookSecret := "<secret>"

	gitRevision := &buildv1.SourceRevision{
		Git: &buildv1.GitSourceRevision{
			Author: buildv1.SourceControlUser{
				Name:  "John Doe",
				Email: "johndoe@test.com",
			},
			Message: "A random act of kindness",
		},
	}

	buildRequest := &buildv1.BuildRequest{
		TriggeredBy: append(buildTriggerCauses,
			buildv1.BuildTriggerCause{
				Message: changeMessage,
				GitHubWebHook: &buildv1.GitHubWebHookCause{
					Secret:   "<secret>",
					Revision: gitRevision,
				},
			},
		),
	}

	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	buildObject, err := generator.Instantiate(apirequest.NewDefaultContext(), buildRequest, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Expected error to be nil, got %v", err)
	}
	for _, cause := range buildObject.Spec.TriggeredBy {
		if cause.Message != changeMessage {
			t.Errorf("Expected reason %s, got %s", changeMessage, cause.Message)
		}
		if cause.GitHubWebHook.Secret != webHookSecret {
			t.Errorf("Expected WebHook secret %s, got %s", webHookSecret, cause.GitHubWebHook.Secret)
		}
		if !reflect.DeepEqual(gitRevision, cause.GitHubWebHook.Revision) {
			t.Errorf("Expected return revision to match")
		}
	}
}

func TestInstantiateBuildTriggerCauseGitLabWebHook(t *testing.T) {
	buildTriggerCauses := []buildv1.BuildTriggerCause{}
	changeMessage := apiserverbuildutil.BuildTriggerCauseGitLabMsg
	webHookSecret := "<secret>"

	gitRevision := &buildv1.SourceRevision{
		Git: &buildv1.GitSourceRevision{
			Author: buildv1.SourceControlUser{
				Name:  "John Doe",
				Email: "johndoe@test.com",
			},
			Message: "A random act of kindness",
		},
	}

	buildRequest := &buildv1.BuildRequest{
		TriggeredBy: append(buildTriggerCauses,
			buildv1.BuildTriggerCause{
				Message: changeMessage,
				GitLabWebHook: &buildv1.GitLabWebHookCause{
					CommonWebHookCause: buildv1.CommonWebHookCause{
						Revision: gitRevision,
						Secret:   "<secret>",
					},
				},
			},
		),
	}

	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	buildObject, err := generator.Instantiate(apirequest.NewDefaultContext(), buildRequest, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Expected error to be nil, got %v", err)
	}
	for _, cause := range buildObject.Spec.TriggeredBy {
		if cause.Message != changeMessage {
			t.Errorf("Expected reason %s, got %s", changeMessage, cause.Message)
		}
		if cause.GitLabWebHook.Secret != webHookSecret {
			t.Errorf("Expected WebHook secret %s, got %s", webHookSecret, cause.GitLabWebHook.Secret)
		}
		if !reflect.DeepEqual(gitRevision, cause.GitLabWebHook.Revision) {
			t.Errorf("Expected return revision to match")
		}
	}
}

func TestInstantiateBuildTriggerCauseBitbucketWebHook(t *testing.T) {
	buildTriggerCauses := []buildv1.BuildTriggerCause{}
	changeMessage := apiserverbuildutil.BuildTriggerCauseBitbucketMsg
	webHookSecret := "<secret>"

	gitRevision := &buildv1.SourceRevision{
		Git: &buildv1.GitSourceRevision{
			Author: buildv1.SourceControlUser{
				Name:  "John Doe",
				Email: "johndoe@test.com",
			},
			Message: "A random act of kindness",
		},
	}

	buildRequest := &buildv1.BuildRequest{
		TriggeredBy: append(buildTriggerCauses,
			buildv1.BuildTriggerCause{
				Message: changeMessage,
				BitbucketWebHook: &buildv1.BitbucketWebHookCause{
					CommonWebHookCause: buildv1.CommonWebHookCause{
						Secret:   "<secret>",
						Revision: gitRevision,
					},
				},
			},
		),
	}

	generator := mockBuildGenerator(nil, nil, nil, nil, nil, nil, nil)
	buildObject, err := generator.Instantiate(apirequest.NewDefaultContext(), buildRequest, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Expected error to be nil, got %v", err)
	}
	for _, cause := range buildObject.Spec.TriggeredBy {
		if cause.Message != changeMessage {
			t.Errorf("Expected reason %s, got %s", changeMessage, cause.Message)
		}
		if cause.BitbucketWebHook.Secret != webHookSecret {
			t.Errorf("Expected WebHook secret %s, got %s", webHookSecret, cause.BitbucketWebHook.Secret)
		}
		if !reflect.DeepEqual(gitRevision, cause.BitbucketWebHook.Revision) {
			t.Errorf("Expected return revision to match")
		}
	}
}

func TestOverrideDockerStrategyNoCacheOption(t *testing.T) {
	buildConfigFunc := func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
		return &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceDefault,
			},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Source: MockSource(),
					Strategy: buildv1.BuildStrategy{
						DockerStrategy: &buildv1.DockerBuildStrategy{
							NoCache: true,
						},
					},
					Revision: &buildv1.SourceRevision{
						Git: &buildv1.GitSourceRevision{
							Commit: "1234",
						},
					},
				},
			},
		}, nil
	}

	g := mockBuildGenerator(buildConfigFunc, nil, nil, nil, nil, nil, nil)
	build, err := g.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected error encountered:  %v", err)
	}
	if build.Spec.Strategy.DockerStrategy.NoCache != true {
		t.Errorf("Spec.Strategy.DockerStrategy.NoCache was overwritten by nil buildRequest option, but should not have been")
	}
}

func TestOverrideSourceStrategyIncrementalOption(t *testing.T) {
	myTrue := true
	buildConfigFunc := func(ctx context.Context, name string, options metav1.GetOptions) (*buildv1.BuildConfig, error) {
		return &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceDefault,
			},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Source: MockSource(),
					Strategy: buildv1.BuildStrategy{
						SourceStrategy: &buildv1.SourceBuildStrategy{
							Incremental: &myTrue,
							From: corev1.ObjectReference{
								Kind:      "ImageStreamTag",
								Name:      "testRepo:test",
								Namespace: "testns",
							},
						},
					},
					Revision: &buildv1.SourceRevision{
						Git: &buildv1.GitSourceRevision{
							Commit: "1234",
						},
					},
				},
			},
		}, nil
	}

	g := mockBuildGenerator(buildConfigFunc, nil, nil, nil, nil, nil, nil)
	build, err := g.Instantiate(apirequest.NewDefaultContext(), &buildv1.BuildRequest{}, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Unexpected error encountered:  %v", err)
	}
	if *build.Spec.Strategy.SourceStrategy.Incremental != true {
		t.Errorf("Spec.Strategy.SourceStrategy.Incremental was overwritten by nil buildRequest option, but should not have been")
	}
}

func TestLabelValue(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedOutput string
	}{
		{
			name:           "allow-decimals",
			input:          "my.label.with.decimals",
			expectedOutput: "my.label.with.decimals",
		},
		{
			name:           "do-not-end-with-a-decimal",
			input:          "my.label.ends.with.a.decimal.",
			expectedOutput: "my.label.ends.with.a.decimal",
		},
		{
			name:           "allow-hyphens",
			input:          "my-label-with-hyphens",
			expectedOutput: "my-label-with-hyphens",
		},
		{
			name:           "do-not-end-with-a-hyphen",
			input:          "my-label-ends-with-a-hyphen-",
			expectedOutput: "my-label-ends-with-a-hyphen",
		},
		{
			name:           "allow-underscores",
			input:          "my_label_with_underscores",
			expectedOutput: "my_label_with_underscores",
		},
		{
			name:           "do-not-end-with-an-underscore",
			input:          "my_label_ends_with_an_underscore_",
			expectedOutput: "my_label_ends_with_an_underscore",
		},
		{
			name:           "truncate-to-63-characters",
			input:          "myreallylonglabelthatshouldbelessthan63charactersbutismorethanthat",
			expectedOutput: "myreallylonglabelthatshouldbelessthan63charactersbutismorethant",
		},
		{
			name:           "allow-a-label-with-semantic-versioning",
			input:          "some-label-v4.3.2-beta3",
			expectedOutput: "some-label-v4.3.2-beta3",
		},
	}

	for _, tc := range testCases {
		result := labelValue(tc.input)
		if result != tc.expectedOutput {
			t.Errorf("tc %s got %s for %s instead of %s", tc.name, result, tc.input, tc.expectedOutput)
		}
	}
}

var (
	Encode = func(src string) []byte {
		return []byte(src)
	}

	SampleDockerConfigs = map[string][]byte{
		"hub":  Encode(`{"https://index.docker.io/v1/":{"auth": "Zm9vOmJhcgo=", "email": ""}}`),
		"ipv4": Encode(`{"https://1.1.1.1:5000/v1/":{"auth": "Zm9vOmJhcgo=", "email": ""}}`),
		"host": Encode(`{"https://registry.host/v1/":{"auth": "Zm9vOmJhcgo=", "email": ""}}`),
	}
)

func MockBuilderSecrets() []*corev1.Secret {
	var secrets []*corev1.Secret
	for name, conf := range SampleDockerConfigs {
		secrets = append(secrets, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: metav1.NamespaceDefault,
			},
			Type: corev1.SecretTypeDockercfg,
			Data: map[string][]byte{".dockercfg": conf},
		})
	}
	return secrets
}

func MockBuilderServiceAccount(secrets []*corev1.Secret) corev1client.ServiceAccountsGetter {
	var (
		secretRefs  []corev1.ObjectReference
		fakeObjects []runtime.Object
	)
	for _, secret := range secrets {
		secretRefs = append(secretRefs, corev1.ObjectReference{
			Name: secret.Name,
			Kind: "Secret",
		})
		fakeObjects = append(fakeObjects, secret)
	}
	fakeObjects = append(fakeObjects, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrappolicy.BuilderServiceAccountName,
			Namespace: metav1.NamespaceDefault,
		},
		Secrets: secretRefs,
	})
	return fake.NewSimpleClientset(fakeObjects...).CoreV1()
}

func MockBuildConfig(source buildv1.BuildSource, strategy buildv1.BuildStrategy, output buildv1.BuildOutput) *buildv1.BuildConfig {
	return &buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-build-config",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"testbclabel": "testbcvalue",
			},
		},
		Spec: buildv1.BuildConfigSpec{
			CommonSpec: buildv1.CommonSpec{
				Source: source,
				Revision: &buildv1.SourceRevision{
					Git: &buildv1.GitSourceRevision{
						Commit: "1234",
					},
				},
				Strategy: strategy,
				Output:   output,
			},
		},
	}
}

func MockSource() buildv1.BuildSource {
	return buildv1.BuildSource{
		Git: &buildv1.GitBuildSource{
			URI: "http://test.repository/namespace/name",
			Ref: "test-tag",
		},
	}
}

func MockSourceStrategyForImageRepository() buildv1.BuildStrategy {
	return buildv1.BuildStrategy{
		SourceStrategy: &buildv1.SourceBuildStrategy{
			From: corev1.ObjectReference{
				Kind:      "ImageStreamTag",
				Name:      imageRepoName + ":" + tagName,
				Namespace: imageRepoNamespace,
			},
		},
	}
}

func MockSourceStrategyForEnvs() buildv1.BuildStrategy {
	return buildv1.BuildStrategy{
		SourceStrategy: &buildv1.SourceBuildStrategy{
			Env: []corev1.EnvVar{{Name: "FOO", Value: "VAR"}},
			From: corev1.ObjectReference{
				Kind:      "ImageStreamTag",
				Name:      imageRepoName + ":" + tagName,
				Namespace: imageRepoNamespace,
			},
		},
	}
}

func MockDockerStrategyForEnvs() buildv1.BuildStrategy {
	return buildv1.BuildStrategy{
		DockerStrategy: &buildv1.DockerBuildStrategy{
			Env: []corev1.EnvVar{{Name: "FOO", Value: "VAR"}},
			From: &corev1.ObjectReference{
				Kind:      "ImageStreamTag",
				Name:      imageRepoName + ":" + tagName,
				Namespace: imageRepoNamespace,
			},
		},
	}
}

func MockCustomStrategyForEnvs() buildv1.BuildStrategy {
	return buildv1.BuildStrategy{
		CustomStrategy: &buildv1.CustomBuildStrategy{
			Env: []corev1.EnvVar{{Name: "FOO", Value: "VAR"}},
			From: corev1.ObjectReference{
				Kind:      "ImageStreamTag",
				Name:      imageRepoName + ":" + tagName,
				Namespace: imageRepoNamespace,
			},
		},
	}
}

func MockJenkinsStrategyForEnvs() buildv1.BuildStrategy {
	return buildv1.BuildStrategy{
		JenkinsPipelineStrategy: &buildv1.JenkinsPipelineBuildStrategy{
			Env: []corev1.EnvVar{{Name: "FOO", Value: "VAR"}},
		},
	}
}

func MockOutput() buildv1.BuildOutput {
	return buildv1.BuildOutput{
		To: &corev1.ObjectReference{
			Kind: "DockerImage",
			Name: "localhost:5000/test/image-tag",
		},
	}
}

func MockImageStream(repoName, dockerImageRepo string, tags map[string]string) *imagev1.ImageStream {
	tagHistory := []imagev1.NamedTagEventList{}
	for tag, imageID := range tags {
		tagHistory = append(tagHistory, imagev1.NamedTagEventList{
			Tag: tag,
			Items: []imagev1.TagEvent{
				{
					Image:                imageID,
					DockerImageReference: fmt.Sprintf("%s:%s", dockerImageRepo, imageID),
				},
			},
		})
	}

	return &imagev1.ImageStream{
		ObjectMeta: metav1.ObjectMeta{
			Name: repoName,
		},
		Status: imagev1.ImageStreamStatus{
			DockerImageRepository: dockerImageRepo,
			Tags:                  tagHistory,
		},
	}
}
