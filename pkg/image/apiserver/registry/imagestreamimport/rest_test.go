package imagestreamimport

import (
	"context"
	"fmt"
	"testing"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	kapi "k8s.io/kubernetes/pkg/apis/core"
	kapihelper "k8s.io/kubernetes/pkg/apis/core/helper"

	"github.com/openshift/library-go/pkg/image/reference"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
)

func mockImage(digest string) *imageapi.Image {
	return &imageapi.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: digest,
		},
		DockerImageReference: "registry.com/namespace/image@" + digest,
	}
}

type fakeImageCreater struct {
	count  int
	errors map[string]error
	images map[string]*imageapi.Image
}

func (_ fakeImageCreater) New() runtime.Object {
	return nil
}

func (f *fakeImageCreater) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	image, ok := obj.(*imageapi.Image)
	if !ok {
		panic(fmt.Errorf("wrong object passed to fakeImageCreater: %#v", obj))
	}

	f.count++

	if f.images != nil {
		f.images[image.Name] = image
	}

	if err, ok := f.errors[image.Name]; ok {
		return nil, err
	}

	return obj, nil
}

func TestImportSuccessful(t *testing.T) {
	const (
		tag              = "mytag"
		imageDigest      = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
		imageReference   = "registry.com/namespace/image@" + imageDigest
		anotherDigest    = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
		anotherReference = "registry.com/namespace/image@" + anotherDigest
	)

	one := int64(1)
	two := int64(2)
	now := metav1.Now()
	tests := map[string]struct {
		image                       *imageapi.Image
		stream                      *imageapi.ImageStream
		importReferencePolicyType   imageapi.TagReferencePolicyType
		expectedTagEvent            imageapi.TagEvent
		expectedReferencePolicyType imageapi.TagReferencePolicyType
	}{
		"reference differs": {
			image: &imageapi.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name: imageDigest,
				},
				DockerImageReference: imageReference,
			},
			stream: &imageapi.ImageStream{
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						tag: {
							Name: tag,
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.com/namespace/image:mytag",
							},
							Generation: &one,
						},
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						tag: {
							Items: []imageapi.TagEvent{{
								DockerImageReference: anotherReference,
								Image:                anotherDigest,
								Generation:           one,
							}},
						},
					},
				},
			},
			expectedTagEvent: imageapi.TagEvent{
				Created:              now,
				DockerImageReference: imageReference,
				Image:                imageDigest,
				Generation:           two,
			},
			importReferencePolicyType:   imageapi.SourceTagReferencePolicy,
			expectedReferencePolicyType: imageapi.SourceTagReferencePolicy,
		},
		"image differs": {
			image: &imageapi.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name: imageDigest,
				},
				DockerImageReference: imageReference,
			},
			stream: &imageapi.ImageStream{
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						tag: {
							Name: tag,
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.com/namespace/image:mytag",
							},
							Generation: &one,
						},
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						tag: {
							Items: []imageapi.TagEvent{{
								DockerImageReference: "registry.com/namespace/image:othertag",
								Image:                "non-image",
								Generation:           one,
							}},
						},
					},
				},
			},
			expectedTagEvent: imageapi.TagEvent{
				Created:              now,
				DockerImageReference: imageReference,
				Image:                imageDigest,
				Generation:           two,
			},
			importReferencePolicyType:   imageapi.LocalTagReferencePolicy,
			expectedReferencePolicyType: imageapi.LocalTagReferencePolicy,
		},
		"empty status": {
			image: &imageapi.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name: imageDigest,
				},
				DockerImageReference: imageReference,
			},
			stream: &imageapi.ImageStream{
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						tag: {
							Name: tag,
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.com/namespace/image:mytag",
							},
							Generation: &one,
							ReferencePolicy: imageapi.TagReferencePolicy{
								Type: imageapi.SourceTagReferencePolicy,
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{},
			},
			expectedTagEvent: imageapi.TagEvent{
				Created:              now,
				DockerImageReference: imageReference,
				Image:                imageDigest,
				Generation:           two,
			},
			importReferencePolicyType:   imageapi.LocalTagReferencePolicy,
			expectedReferencePolicyType: imageapi.SourceTagReferencePolicy,
		},
		// https://github.com/openshift/origin/issues/10402:
		"only generation differ": {
			image: &imageapi.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name: imageDigest,
				},
				DockerImageReference: imageReference,
			},
			stream: &imageapi.ImageStream{
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						tag: {
							Name: tag,
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.com/namespace/image:mytag",
							},
							Generation: &two,
							ReferencePolicy: imageapi.TagReferencePolicy{
								Type: imageapi.LocalTagReferencePolicy,
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						tag: {
							Items: []imageapi.TagEvent{{
								DockerImageReference: imageReference,
								Image:                imageDigest,
								Generation:           one,
							}},
						},
					},
				},
			},
			expectedTagEvent: imageapi.TagEvent{
				DockerImageReference: imageReference,
				Image:                imageDigest,
				Generation:           two,
			},
			importReferencePolicyType:   imageapi.SourceTagReferencePolicy,
			expectedReferencePolicyType: imageapi.LocalTagReferencePolicy,
		},
	}

	for name, test := range tests {
		ref, err := reference.Parse(test.image.DockerImageReference)
		if err != nil {
			t.Errorf("%s: error parsing image ref: %v", name, err)
			continue
		}

		importPolicy := imageapi.TagImportPolicy{}
		referencePolicy := imageapi.TagReferencePolicy{Type: test.importReferencePolicyType}
		storage := REST{
			images: &fakeImageCreater{},
		}
		imageCreater := newCachedImageCreater(nil, storage.images)
		_, _, err = storage.importSuccessful(apirequest.NewDefaultContext(), test.image, nil, test.stream,
			tag, ref.Exact(), two, now, importPolicy, referencePolicy, imageCreater)
		if err != nil {
			t.Errorf("%s: expected success, got: %v", name, err)
		}
		actual := test.stream.Status.Tags[tag].Items[0]
		if !kapihelper.Semantic.DeepEqual(actual, test.expectedTagEvent) {
			t.Errorf("%s: expected %#v, got %#v", name, test.expectedTagEvent, actual)
		}

		actualRefType := test.stream.Spec.Tags[tag].ReferencePolicy.Type
		if actualRefType != test.expectedReferencePolicyType {
			t.Errorf("%s: expected %#v, got %#v", name, test.expectedReferencePolicyType, actualRefType)
		}
	}
}

