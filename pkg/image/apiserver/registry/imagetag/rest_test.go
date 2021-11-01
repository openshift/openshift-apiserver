package imagetag

import (
	"context"
	"reflect"
	"testing"
	"time"

	etcd "go.etcd.io/etcd/client/v3"
	authorizationapi "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apiserver/pkg/authentication/user"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	etcdtesting "k8s.io/apiserver/pkg/storage/etcd3/testing"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	kapi "k8s.io/kubernetes/pkg/apis/core"

	imagev1 "github.com/openshift/api/image/v1"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation/fake"
	admfake "github.com/openshift/openshift-apiserver/pkg/image/apiserver/admission/fake"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/image"
	imageetcd "github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/image/etcd"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/imagestream"
	imagestreametcd "github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/imagestream/etcd"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registryhostname"

	_ "github.com/openshift/openshift-apiserver/pkg/api/install"
)

var testDefaultRegistry = func(_ context.Context) (string, bool) { return "defaultregistry:5000", true }

type fakeSubjectAccessReviewRegistry struct {
}

func (f *fakeSubjectAccessReviewRegistry) Create(_ context.Context, subjectAccessReview *authorizationapi.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationapi.SubjectAccessReview, error) {
	return nil, nil
}

func (f *fakeSubjectAccessReviewRegistry) CreateContext(ctx context.Context, subjectAccessReview *authorizationapi.SubjectAccessReview) (*authorizationapi.SubjectAccessReview, error) {
	return nil, nil
}

type fakeUser struct {
}

var _ user.Info = &fakeUser{}

func (u *fakeUser) GetName() string {
	return "user"
}

func (u *fakeUser) GetUID() string {
	return "uid"
}

func (u *fakeUser) GetGroups() []string {
	return []string{"group1"}
}

func (u *fakeUser) GetExtra() map[string][]string {
	return map[string][]string{}
}

func setup(t *testing.T) (etcd.KV, *etcdtesting.EtcdTestServer, *REST) {
	server, etcdStorage := etcdtesting.NewUnsecuredEtcd3TestClientServer(t)
	etcdStorage.Codec = legacyscheme.Codecs.LegacyCodec(schema.GroupVersion{Group: "image.openshift.io", Version: "v1"})
	etcdClient := etcd.NewKV(server.V3Client)
	imagestreamRESTOptions := generic.RESTOptions{StorageConfig: etcdStorage, Decorator: generic.UndecoratedStorage, DeleteCollectionWorkers: 1, ResourcePrefix: "imagestreams"}
	rw := &fake.RegistryWhitelister{}

	imageRESTOptions := generic.RESTOptions{StorageConfig: etcdStorage, Decorator: generic.UndecoratedStorage, DeleteCollectionWorkers: 1, ResourcePrefix: "images"}
	imageStorage, err := imageetcd.NewREST(imageRESTOptions)
	if err != nil {
		t.Fatal(err)
	}
	registry := registryhostname.TestingRegistryHostnameRetriever(testDefaultRegistry, "", "")
	imageStreamStorage, _, imageStreamStatus, internalStorage, err := imagestreametcd.NewRESTWithLimitVerifier(
		imagestreamRESTOptions,
		registry,
		&fakeSubjectAccessReviewRegistry{},
		&admfake.ImageStreamLimitVerifier{},
		rw,
		imagestreametcd.NewEmptyLayerIndex(),
	)
	if err != nil {
		t.Fatal(err)
	}

	imageRegistry := image.NewRegistry(imageStorage)
	imageStreamRegistry := imagestream.NewRegistry(imageStreamStorage, imageStreamStatus, internalStorage)

	storage := NewREST(imageRegistry, imageStreamRegistry, rw)

	return etcdClient, server, storage
}

type statusError interface {
	Status() metav1.Status
}

