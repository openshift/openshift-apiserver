package imagestreamimage

import (
	"context"
	"testing"

	etcd "go.etcd.io/etcd/client/v3"
	authorizationapi "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	etcdtesting "k8s.io/apiserver/pkg/storage/etcd3/testing"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/kubernetes/pkg/api/legacyscheme"

	imagev1 "github.com/openshift/api/image/v1"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation/fake"
	admfake "github.com/openshift/openshift-apiserver/pkg/image/apiserver/admission/fake"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/internal/testutil"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/image"
	imageetcd "github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/image/etcd"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/imagestream"
	imagestreametcd "github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/imagestream/etcd"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registryhostname"

	_ "github.com/openshift/openshift-apiserver/pkg/api/install"
)

type fakeSubjectAccessReviewRegistry struct{}

func (f *fakeSubjectAccessReviewRegistry) Create(_ context.Context, subjectAccessReview *authorizationapi.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationapi.SubjectAccessReview, error) {
	return nil, nil
}

func (f *fakeSubjectAccessReviewRegistry) CreateContext(ctx context.Context, subjectAccessReview *authorizationapi.SubjectAccessReview) (*authorizationapi.SubjectAccessReview, error) {
	return f.Create(ctx, subjectAccessReview, metav1.CreateOptions{})
}

