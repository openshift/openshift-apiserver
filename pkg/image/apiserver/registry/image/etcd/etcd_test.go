package etcd

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistrytest "k8s.io/apiserver/pkg/registry/generic/testing"
	"k8s.io/apiserver/pkg/registry/rest"
	etcdtesting "k8s.io/apiserver/pkg/storage/etcd3/testing"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/kubernetes/pkg/api/legacyscheme"

	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/internal/testutil"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/image"

	// install all APIs
	_ "github.com/openshift/openshift-apiserver/pkg/api/install"
)

func newStorage(t *testing.T) (*REST, *etcdtesting.EtcdTestServer) {
	server, etcdStorage := etcdtesting.NewUnsecuredEtcd3TestClientServer(t)
	etcdStorage.Codec = legacyscheme.Codecs.LegacyCodec(schema.GroupVersion{Group: "image.openshift.io", Version: "v1"})
	etcdStorageConfigForImages := &storagebackend.ConfigForResource{Config: *etcdStorage, GroupResource: schema.GroupResource{Group: "image.openshift.io", Resource: "images"}}
	imageRESTOptions := generic.RESTOptions{StorageConfig: etcdStorageConfigForImages, Decorator: generic.UndecoratedStorage, DeleteCollectionWorkers: 1, ResourcePrefix: "images"}
	storage, err := NewREST(imageRESTOptions)
	if err != nil {
		t.Fatal(err)
	}
	return storage, server
}

func TestStorage(t *testing.T) {
	storage, _ := newStorage(t)
	image.NewRegistry(storage)
}

func TestCreate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()
	valid := validImage()
	valid.Name = ""
	valid.GenerateName = "test-"
	test.TestCreate(
		valid,
		// invalid
		&imageapi.Image{},
	)
}

func TestUpdate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()
	test.TestUpdate(
		validImage(),
		// updateFunc
		func(obj runtime.Object) runtime.Object {
			object := obj.(*imageapi.Image)
			object.DockerImageReference = "openshift/origin"
			return object
		},
		// invalid updateFunc
		func(obj runtime.Object) runtime.Object {
			object := obj.(*imageapi.Image)
			object.DockerImageReference = "\\"
			return object
		},
	)
}

func TestList(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()
	test.TestList(
		validImage(),
	)
}

func TestGet(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()
	test.TestGet(
		validImage(),
	)
}

func TestDelete(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()
	image := validImage()
	image.ObjectMeta = metav1.ObjectMeta{GenerateName: "foo"}
	test.TestDelete(
		validImage(),
	)
}

func TestWatch(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store)

	valid := validImage()
	valid.Name = "foo"
	valid.Labels = map[string]string{"foo": "bar"}

	test.TestWatch(
		valid,
		// matching labels
		[]labels.Set{{"foo": "bar"}},
		// not matching labels
		[]labels.Set{{"foo": "baz"}},
		// matching fields
		[]fields.Set{
			{"metadata.name": "foo"},
		},
		// not matchin fields
		[]fields.Set{
			{"metadata.name": "bar"},
		},
	)
}

func TestCreateSetsMetadata(t *testing.T) {
	testCases := []struct {
		name   string
		image  *imageapi.Image
		expect func(*imageapi.Image) bool
	}{
		{
			name: "image schema v2",
			expect: func(image *imageapi.Image) bool {
				ok := true
				if image.DockerImageMetadata.Size != 451763572 {
					t.Errorf("image had size %d", image.DockerImageMetadata.Size)
					ok = false
				}
				if len(image.DockerImageLayers) != 2 || image.DockerImageLayers[0].Name != "sha256:dc42dfa52495c90dc5b99c19534d6d4fa9cd37fa439356fcbd73e770c35f2293" || image.DockerImageLayers[0].LayerSize != 132852852 {
					t.Errorf("unexpected layers: %#v", image.DockerImageLayers)
					ok = false
				}
				return ok
			},
			image: testutil.MustKindestCompleteImage(),
		},
	}

	for i, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			storage, server := newStorage(t)
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			obj, err := storage.Create(apirequest.NewDefaultContext(), test.image, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if obj == nil {
				t.Errorf("%d: Expected nil obj, got %v", i, obj)
				return
			}
			if err != nil {
				t.Errorf("%d: Unexpected non-nil error: %#v", i, err)
				return
			}
			image, ok := obj.(*imageapi.Image)
			if !ok {
				t.Errorf("%d: Expected image type, got: %#v", i, obj)
				return
			}
			if test.expect != nil && !test.expect(image) {
				t.Errorf("%d: Unexpected image: %#v", i, obj)
			}
		})
	}
}