func TestGetImageTag(t *testing.T) {
	tests := map[string]struct {
		image           *imageapi.Image
		repo            *imageapi.ImageStream
		tag             *imageapi.ImageTag
		expectError     bool
		errorTargetKind string
		errorTargetID   string
	}{
		"happy path": {
			image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
					Annotations: map[string]string{
						"test": "other",
					},
					Labels: map[string]string{
						"label": "to",
					},
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"latest": {
							Annotations: map[string]string{
								"color": "blue",
								"size":  "large",
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{
									Created:              metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
									DockerImageReference: "test",
									Image:                "10",
								},
							},
						},
					},
				},
			},
			tag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test:latest",
					Namespace:       "default",
					ResourceVersion: "3",
					Labels:          map[string]string{"label": "to"},
					Annotations:     map[string]string{"test": "other"},
				},
				Spec: &imageapi.TagReference{
					Name:            "latest",
					Annotations:     map[string]string{"color": "blue", "size": "large"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: "Source"},
				},
				Status: &imageapi.NamedTagEventList{
					Tag: "latest",
					Items: []imageapi.TagEvent{
						{
							Created:              metav1.Time{Time: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC).Local()},
							DockerImageReference: "test",
							Image:                "10",
						},
					},
				},
				Image: &imageapi.Image{
					ObjectMeta:                 metav1.ObjectMeta{Name: "10", ResourceVersion: "2"},
					DockerImageReference:       "test",
					DockerImageMetadataVersion: "1.0",
				},
			},
		},
		"no image": {
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
					Annotations: map[string]string{
						"test": "other",
					},
					Labels: map[string]string{
						"label": "to",
					},
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"latest": {
							Annotations: map[string]string{
								"color": "blue",
								"size":  "large",
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{
									Created:              metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
									DockerImageReference: "test",
									Image:                "10",
								},
							},
						},
					},
				},
			},
			tag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test:latest",
					Namespace:       "default",
					ResourceVersion: "2",
					Labels:          map[string]string{"label": "to"},
					Annotations:     map[string]string{"test": "other"},
				},
				Spec: &imageapi.TagReference{
					Name:            "latest",
					Annotations:     map[string]string{"color": "blue", "size": "large"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: "Source"},
				},
				Status: &imageapi.NamedTagEventList{
					Tag: "latest",
					Items: []imageapi.TagEvent{
						{
							Created:              metav1.Time{Time: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC).Local()},
							DockerImageReference: "test",
							Image:                "10",
						},
					},
				},
			},
		},
		"no spec": {
			image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
					Annotations: map[string]string{
						"test": "other",
					},
					Labels: map[string]string{
						"label": "to",
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{
									Created:              metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
									DockerImageReference: "test",
									Image:                "10",
								},
							},
						},
					},
				},
			},
			tag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test:latest",
					Namespace:       "default",
					ResourceVersion: "3",
					Labels:          map[string]string{"label": "to"},
					Annotations:     map[string]string{"test": "other"},
				},
				Status: &imageapi.NamedTagEventList{
					Tag: "latest",
					Items: []imageapi.TagEvent{
						{
							Created:              metav1.Time{Time: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC).Local()},
							DockerImageReference: "test",
							Image:                "10",
						},
					},
				},
				Image: &imageapi.Image{
					ObjectMeta:                 metav1.ObjectMeta{Name: "10", ResourceVersion: "2"},
					DockerImageReference:       "test",
					DockerImageMetadataVersion: "1.0",
				},
			},
		},
		"no status": {
			image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
					Annotations: map[string]string{
						"test": "other",
					},
					Labels: map[string]string{
						"label": "to",
					},
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"latest": {
							Annotations: map[string]string{
								"color": "blue",
								"size":  "large",
							},
						},
					},
				},
			},
			tag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test:latest",
					Namespace:       "default",
					ResourceVersion: "3",
					Labels:          map[string]string{"label": "to"},
					Annotations:     map[string]string{"test": "other"},
				},
				Spec: &imageapi.TagReference{
					Name:            "latest",
					Annotations:     map[string]string{"color": "blue", "size": "large"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: "Source"},
				},
			},
		},
		"missing repo": {
			expectError:     true,
			errorTargetKind: "imagestreams",
			errorTargetID:   "test",
		},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			client, server, storage := setup(t)
			defer server.Terminate(t)

			if testCase.image != nil {
				client.Put(
					context.TODO(),
					etcdtesting.AddPrefix("/images/"+testCase.image.Name),
					runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion), testCase.image),
				)
			}
			if testCase.repo != nil {
				client.Put(
					context.TODO(),
					etcdtesting.AddPrefix("/imagestreams/default/test"),
					runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion), testCase.repo),
				)
			}

			obj, err := storage.Get(apirequest.NewDefaultContext(), "test:latest", &metav1.GetOptions{})
			gotErr := err != nil
			if e, a := testCase.expectError, gotErr; e != a {
				t.Errorf("%s: Expected err=%v: got %v: %v", name, e, a, err)
				return
			}

			if testCase.expectError {
				if !errors.IsNotFound(err) {
					t.Errorf("%s: unexpected error type: %v", name, err)
					return
				}
				status := err.(statusError).Status()
				if status.Details.Kind != testCase.errorTargetKind || status.Details.Name != testCase.errorTargetID {
					t.Errorf("%s: unexpected status: %#v", name, status.Details)
					return
				}
				return
			}

			if !reflect.DeepEqual(testCase.tag, obj) {
				t.Errorf("%s", diff.ObjectReflectDiff(testCase.tag, obj))
			}
		})
	}
}