func TestGet(t *testing.T) {
	tests := map[string]struct {
		input                   string
		repo                    *imageapi.ImageStream
		images                  []*imageapi.Image
		expectedImageMetadataID string
		expectedConfigHostname  string
		expectError             bool
	}{
		"empty string": {
			input:       "",
			expectError: true,
		},
		"one part": {
			input:       "a",
			expectError: true,
		},
		"more than 2 parts": {
			input:       "a@b@c",
			expectError: true,
		},
		"empty name part": {
			input:       "@id",
			expectError: true,
		},
		"empty id part": {
			input:       "name@",
			expectError: true,
		},
		"repo not found": {
			input:       "repo@id",
			repo:        nil,
			expectError: true,
		},
		"nil tags": {
			input:       "repo@id",
			repo:        &imageapi.ImageStream{},
			expectError: true,
		},
		"image not found": {
			input: "repo@id",
			repo: &imageapi.ImageStream{
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{Image: "anotherid"},
							},
						},
					},
				},
			},
			expectError: true,
		},
		"happy path": {
			input: "repo@" + testutil.KindestConfigDigest,
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "repo",
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{Image: "anotherid"},
								{Image: "anotherid2"},
								{Image: testutil.KindestConfigDigest},
							},
						},
					},
				},
			},
			expectedImageMetadataID: testutil.KindestConfigDigest,
			expectedConfigHostname:  "5e7483a6cf0e",
			images: []*imageapi.Image{
				testutil.MustKindestCompleteImage(),
			},
		},
		"uses annotations from image stream": {
			input:       "repo@sha256:abc321",
			expectError: false,
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "repo",
					Annotations: map[string]string{
						"test":         "123",
						"another-test": "abc",
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{Image: "anotherid"},
								{Image: "anotherid2"},
								{Image: "sha256:abc321"},
							},
						},
					},
				},
			},
			images: []*imageapi.Image{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sha256:abc321",
					},
				},
			},
		},
		"matches partial sha": {
			input:       "repo@sha256:ff46b782",
			expectError: false,
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "repo",
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{Image: "anotherid"},
								{Image: "anotherid2"},
								{Image: "sha256:ff46b78279f207db3b8e57e20dee7cecef3567d09489369d80591f150f9c8154"},
							},
						},
					},
				},
			},
			images: []*imageapi.Image{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sha256:ff46b78279f207db3b8e57e20dee7cecef3567d09489369d80591f150f9c8154",
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server, etcdStorage := etcdtesting.NewUnsecuredEtcd3TestClientServer(t)
			defer server.Terminate(t)
			etcdStorage.Codec = legacyscheme.Codecs.LegacyCodec(
				schema.GroupVersion{Group: "image.openshift.io", Version: "v1"})
			client := etcd.NewKV(server.V3Client.Client)
			etcdStorageConfigForImages := &storagebackend.ConfigForResource{
				Config:        *etcdStorage,
				GroupResource: schema.GroupResource{Group: "image.openshift.io", Resource: "images"},
			}
			imageRESTOptions := generic.RESTOptions{
				StorageConfig:           etcdStorageConfigForImages,
				Decorator:               generic.UndecoratedStorage,
				DeleteCollectionWorkers: 1,
				ResourcePrefix:          "images",
			}
			imageStorage, err := imageetcd.NewREST(imageRESTOptions)
			if err != nil {
				t.Fatal(err)
			}
			defaultRegistry := registryhostname.DefaultRegistryHostnameRetriever("", "defaultregistry:5000")
			etcdStorageConfigForImageStreams := &storagebackend.ConfigForResource{
				Config:        *etcdStorage,
				GroupResource: schema.GroupResource{Group: "image.openshift.io", Resource: "imagestreams"},
			}
			imagestreamRESTOptions := generic.RESTOptions{
				StorageConfig:           etcdStorageConfigForImageStreams,
				Decorator:               generic.UndecoratedStorage,
				DeleteCollectionWorkers: 1,
				ResourcePrefix:          "imagestreams",
			}
			imageIndex := imagestreametcd.NewMockImageLayerIndex()
			imageStreamStorage, imageStreamLayersStorage, imageStreamStatus, internalStorage, err := imagestreametcd.NewRESTWithLimitVerifier(
				imagestreamRESTOptions,
				defaultRegistry,
				&fakeSubjectAccessReviewRegistry{},
				&admfake.ImageStreamLimitVerifier{},
				&fake.RegistryWhitelister{},
				imageIndex,
			)
			if err != nil {
				t.Fatal(err)
			}

			imageRegistry := image.NewRegistry(imageStorage)
			imageStreamRegistry := imagestream.NewRegistry(
				imageStreamStorage,
				imageStreamStatus,
				internalStorage,
				imageStreamLayersStorage,
			)

			storage := NewREST(imageRegistry, imageStreamRegistry)
			ctx := apirequest.NewDefaultContext()

			if test.repo != nil {
				ctx = apirequest.WithNamespace(apirequest.NewContext(), test.repo.Namespace)
				_, err := client.Put(
					context.TODO(),
					etcdtesting.AddPrefix("/imagestreams/"+test.repo.Namespace+"/"+test.repo.Name),
					runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion), test.repo),
				)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
			}
			if len(test.images) > 0 {
				for _, image := range test.images {
					_, err := client.Put(
						context.TODO(),
						etcdtesting.AddPrefix("/images/"+image.Name),
						runtime.EncodeOrDie(
							legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion),
							image,
						),
					)
					if err != nil {
						t.Fatalf("Unexpected error: %v", err)
					}
					imageIndex.Add(&imagev1.Image{
						ObjectMeta:          image.ObjectMeta,
						DockerImageManifest: image.DockerImageManifest,
					})
				}
			}

			obj, err := storage.Get(ctx, test.input, &metav1.GetOptions{})
			if test.expectError {
				if err == nil {
					t.Fatal("expected error but didn't get one")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %#v", err)
			}

			imageStreamImage := obj.(*imageapi.ImageStreamImage)
			// validate a couple of the fields
			if e, a := test.repo.Namespace, "ns"; e != a {
				t.Errorf("%s: namespace: expected %q, got %q", name, e, a)
			}
			if e, a := test.input, imageStreamImage.Name; e != a {
				t.Errorf("%s: name: expected %q, got %q", name, e, a)
			}

			expectedAnnotations := test.repo.ObjectMeta.Annotations
			gotAnnotations := imageStreamImage.ObjectMeta.Annotations
			if !equality.Semantic.DeepEqual(expectedAnnotations, gotAnnotations) {
				t.Error("Expected image stream annotations to match image stream image's")
				t.Log(diff.ObjectGoPrintDiff(expectedAnnotations, gotAnnotations))
			}

			expectedID := test.expectedImageMetadataID
			if expectedID != "" && expectedID != imageStreamImage.Image.DockerImageMetadata.ID {
				t.Errorf("id: expected %q, got %q", expectedID, imageStreamImage.Image.DockerImageMetadata.ID)
			}
			expectedConfigHostname := test.expectedConfigHostname
			hostname := imageStreamImage.Image.DockerImageMetadata.ContainerConfig.Hostname
			if expectedConfigHostname != "" && expectedConfigHostname != hostname {
				t.Errorf("container config hostname: expected %q, got %q", expectedConfigHostname, hostname)
			}
		})
	}
}
