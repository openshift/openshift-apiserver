package etcd

import (
	"context"
	"testing"

	authorizationapi "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistrytest "k8s.io/apiserver/pkg/registry/generic/testing"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage/storagebackend"

	etcdtesting "k8s.io/apiserver/pkg/storage/etcd3/testing"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	kapihelper "k8s.io/kubernetes/pkg/apis/core/helper"

	imagev1 "github.com/openshift/api/image/v1"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation/fake"
	admfake "github.com/openshift/openshift-apiserver/pkg/image/apiserver/admission/fake"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registryhostname"

	// install all APIs
	_ "github.com/openshift/openshift-apiserver/pkg/api/install"
)

const (
	name = "foo"
)

var (
	testDefaultRegistry = func(_ context.Context) (string, bool) { return "test", true }
	noDefaultRegistry   = func(_ context.Context) (string, bool) { return "", false }
)

type fakeSubjectAccessReviewRegistry struct {
	err              error
	allow            bool
	request          *authorizationapi.SubjectAccessReview
	requestNamespace string
}

func (f *fakeSubjectAccessReviewRegistry) Create(_ context.Context, subjectAccessReview *authorizationapi.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationapi.SubjectAccessReview, error) {
	f.request = subjectAccessReview
	f.requestNamespace = subjectAccessReview.Spec.ResourceAttributes.Namespace
	return &authorizationapi.SubjectAccessReview{
		Status: authorizationapi.SubjectAccessReviewStatus{
			Allowed: f.allow,
		},
	}, f.err
}

func (f *fakeSubjectAccessReviewRegistry) CreateContext(ctx context.Context, subjectAccessReview *authorizationapi.SubjectAccessReview) (*authorizationapi.SubjectAccessReview, error) {
	return f.Create(ctx, subjectAccessReview, metav1.CreateOptions{})
}