func TestListImageTag(t *testing.T) {
	tests := map[string]struct {
		images          []imageapi.Image
		repos           []imageapi.ImageStream
		tags            []imageapi.ImageTag
		expectError     bool
		errorTargetKind string
		errorTargetID   string
	}{
		"happy path": {
			images: []imageapi.Image{{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"}},
			repos: []imageapi.ImageStream{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							"test": "other",
						},
						Labels: map[string]string{
							"label": "to",
						},
					},
					Spec: imageapi.ImageStreamSpec{
						Tags: map[string]imageapi.TagReference{
							"latest": {
								Annotations: map[string]string{
									"color": "blue",
									"size":  "large",
								},
							},
						},
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										Created:              metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
										DockerImageReference: "test",
										Image:                "10",
									},
								},
							},
						},
					},
				},
			},
			tags: []imageapi.ImageTag{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test:latest",
						Namespace:       "default",
						ResourceVersion: "3",
						Labels:          map[string]string{"label": "to"},
						Annotations:     map[string]string{"test": "other"},
					},
					Spec: &imageapi.TagReference{
						Name:            "latest",
						Annotations:     map[string]string{"color": "blue", "size": "large"},
						ReferencePolicy: imageapi.TagReferencePolicy{Type: "Source"},
					},
					Status: &imageapi.NamedTagEventList{
						Tag: "latest",
						Items: []imageapi.TagEvent{
							{
								Created:              metav1.Time{Time: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC).Local()},
								DockerImageReference: "test",
								Image:                "10",
							},
						},
					},
					// Images are not loaded
					// Image: &imageapi.Image{
					// 	ObjectMeta:                 metav1.ObjectMeta{Name: "10", ResourceVersion: "2"},
					// 	DockerImageReference:       "test",
					// 	DockerImageMetadataVersion: "1.0",
					// },
				},
			},
		},
		"no image": {
			repos: []imageapi.ImageStream{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							"test": "other",
						},
						Labels: map[string]string{
							"label": "to",
						},
					},
					Spec: imageapi.ImageStreamSpec{
						Tags: map[string]imageapi.TagReference{
							"latest": {
								Annotations: map[string]string{
									"color": "blue",
									"size":  "large",
								},
							},
						},
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										Created:              metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
										DockerImageReference: "test",
										Image:                "10",
									},
								},
							},
						},
					},
				},
			},
			tags: []imageapi.ImageTag{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test:latest",
						Namespace:       "default",
						ResourceVersion: "2",
						Labels:          map[string]string{"label": "to"},
						Annotations:     map[string]string{"test": "other"},
					},
					Spec: &imageapi.TagReference{
						Name:            "latest",
						Annotations:     map[string]string{"color": "blue", "size": "large"},
						ReferencePolicy: imageapi.TagReferencePolicy{Type: "Source"},
					},
					Status: &imageapi.NamedTagEventList{
						Tag: "latest",
						Items: []imageapi.TagEvent{
							{
								Created:              metav1.Time{Time: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC).Local()},
								DockerImageReference: "test",
								Image:                "10",
							},
						},
					},
				},
			},
		},
		"no spec": {
			images: []imageapi.Image{{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"}},
			repos: []imageapi.ImageStream{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							"test": "other",
						},
						Labels: map[string]string{
							"label": "to",
						},
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										Created:              metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
										DockerImageReference: "test",
										Image:                "10",
									},
								},
							},
							"other": {
								Items: []imageapi.TagEvent{
									{
										Created:              metav1.Date(2015, 4, 24, 9, 38, 0, 0, time.UTC),
										DockerImageReference: "test2",
										Image:                "11",
									},
								},
							},
						},
					},
				},
			},
			tags: []imageapi.ImageTag{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test:latest",
						Namespace:       "default",
						ResourceVersion: "3",
						Labels:          map[string]string{"label": "to"},
						Annotations:     map[string]string{"test": "other"},
					},
					Status: &imageapi.NamedTagEventList{
						Tag: "latest",
						Items: []imageapi.TagEvent{
							{
								Created:              metav1.Time{Time: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC).Local()},
								DockerImageReference: "test",
								Image:                "10",
							},
						},
					},
					// Images are not loaded
					// Image: &imageapi.Image{
					// 	ObjectMeta:                 metav1.ObjectMeta{Name: "10", ResourceVersion: "2"},
					// 	DockerImageReference:       "test",
					// 	DockerImageMetadataVersion: "1.0",
					// },
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test:other",
						Namespace:       "default",
						ResourceVersion: "3",
						Labels:          map[string]string{"label": "to"},
						Annotations:     map[string]string{"test": "other"},
					},
					Status: &imageapi.NamedTagEventList{
						Tag: "other",
						Items: []imageapi.TagEvent{
							{
								Created:              metav1.Time{Time: metav1.Date(2015, 4, 24, 9, 38, 0, 0, time.UTC).Local()},
								DockerImageReference: "test2",
								Image:                "11",
							},
						},
					},
					// Images are not loaded
					// Image: &imageapi.Image{
					// 	ObjectMeta:                 metav1.ObjectMeta{Name: "10", ResourceVersion: "2"},
					// 	DockerImageReference:       "test",
					// 	DockerImageMetadataVersion: "1.0",
					// },
				},
			},
		},
		"no status": {
			images: []imageapi.Image{{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"}},
			repos: []imageapi.ImageStream{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							"test": "other",
						},
						Labels: map[string]string{
							"label": "to",
						},
					},
					Spec: imageapi.ImageStreamSpec{
						Tags: map[string]imageapi.TagReference{
							"latest": {
								Annotations: map[string]string{
									"color": "blue",
									"size":  "large",
								},
							},
						},
					},
				},
			},
			tags: []imageapi.ImageTag{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test:latest",
						Namespace:       "default",
						ResourceVersion: "3",
						Labels:          map[string]string{"label": "to"},
						Annotations:     map[string]string{"test": "other"},
					},
					Spec: &imageapi.TagReference{
						Name:            "latest",
						Annotations:     map[string]string{"color": "blue", "size": "large"},
						ReferencePolicy: imageapi.TagReferencePolicy{Type: "Source"},
					},
				},
			},
		},
		"no repo": {},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			client, server, storage := setup(t)
			defer server.Terminate(t)

			for _, image := range testCase.images {
				client.Put(
					context.TODO(),
					etcdtesting.AddPrefix("/images/"+image.Name),
					runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion), &image),
				)
			}
			for _, repo := range testCase.repos {
				client.Put(
					context.TODO(),
					etcdtesting.AddPrefix("/imagestreams/default/"+repo.Name),
					runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion), &repo),
				)
			}

			obj, err := storage.List(apirequest.NewDefaultContext(), &metainternalversion.ListOptions{})
			gotErr := err != nil
			if e, a := testCase.expectError, gotErr; e != a {
				t.Fatalf("Expected err=%v: got %v: %v", e, a, err)
			}

			if testCase.expectError {
				if !errors.IsNotFound(err) {
					t.Fatalf("unexpected error type: %v", err)
				}
				status := err.(statusError).Status()
				if status.Details.Kind != testCase.errorTargetKind || status.Details.Name != testCase.errorTargetID {
					t.Fatalf("unexpected status: %#v", status.Details)
				}
				return
			}

			tags := obj.(*imageapi.ImageTagList).Items
			if !reflect.DeepEqual(testCase.tags, tags) {
				t.Errorf("%s", diff.ObjectReflectDiff(testCase.tags, tags))
			}
		})
	}
}