func TestUpdateResetsMetadata(t *testing.T) {
	testCases := []struct {
		name     string
		image    *imageapi.Image
		existing *imageapi.Image
		expect   func(*imageapi.Image) bool
	}{
		{
			name: "labels and Docker image reference updated",
			expect: func(image *imageapi.Image) bool {
				ok := true
				if image.Labels["a"] != "b" {
					t.Errorf("unexpected labels: %s", image.Labels)
					ok = false
				}
				if image.DockerImageMetadata.ID != testutil.KindestConfigDigest {
					t.Errorf("unexpected container image: %#v", image.DockerImageMetadata)
					ok = false
				}
				if image.DockerImageReference != "kindest/node-updated" {
					t.Errorf("image reference not changed: %s", image.DockerImageReference)
					ok = false
				}
				if image.DockerImageMetadata.Size != 451763572 {
					t.Errorf("image had size %d", image.DockerImageMetadata.Size)
					ok = false
				}
				if len(image.DockerImageLayers) != 2 || image.DockerImageLayers[0].LayerSize != 132852852 {
					t.Errorf("unexpected layers: %#v", image.DockerImageLayers)
					ok = false
				}
				return ok
			},
			existing: testutil.MustKindestCompleteImage(),
			image: testutil.MustKindestCompleteImage(func(img *imageapi.Image) {
				img.Labels = map[string]string{"a": "b"}
				img.DockerImageReference = "kindest/node-updated"
			}),
		},
		{
			name: "manifest is preserved and unpacked",
			expect: func(image *imageapi.Image) bool {
				if len(image.DockerImageManifest) != 0 {
					t.Errorf("unexpected not empty manifest")
					return false
				}
				if image.DockerImageMetadata.ID != testutil.KindestConfigDigest {
					t.Errorf("unexpected container image: %#v", image.DockerImageMetadata)
					return false
				}
				if image.DockerImageReference != "kindest/node-updated" {
					t.Errorf("image reference not changed: %s", image.DockerImageReference)
					return false
				}
				if image.DockerImageMetadata.Size != 451763572 {
					t.Errorf("image had size %d", image.DockerImageMetadata.Size)
					return false
				}
				if len(image.DockerImageLayers) != 2 || image.DockerImageLayers[0].Name != "sha256:dc42dfa52495c90dc5b99c19534d6d4fa9cd37fa439356fcbd73e770c35f2293" || image.DockerImageLayers[0].LayerSize != 132852852 {
					t.Errorf("unexpected layers: %#v", image.DockerImageLayers)
					return false
				}
				return true
			},
			existing: testutil.MustKindestCompleteImage(),
			image: &imageapi.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:         testutil.KindestConfigDigest,
					GenerateName: "kindest",
				},
				DockerImageReference: "kindest/node-updated",
			},
		},
	}

	for i, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			storage, server := newStorage(t)
			defer server.Terminate(t)
			defer storage.Store.DestroyFunc()

			// Clear the resource version before creating
			test.existing.ResourceVersion = ""
			created, err := storage.Create(apirequest.NewDefaultContext(), test.existing, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
			if err != nil {
				t.Errorf("%d: Unexpected non-nil error: %#v", i, err)
				return
			}

			// Copy the resource version into our update object
			test.image.ResourceVersion = created.(*imageapi.Image).ResourceVersion
			obj, _, err := storage.Update(apirequest.NewDefaultContext(), test.image.Name, rest.DefaultUpdatedObjectInfo(test.image), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
			if err != nil {
				t.Errorf("%d: Unexpected non-nil error: %#v", i, err)
				return
			}
			if obj == nil {
				t.Errorf("%d: Expected nil obj, got %v", i, obj)
				return
			}
			image, ok := obj.(*imageapi.Image)
			if !ok {
				t.Errorf("%d: Expected image type, got: %#v", i, obj)
				return
			}
			if test.expect != nil && !test.expect(image) {
				t.Errorf("%d: Unexpected image: %#v", i, obj)
			}
		})
	}
}

func validImage() *imageapi.Image {
	return testutil.KindestBareImage()
}