func newStorage(t *testing.T) (*REST, *LayersREST, *InternalREST, *MockImageLayerIndex) {
	server, etcdStorage := etcdtesting.NewUnsecuredEtcd3TestClientServer(t)
	t.Cleanup(func() {
		server.Terminate(t)
	})

	etcdStorage.Codec = legacyscheme.Codecs.LegacyCodec(schema.GroupVersion{
		Group:   "image.openshift.io",
		Version: "v1",
	})
	imagestreamRESTOptions := generic.RESTOptions{
		StorageConfig: &storagebackend.ConfigForResource{
			Config: *etcdStorage,
			GroupResource: schema.GroupResource{
				Group:    "image.openshift.io",
				Resource: "imagestreams",
			},
		},
		Decorator:               generic.UndecoratedStorage,
		DeleteCollectionWorkers: 1,
		ResourcePrefix:          "imagestreams",
	}
	registry := registryhostname.TestingRegistryHostnameRetriever(noDefaultRegistry, "", "")
	imageIndex := NewMockImageLayerIndex()
	imageStorage, layersStorage, _, internalStorage, err := NewRESTWithLimitVerifier(
		imagestreamRESTOptions,
		registry,
		&fakeSubjectAccessReviewRegistry{},
		&admfake.ImageStreamLimitVerifier{},
		&fake.RegistryWhitelister{},
		imageIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	return imageStorage, layersStorage, internalStorage, imageIndex
}

func validImageStream() *imageapi.ImageStream {
	return &imageapi.ImageStream{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func create(t *testing.T, storage *InternalREST, obj *imageapi.ImageStream) *imageapi.ImageStream {
	ctx := apirequest.WithUser(apirequest.NewDefaultContext(), &fakeUser{})
	newObj, err := storage.Create(ctx, obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	return newObj.(*imageapi.ImageStream)
}

func TestCreate(t *testing.T) {
	storage, _, internalStorage, _ := newStorage(t)
	defer storage.Store.DestroyFunc()

	// TODO switch to upstream testing suite, when there will be possibility
	// to inject context with user, needed for these tests
	create(t, internalStorage, validImageStream())
}

func TestList(t *testing.T) {
	storage, _, _, _ := newStorage(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)
	test.TestList(
		validImageStream(),
	)
}

func TestGetImageStreamError(t *testing.T) {
	storage, _, _, _ := newStorage(t)
	defer storage.Store.DestroyFunc()

	image, err := storage.Get(apirequest.NewDefaultContext(), "image1", &metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		t.Errorf("Expected not-found error, got %v", err)
	}
	if image != nil {
		t.Errorf("Unexpected non-nil image stream: %#v", image)
	}
}

func TestGetImageStreamOK(t *testing.T) {
	storage, _, internalStorage, _ := newStorage(t)
	defer storage.Store.DestroyFunc()

	image := create(t, internalStorage, validImageStream())

	obj, err := storage.Get(apirequest.NewDefaultContext(), name, &metav1.GetOptions{})
	if err != nil {
		t.Errorf("Unexpected error: %#v", err)
	}
	if obj == nil {
		t.Fatalf("Unexpected nil stream")
	}
	got := obj.(*imageapi.ImageStream)
	got.ResourceVersion = image.ResourceVersion
	if !kapihelper.Semantic.DeepEqual(image, got) {
		t.Errorf("Expected %#v, got %#v", image, got)
	}
}

func TestGetLayersOK(t *testing.T) {
	storage, layersStorage, internalStorage, imageIndex := newStorage(t)
	defer storage.Store.DestroyFunc()

	manifestListDigest := "sha256:ad9bd57a3a57cc95515c537b89aaa69d83a6df54c4050fcf2b41ad367bec0cd5"
	amd64Digest := "sha256:bbb248c803ff97f51db3b37a2a604a6270cd2ee1ca9266120aeccb3b19ce80d2"

	imageIndex.Add(&imagev1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: manifestListDigest,
		},
		DockerImageReference:         "test/image@" + manifestListDigest,
		DockerImageManifestMediaType: "application/vnd.docker.distribution.manifest.list.v2+json",
		DockerImageManifests: []imagev1.ImageManifest{
			{
				MediaType:    "application/vnd.docker.distribution.manifest.v2+json",
				Digest:       amd64Digest,
				Architecture: "amd64",
				OS:           "linux",
			},
		},
	})
	imageIndex.Add(&imagev1.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: amd64Digest,
		},
		DockerImageReference:         "test/image@" + amd64Digest,
		DockerImageManifestMediaType: "application/vnd.docker.distribution.manifest.v2+json",
		DockerImageLayers: []imagev1.ImageLayer{
			{
				Name:      "sha256:729ce43e2c915c3463b620f3fba201a4a641ca5a282387e233db799208342a08",
				LayerSize: 772986,
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			},
		},
		DockerImageMetadata: runtime.RawExtension{
			Raw: []byte(`{"ID":"sha256:2bd29714875d9206777f9e8876033cbcd58edd14f2c0f1203435296b3f31c5f7"}`),
		},
	})

	create(
		t,
		internalStorage,
		&imageapi.ImageStream{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Status: imageapi.ImageStreamStatus{
				Tags: map[string]imageapi.TagEventList{
					"latest": {
						Items: []imageapi.TagEvent{
							{
								DockerImageReference: "test/image@" + manifestListDigest,
								Image:                manifestListDigest,
							},
						},
					},
				},
			},
		},
	)

	obj, err := layersStorage.Get(apirequest.NewDefaultContext(), name, &metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Unexpected error: %#v", err)
	}
	if obj == nil {
		t.Fatalf("Unexpected nil layers")
	}
	imageStreamLayers := obj.(*imageapi.ImageStreamLayers)

	configDigest := "sha256:2bd29714875d9206777f9e8876033cbcd58edd14f2c0f1203435296b3f31c5f7"
	expectedImages := map[string]imageapi.ImageBlobReferences{
		manifestListDigest: {
			Manifests: []string{amd64Digest},
		},
		amd64Digest: {
			Layers: []string{"sha256:729ce43e2c915c3463b620f3fba201a4a641ca5a282387e233db799208342a08"},
			Config: &configDigest,
		},
	}
	for image, got := range imageStreamLayers.Images {
		if expected, ok := expectedImages[image]; ok {
			if !kapihelper.Semantic.DeepEqual(got, expected) {
				t.Errorf("ImageBlobReferences for %s: got %#+v, expected %#+v", image, got, expected)
			}
			delete(expectedImages, image)
		} else {
			t.Errorf("Unexpected image in ImageStreamLayers: %s", image)
		}
	}
	if len(expectedImages) > 0 {
		t.Errorf("Missing images in ImageStreamLayers: %v", expectedImages)
	}
}

type fakeUser struct{}

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