func TestGetImageTagDIR(t *testing.T) {
	expDockerImageReference := "foo/bar/baz:latest"
	image := &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz:different"}
	repo := &imageapi.ImageStream{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Status: imageapi.ImageStreamStatus{
			Tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							Created:              metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
							DockerImageReference: expDockerImageReference,
							Image:                "10",
						},
					},
				},
			},
		},
	}

	client, server, storage := setup(t)
	defer server.Terminate(t)
	client.Put(
		context.TODO(),
		etcdtesting.AddPrefix("/images/"+image.Name),
		runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion), image),
	)
	client.Put(
		context.TODO(),
		etcdtesting.AddPrefix("/imagestreams/default/test"),
		runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion), repo),
	)
	obj, err := storage.Get(apirequest.NewDefaultContext(), "test:latest", &metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	actual := obj.(*imageapi.ImageTag)
	if actual.Image.DockerImageReference != expDockerImageReference {
		t.Errorf("Different DockerImageReference: expected %s, got %s", expDockerImageReference, actual.Image.DockerImageReference)
	}
}

func TestDeleteImageTag(t *testing.T) {
	tests := map[string]struct {
		repo        *imageapi.ImageStream
		expectError bool
	}{
		"repo not found": {
			expectError: true,
		},
		"nil tag map": {
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
			},
			expectError: true,
		},
		"missing tag": {
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"other": {
							From: &kapi.ObjectReference{
								Kind: "ImageStreamTag",
								Name: "test:foo",
							},
						},
					},
				},
			},
			expectError: true,
		},
		"happy path": {
			repo: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "default",
					Name:       "test",
					Generation: 2,
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"another": {
							From: &kapi.ObjectReference{
								Kind: "ImageStreamTag",
								Name: "test:foo",
							},
						},
						"latest": {
							From: &kapi.ObjectReference{
								Kind: "ImageStreamTag",
								Name: "test:bar",
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{
					DockerImageRepository: "registry.default.local/default/test",
					Tags: map[string]imageapi.TagEventList{
						"another": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: "registry.default.local/default/test@sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
									Image:                "sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
									Generation:           2,
								},
							},
						},
						"foo": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: "registry.default.local/default/test@sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
									Image:                "sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
									Generation:           2,
								},
							},
						},
						"latest": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: "registry.default.local/default/test@sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
									Image:                "sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
									Generation:           2,
								},
							},
						},
						"bar": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: "registry.default.local/default/test@sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
									Image:                "sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
									Generation:           2,
								},
							},
						},
					},
				},
			},
		},
	}

	for name, testCase := range tests {
		func() {
			client, server, storage := setup(t)
			defer server.Terminate(t)

			if testCase.repo != nil {
				client.Put(
					context.TODO(),
					etcdtesting.AddPrefix("/imagestreams/default/test"),
					runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion), testCase.repo),
				)
			}

			ctx := apirequest.WithUser(apirequest.NewDefaultContext(), &fakeUser{})
			obj, _, err := storage.Delete(ctx, "test:latest", nil, nil)
			gotError := err != nil
			if e, a := testCase.expectError, gotError; e != a {
				t.Fatalf("%s: expectError=%t, gotError=%t: %s", name, e, a, err)
			}
			if testCase.expectError {
				return
			}

			if obj == nil {
				t.Fatalf("%s: unexpected nil response", name)
			}
			expectedStatus := &metav1.Status{Status: metav1.StatusSuccess}
			if e, a := expectedStatus, obj; !reflect.DeepEqual(e, a) {
				t.Errorf("%s:\nexpect=%#v\nactual=%#v", name, e, a)
			}

			updatedRepo, err := storage.imageStreamRegistry.GetImageStream(apirequest.NewDefaultContext(), "test", &metav1.GetOptions{})
			if err != nil {
				t.Fatalf("%s: error retrieving updated repo: %s", name, err)
			}
			three := int64(3)
			expectedStreamSpec := map[string]imageapi.TagReference{
				"another": {
					Name: "another",
					From: &kapi.ObjectReference{
						Kind: "ImageStreamTag",
						Name: "test:foo",
					},
					Generation: &three,
					ReferencePolicy: imageapi.TagReferencePolicy{
						Type: imageapi.SourceTagReferencePolicy,
					},
				},
			}
			expectedStreamStatus := map[string]imageapi.TagEventList{
				"another": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "registry.default.local/default/test@sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
							Image:                "sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
							Generation:           2,
						},
					},
				},
				"foo": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "registry.default.local/default/test@sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
							Image:                "sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
							Generation:           2,
						},
					},
				},
				"bar": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "registry.default.local/default/test@sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
							Image:                "sha256:381151ac5b7f775e8371e489f3479b84a4c004c90ceddb2ad80b6877215a892f",
							Generation:           2,
						},
					},
				},
			}

			if updatedRepo.Generation != 3 {
				t.Errorf("%s: unexpected generation: %d", name, updatedRepo.Generation)
			}
			if e, a := expectedStreamStatus, updatedRepo.Status.Tags; !reflect.DeepEqual(e, a) {
				t.Errorf("%s: stream spec:\nexpect=%#v\nactual=%#v", name, e, a)
			}
			if e, a := expectedStreamSpec, updatedRepo.Spec.Tags; !reflect.DeepEqual(e, a) {
				t.Errorf("%s: stream spec:\nexpect=%#v\nactual=%#v", name, e, a)
			}
		}()
	}
}

