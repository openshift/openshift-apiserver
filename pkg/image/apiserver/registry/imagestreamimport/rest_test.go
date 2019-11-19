package imagestreamimport

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	kapi "k8s.io/kubernetes/pkg/apis/core"
	kapihelper "k8s.io/kubernetes/pkg/apis/core/helper"

	"github.com/openshift/library-go/pkg/image/reference"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
)

type fakeImageCreater struct{}

func (_ fakeImageCreater) New() runtime.Object {
	return nil
}

func (_ fakeImageCreater) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
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
		importedImages := make(map[string]error)
		updatedImages := make(map[string]*imageapi.Image)
		storage := REST{images: fakeImageCreater{}}
		_, ok := storage.importSuccessful(apirequest.NewDefaultContext(), test.image, test.stream,
			tag, ref.Exact(), two, now, importPolicy, referencePolicy, importedImages, updatedImages)
		if !ok {
			t.Errorf("%s: expected success, didn't get one", name)
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