func TestImportSuccessfulWithSubmanifests(t *testing.T) {
	const (
		tag         = "mytag"
		imageDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
		sub1Digest  = "sha256:0000000000000000000000000000000000000000000000000000000000000001"
		sub2Digest  = "sha256:0000000000000000000000000000000000000000000000000000000000000002"
	)

	one := int64(1)
	two := int64(2)
	now := metav1.Now()
	tests := []struct {
		name          string
		image         *imageapi.Image
		manifests     []imageapi.Image
		stream        *imageapi.ImageStream
		createrErrors map[string]error
	}{
		{
			name:  "FirstImport",
			image: mockImage(imageDigest),
			manifests: []imageapi.Image{
				*mockImage(sub1Digest),
				*mockImage(sub2Digest),
			},
			stream: &imageapi.ImageStream{
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						tag: {
							Name: tag,
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.com/namespace/image:mytag",
							},
							Generation: &one,
							ImportPolicy: imageapi.TagImportPolicy{
								ImportMode: imageapi.ImportModePreserveOriginal,
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{},
			},
		},
		{
			name:  "SubmanifestExists",
			image: mockImage(imageDigest),
			manifests: []imageapi.Image{
				*mockImage(sub1Digest),
				*mockImage(sub2Digest),
			},
			stream: &imageapi.ImageStream{
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						tag: {
							Name: tag,
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.com/namespace/image:mytag",
							},
							Generation: &one,
							ImportPolicy: imageapi.TagImportPolicy{
								ImportMode: imageapi.ImportModePreserveOriginal,
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{},
			},
			createrErrors: map[string]error{
				sub1Digest: kerrors.NewAlreadyExists(imageapi.Resource("image"), sub1Digest),
			},
		},
		{
			name:  "ManifestListExists",
			image: mockImage(imageDigest),
			manifests: []imageapi.Image{
				*mockImage(sub1Digest),
				*mockImage(sub2Digest),
			},
			stream: &imageapi.ImageStream{
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						tag: {
							Name: tag,
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.com/namespace/image:mytag",
							},
							Generation: &one,
							ImportPolicy: imageapi.TagImportPolicy{
								ImportMode: imageapi.ImportModePreserveOriginal,
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{},
			},
			createrErrors: map[string]error{
				imageDigest: kerrors.NewAlreadyExists(imageapi.Resource("image"), imageDigest),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref, err := reference.Parse(test.image.DockerImageReference)
			if err != nil {
				t.Fatalf("error parsing image ref: %v", err)
			}

			importPolicy := imageapi.TagImportPolicy{}
			referencePolicy := imageapi.TagReferencePolicy{
				Type: imageapi.SourceTagReferencePolicy,
			}
			restImageCreater := &fakeImageCreater{
				errors: test.createrErrors,
				images: make(map[string]*imageapi.Image),
			}
			storage := REST{
				images: restImageCreater,
			}
			imageCreater := newCachedImageCreater(nil, storage.images)

			_, updatedSubmanifests, err := storage.importSuccessful(
				apirequest.NewDefaultContext(),
				test.image,
				test.manifests,
				test.stream,
				tag,
				ref.Exact(),
				two,
				now,
				importPolicy,
				referencePolicy,
				imageCreater,
			)
			if err != nil {
				t.Errorf("expected success, got: %v", err)
			}
			if len(updatedSubmanifests) != len(test.manifests) {
				t.Fatalf("got %d updated submanifests, expected %d", len(updatedSubmanifests), len(test.manifests))
			}
			for i, manifest := range test.manifests {
				updatedSubmanifest := updatedSubmanifests[i]
				if updatedSubmanifest.Name != manifest.Name {
					t.Errorf("got updated submanifest %d name %q, expected %q", i, updatedSubmanifest.Name, manifest.Name)
				}
			}
			if restImageCreater.count != 1+len(test.manifests) {
				t.Errorf("expected %d images to be created, got %d", 1+len(test.manifests), restImageCreater.count)
			}
			if _, ok := restImageCreater.images[test.image.Name]; !ok {
				t.Errorf("expected image %s to be created", test.image.Name)
			}
			for _, submanifest := range test.manifests {
				if _, ok := restImageCreater.images[submanifest.Name]; !ok {
					t.Errorf("expected subimage %s to be created", submanifest.Name)
				}
			}

			// check that cached results in updatedImages are used and don't cause extra requests to the server
			_, updatedSubmanifests, err = storage.importSuccessful(
				apirequest.NewDefaultContext(),
				test.image,
				test.manifests,
				test.stream,
				tag,
				ref.Exact(),
				two,
				now,
				importPolicy,
				referencePolicy,
				imageCreater,
			)
			if err != nil {
				t.Errorf("expected success, got: %v", err)
			}
			if restImageCreater.count != 1+len(test.manifests) {
				t.Errorf("expected %d images to be created, got %d", 1+len(test.manifests), restImageCreater.count)
			}
			if len(updatedSubmanifests) != len(test.manifests) {
				t.Fatalf("got %d updated submanifests, expected %d", len(updatedSubmanifests), len(test.manifests))
			}
			for i, manifest := range test.manifests {
				updatedSubmanifest := updatedSubmanifests[i]
				if updatedSubmanifest.Name != manifest.Name {
					t.Errorf("got updated submanifest %d name %q, expected %q", i, updatedSubmanifest.Name, manifest.Name)
				}
			}
		})
	}
}

func TestCreateImages(t *testing.T) {
	one := int64(1)
	tag := "mytag"
	imageDigest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	sub1Digest := "sha256:0000000000000000000000000000000000000000000000000000000000000001"
	sub2Digest := "sha256:0000000000000000000000000000000000000000000000000000000000000002"

	testCases := []struct {
		name              string
		expectedCondition imageapi.TagEventCondition
		imageImportStatus imageapi.ImageImportStatus
		expectedCallCount int
	}{
		{
			name:              "successfulImport",
			expectedCondition: imageapi.TagEventCondition{},
			imageImportStatus: imageapi.ImageImportStatus{
				Image: mockImage(imageDigest),
				Manifests: []imageapi.Image{
					*mockImage(sub1Digest),
					*mockImage(sub2Digest),
				},
				Status: metav1.Status{
					Status: metav1.StatusSuccess,
				},
			},
			expectedCallCount: 3,
		},
		{
			name: "failedImport",
			expectedCondition: imageapi.TagEventCondition{
				Type:    imageapi.ImportSuccess,
				Status:  kapi.ConditionFalse,
				Message: "unknown error prevented import",
			},
			imageImportStatus: imageapi.ImageImportStatus{
				Status: metav1.Status{
					Status: metav1.StatusFailure,
				},
			},
			expectedCallCount: 0,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			imageCreater := &fakeImageCreater{
				images: make(map[string]*imageapi.Image),
			}
			storage := REST{
				images: imageCreater,
			}
			ctx := context.Background()
			is := &imageapi.ImageStream{
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						tag: {
							Name: tag,
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.com/namespace/image:mytag",
							},
							Generation: &one,
							ImportPolicy: imageapi.TagImportPolicy{
								ImportMode: imageapi.ImportModePreserveOriginal,
							},
						},
					},
				},
			}
			isi := &imageapi.ImageStreamImport{
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							ImportPolicy: imageapi.TagImportPolicy{
								ImportMode: imageapi.ImportModePreserveOriginal,
							},
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.com/namespace/image:mytag",
							},
							To: &kapi.LocalObjectReference{
								Name: "mytag",
							},
						},
					},
				},
				Status: imageapi.ImageStreamImportStatus{
					Import: is,
					Images: []imageapi.ImageImportStatus{testCase.imageImportStatus},
				},
			}
			err := storage.createImages(ctx, isi, is, one, metav1.NewTime(time.Now()))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if imageCreater.count != testCase.expectedCallCount {
				t.Errorf("expected %d images to be created, got %d", 3, imageCreater.count)
			}

			tagname := "mytag"
			conditions := is.Status.Tags[tagname].Conditions
			emptyCondition := imageapi.TagEventCondition{}
			if testCase.expectedCondition == emptyCondition {
				if len(conditions) != 0 {
					t.Fatalf("unexpected conditions found, wanted nil got %#v", conditions)
				}
				return
			}
			condition := conditions[0]
			if condition.Type != testCase.expectedCondition.Type {
				t.Errorf("unexpected condition type, wanted %q, got %q", testCase.expectedCondition.Type, condition.Type)
			}
			if condition.Status != testCase.expectedCondition.Status {
				t.Errorf("unexpected condition status, wanted %q, got %q", testCase.expectedCondition.Status, condition.Status)
			}
			if condition.Message != testCase.expectedCondition.Message {
				t.Errorf("unexpected condition message, wanted %q, got %q", testCase.expectedCondition.Message, condition.Message)
			}
		})
	}
	// expectedCondition := imageapi.TagEventCondition{
	// 	Type:               imageapi.ImportSuccess,
	// 	Status:             kapi.ConditionTrue,
	// 	// Message:            message,
	// 	// Reason:             string(status.Status.Reason),
	// 	// Generation:         nextGeneration,
	// 	// LastTransitionTime: now,
	// }
	// success image stream condition should be empty
}