func int64p(i int64) *int64 {
	return &i
}

func TestCreateImageTag(t *testing.T) {
	existing := &imageapi.ImageStream{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
			Namespace:         "default",
			Name:              "test",
			Labels:            map[string]string{"a": "b"},
			Annotations:       map[string]string{"blue": "green"},
		},
	}
	tests := map[string]struct {
		imagestream     *imageapi.ImageStream
		itag            runtime.Object
		expectError     bool
		errorTargetKind string
		errorTargetID   string
		expectTag       *imageapi.ImageTag
	}{
		"valid itag": {
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expectTag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "test:tag",
					Generation:  1,
					Labels:      map[string]string{"a": "b"},
					Annotations: map[string]string{"blue": "green"},
				},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					Generation:      int64p(1),
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
		},
		"mismatched itag name": {
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					Name:            "other",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"invalid spec": {
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec:  &imageapi.TagReference{},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"nil spec": {
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"valid spec with new image stream": {
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expectTag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "default",
					Name:       "test:tag",
					Generation: 1,
				},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					Generation:      int64p(1),
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, server, storage := setup(t)
			defer server.Terminate(t)

			if tc.imagestream != nil {
				if _, err := client.Put(
					context.TODO(),
					etcdtesting.AddPrefix("/imagestreams/default/test"),
					runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion),
						tc.imagestream,
					)); err != nil {
					t.Fatal(err)
				}
			}

			ctx := apirequest.WithUser(apirequest.NewDefaultContext(), &fakeUser{})
			obj, err := storage.Create(ctx, tc.itag, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			gotErr := err != nil
			if e, a := tc.expectError, gotErr; e != a {
				t.Fatalf("Expected err=%v: got %v: %v", e, a, err)
			}
			if tc.expectError {
				status := err.(statusError).Status()
				if status.Details.Kind != tc.errorTargetKind || status.Details.Name != tc.errorTargetID {
					t.Fatalf("unexpected status: %#v", status.Details)
				}
				return
			}
			tag := obj.(*imageapi.ImageTag)
			if tc.expectTag != nil && tag != nil {
				tc.expectTag.CreationTimestamp = tag.CreationTimestamp
				tc.expectTag.UID = tag.UID
				tc.expectTag.ResourceVersion = tag.ResourceVersion
			}
			if !reflect.DeepEqual(tc.expectTag, tag) {
				t.Fatalf("%s", diff.ObjectReflectDiff(tc.expectTag, tag))
			}
		})
	}
}

func TestUpdateImageTag(t *testing.T) {
	int64p := func(i int64) *int64 {
		return &i
	}
	existing := &imageapi.ImageStream{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
			Namespace:         "default",
			Name:              "test",
			Labels:            map[string]string{"a": "b"},
			Annotations:       map[string]string{"blue": "green"},
		},
		Status: imageapi.ImageStreamStatus{
			Tags: map[string]imageapi.TagEventList{
				"tag": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "initial/image:1",
							Image:                "sha256:0000000001",
							Generation:           2,
						},
					},
				},
			},
		},
	}
	tests := map[string]struct {
		updateName      string
		imagestream     *imageapi.ImageStream
		itag            runtime.Object
		expectError     bool
		errorTargetKind string
		errorTargetID   string
		expectTag       *imageapi.ImageTag
		expectCreated   bool
	}{
		"valid itag": {
			updateName:  "test:tag",
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "test:tag",
					Labels:      map[string]string{"a": "b"},
					Annotations: map[string]string{"blue": "green"},
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
				Status: &imageapi.NamedTagEventList{
					Tag: "tag",
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "initial/image:1",
							Image:                "sha256:0000000001",
							Generation:           2,
						},
					},
				},
			},
			expectCreated: true,
			expectTag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "test:tag",
					Generation:  1,
					Labels:      map[string]string{"a": "b"},
					Annotations: map[string]string{"blue": "green"},
				},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					Generation:      int64p(1),
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
				Status: &imageapi.NamedTagEventList{
					Tag: "tag",
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "initial/image:1",
							Image:                "sha256:0000000001",
							Generation:           2,
						},
					},
				},
			},
		},
		"requires status to be unchanged": {
			updateName:  "test:tag",
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "test:tag",
					Labels:      map[string]string{"a": "b"},
					Annotations: map[string]string{"blue": "green"},
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
				Status: &imageapi.NamedTagEventList{
					Tag: "tag",
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "initial/image:1",
							Image:                "sha256:0000000001",
							//Generation:           2,
						},
					},
				},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"no-op update": {
			updateName: "test:tag",
			imagestream: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
					Namespace:         "default",
					Name:              "test",
					Labels:            map[string]string{"a": "b"},
					Annotations:       map[string]string{"blue": "green"},
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"tag": {
							Name:            "tag",
							From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
							ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
						},
					},
				},
			},
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "test:tag",
					Labels:      map[string]string{"a": "b"},
					Annotations: map[string]string{"blue": "green"},
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expectTag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "test:tag",
					Generation:  1,
					Labels:      map[string]string{"a": "b"},
					Annotations: map[string]string{"blue": "green"},
				},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					Generation:      int64p(1),
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
		},
		"tag annotation update": {
			updateName: "test:tag",
			imagestream: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
					Namespace:         "default",
					Name:              "test",
					Labels:            map[string]string{"a": "b"},
					Annotations:       map[string]string{"blue": "green"},
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"tag": {
							Name:            "tag",
							From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
							ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
						},
					},
				},
			},
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "test:tag",
					Labels:      map[string]string{"a": "b"},
					Annotations: map[string]string{"blue": "green"},
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
					Annotations:     map[string]string{"a": "c"},
				},
			},
			expectTag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "test:tag",
					Generation:  1,
					Labels:      map[string]string{"a": "b"},
					Annotations: map[string]string{"blue": "green"},
				},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					Generation:      int64p(1),
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
					Annotations:     map[string]string{"a": "c"},
				},
			},
		},
		"tag name update fails": {
			updateName: "test:tag",
			imagestream: &imageapi.ImageStream{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Date(2015, 3, 24, 9, 38, 0, 0, time.UTC),
					Namespace:         "default",
					Name:              "test",
					Labels:            map[string]string{"a": "b"},
					Annotations:       map[string]string{"blue": "green"},
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"tag": {
							Name:            "tag",
							From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
							ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
						},
					},
				},
			},
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "test:tag",
					Labels:      map[string]string{"a": "b"},
					Annotations: map[string]string{"blue": "green"},
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					Name:            "other",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
					Annotations:     map[string]string{"a": "c"},
				},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"attempted metadata update": {
			updateName:  "test:tag",
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"mismatched itag name": {
			updateName:  "test:tag",
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					Name:            "other",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"empty itag name": {
			updateName:  "test:tag",
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec: &imageapi.TagReference{
					Name:            "",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"invalid spec": {
			updateName:  "test:tag",
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
				Spec:  &imageapi.TagReference{},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"nil spec": {
			updateName:  "test:tag",
			imagestream: existing,
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Image: &imageapi.Image{ObjectMeta: metav1.ObjectMeta{Name: "10"}, DockerImageReference: "foo/bar/baz"},
			},
			expectError:     true,
			errorTargetKind: "ImageTag",
			errorTargetID:   "test:tag",
		},
		"valid spec with new image stream": {
			updateName: "test:tag",
			itag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test:tag",
				},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			expectCreated: true,
			expectTag: &imageapi.ImageTag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "default",
					Name:       "test:tag",
					Generation: 1,
				},
				Spec: &imageapi.TagReference{
					Name:            "tag",
					Generation:      int64p(1),
					From:            &kapi.ObjectReference{Kind: "DockerImage", Name: "foo/bar/baz"},
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client, server, storage := setup(t)
			defer server.Terminate(t)

			if tc.imagestream != nil {
				if _, err := client.Put(
					context.TODO(),
					etcdtesting.AddPrefix("/imagestreams/default/test"),
					runtime.EncodeOrDie(legacyscheme.Codecs.LegacyCodec(imagev1.SchemeGroupVersion),
						tc.imagestream,
					)); err != nil {
					t.Fatal(err)
				}
			}

			ctx := apirequest.WithUser(apirequest.NewDefaultContext(), &fakeUser{})
			obj, created, err := storage.Update(ctx, tc.updateName, rest.DefaultUpdatedObjectInfo(tc.itag), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
			if created != tc.expectCreated {
				t.Errorf("Unexpected create %t", created)
			}
			gotErr := err != nil
			if e, a := tc.expectError, gotErr; e != a {
				t.Fatalf("Expected err=%v: got %v: %v", e, a, err)
			}
			if tc.expectError {
				status := err.(statusError).Status()
				if status.Details.Kind != tc.errorTargetKind || status.Details.Name != tc.errorTargetID {
					t.Fatalf("unexpected status: %#v", status.Details)
				}
				return
			}
			tag := obj.(*imageapi.ImageTag)
			if tc.expectTag != nil && tag != nil {
				tc.expectTag.CreationTimestamp = tag.CreationTimestamp
				tc.expectTag.UID = tag.UID
				tc.expectTag.ResourceVersion = tag.ResourceVersion
			}
			if !reflect.DeepEqual(tc.expectTag, tag) {
				t.Fatalf("%s", diff.ObjectReflectDiff(tc.expectTag, tag))
			}
		})
	}
}
