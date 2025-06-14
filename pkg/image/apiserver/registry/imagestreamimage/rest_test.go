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
			input: "repo@id",
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
								{Image: "id"},
							},
						},
					},
				},
			},
			expectedImageMetadataID: "2d24f826cb16146e2016ff349a8a33ed5830f3b938d45c0f82943f4ab8c097e7",
			expectedConfigHostname:  "43bd710ec89a",
			images: []*imageapi.Image{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "id",
					},
					DockerImageManifest: `{
   "name": "library/ubuntu",
   "tag": "latest",
   "architecture": "amd64",
   "fsLayers": [
      {
         "blobSum": "tarsum.dev+sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
      },
      {
         "blobSum": "tarsum.dev+sha256:b194de3772ebbcdc8f244f663669799ac1cb141834b7cb8b69100285d357a2b0"
      },
      {
         "blobSum": "tarsum.dev+sha256:c937c4bb1c1a21cc6d94340812262c6472092028972ae69b551b1a70d4276171"
      },
      {
         "blobSum": "tarsum.dev+sha256:2aaacc362ac6be2b9e9ae8c6029f6f616bb50aec63746521858e47841b90fabd"
      },
      {
         "blobSum": "tarsum.dev+sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
      }
   ],
   "history": [
      {
         "v1Compatibility": "{\"id\":\"2d24f826cb16146e2016ff349a8a33ed5830f3b938d45c0f82943f4ab8c097e7\",\"parent\":\"117ee323aaa9d1b136ea55e4421f4ce413dfc6c0cc6b2186dea6c88d93e1ad7c\",\"created\":\"2015-02-21T02:11:06.735146646Z\",\"container\":\"c9a3eda5951d28aa8dbe5933be94c523790721e4f80886d0a8e7a710132a38ec\",\"container_config\":{\"Hostname\":\"43bd710ec89a\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"],\"Cmd\":[\"/bin/sh\",\"-c\",\"#(nop) CMD [/bin/bash]\"],\"Image\":\"117ee323aaa9d1b136ea55e4421f4ce413dfc6c0cc6b2186dea6c88d93e1ad7c\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":[]},\"docker_version\":\"1.4.1\",\"config\":{\"Hostname\":\"43bd710ec89a\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"],\"Cmd\":[\"/bin/bash\"],\"Image\":\"117ee323aaa9d1b136ea55e4421f4ce413dfc6c0cc6b2186dea6c88d93e1ad7c\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":[]},\"architecture\":\"amd64\",\"os\":\"linux\",\"checksum\":\"tarsum.dev+sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\",\"Size\":0}\n"
      },
      {
         "v1Compatibility": "{\"id\":\"117ee323aaa9d1b136ea55e4421f4ce413dfc6c0cc6b2186dea6c88d93e1ad7c\",\"parent\":\"1c8294cc516082dfbb731f062806b76b82679ce38864dd87635f08869c993e45\",\"created\":\"2015-02-21T02:11:02.473113442Z\",\"container\":\"b4d4c42c196081cdb8e032446e9db92c0ce8ddeeeb1ef4e582b0275ad62b9af2\",\"container_config\":{\"Hostname\":\"43bd710ec89a\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"],\"Cmd\":[\"/bin/sh\",\"-c\",\"sed -i 's/^#\\\\s*\\\\(deb.*universe\\\\)$/\\\\1/g' /etc/apt/sources.list\"],\"Image\":\"1c8294cc516082dfbb731f062806b76b82679ce38864dd87635f08869c993e45\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":[]},\"docker_version\":\"1.4.1\",\"config\":{\"Hostname\":\"43bd710ec89a\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"],\"Cmd\":null,\"Image\":\"1c8294cc516082dfbb731f062806b76b82679ce38864dd87635f08869c993e45\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":[]},\"architecture\":\"amd64\",\"os\":\"linux\",\"checksum\":\"tarsum.dev+sha256:b194de3772ebbcdc8f244f663669799ac1cb141834b7cb8b69100285d357a2b0\",\"Size\":1895}\n"
      },
      {
         "v1Compatibility": "{\"id\":\"1c8294cc516082dfbb731f062806b76b82679ce38864dd87635f08869c993e45\",\"parent\":\"fa4fd76b09ce9b87bfdc96515f9a5dd5121c01cc996cf5379050d8e13d4a864b\",\"created\":\"2015-02-21T02:10:56.648624065Z\",\"container\":\"d6323d4f92bf13dd2a7d2957e8970c8dc8ba3c2df08ffebdf4a04b4b658c83fb\",\"container_config\":{\"Hostname\":\"43bd710ec89a\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"],\"Cmd\":[\"/bin/sh\",\"-c\",\"echo '#!/bin/sh' \\u003e /usr/sbin/policy-rc.d \\u0009\\u0026\\u0026 echo 'exit 101' \\u003e\\u003e /usr/sbin/policy-rc.d \\u0009\\u0026\\u0026 chmod +x /usr/sbin/policy-rc.d \\u0009\\u0009\\u0026\\u0026 dpkg-divert --local --rename --add /sbin/initctl \\u0009\\u0026\\u0026 cp -a /usr/sbin/policy-rc.d /sbin/initctl \\u0009\\u0026\\u0026 sed -i 's/^exit.*/exit 0/' /sbin/initctl \\u0009\\u0009\\u0026\\u0026 echo 'force-unsafe-io' \\u003e /etc/dpkg/dpkg.cfg.d/docker-apt-speedup \\u0009\\u0009\\u0026\\u0026 echo 'DPkg::Post-Invoke { \\\"rm -f /var/cache/apt/archives/*.deb /var/cache/apt/archives/partial/*.deb /var/cache/apt/*.bin || true\\\"; };' \\u003e /etc/apt/apt.conf.d/docker-clean \\u0009\\u0026\\u0026 echo 'APT::Update::Post-Invoke { \\\"rm -f /var/cache/apt/archives/*.deb /var/cache/apt/archives/partial/*.deb /var/cache/apt/*.bin || true\\\"; };' \\u003e\\u003e /etc/apt/apt.conf.d/docker-clean \\u0009\\u0026\\u0026 echo 'Dir::Cache::pkgcache \\\"\\\"; Dir::Cache::srcpkgcache \\\"\\\";' \\u003e\\u003e /etc/apt/apt.conf.d/docker-clean \\u0009\\u0009\\u0026\\u0026 echo 'Acquire::Languages \\\"none\\\";' \\u003e /etc/apt/apt.conf.d/docker-no-languages \\u0009\\u0009\\u0026\\u0026 echo 'Acquire::GzipIndexes \\\"true\\\"; Acquire::CompressionTypes::Order:: \\\"gz\\\";' \\u003e /etc/apt/apt.conf.d/docker-gzip-indexes\"],\"Image\":\"fa4fd76b09ce9b87bfdc96515f9a5dd5121c01cc996cf5379050d8e13d4a864b\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":[]},\"docker_version\":\"1.4.1\",\"config\":{\"Hostname\":\"43bd710ec89a\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"],\"Cmd\":null,\"Image\":\"fa4fd76b09ce9b87bfdc96515f9a5dd5121c01cc996cf5379050d8e13d4a864b\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":[]},\"architecture\":\"amd64\",\"os\":\"linux\",\"checksum\":\"tarsum.dev+sha256:c937c4bb1c1a21cc6d94340812262c6472092028972ae69b551b1a70d4276171\",\"Size\":194533}\n"
      },
      {
         "v1Compatibility": "{\"id\":\"fa4fd76b09ce9b87bfdc96515f9a5dd5121c01cc996cf5379050d8e13d4a864b\",\"parent\":\"511136ea3c5a64f264b78b5433614aec563103b4d4702f3ba7d4d2698e22c158\",\"created\":\"2015-02-21T02:10:48.627646094Z\",\"container\":\"43bd710ec89aa1d61685c5ddb8737b315f2152d04c9e127ea2a9cf79f950fa5d\",\"container_config\":{\"Hostname\":\"43bd710ec89a\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"],\"Cmd\":[\"/bin/sh\",\"-c\",\"#(nop) ADD file:0018ff77d038472f52512fd66ae2466e6325e5d91289e10a6b324f40582298df in /\"],\"Image\":\"511136ea3c5a64f264b78b5433614aec563103b4d4702f3ba7d4d2698e22c158\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":[]},\"docker_version\":\"1.4.1\",\"config\":{\"Hostname\":\"43bd710ec89a\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"],\"Cmd\":null,\"Image\":\"511136ea3c5a64f264b78b5433614aec563103b4d4702f3ba7d4d2698e22c158\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":[]},\"architecture\":\"amd64\",\"os\":\"linux\",\"checksum\":\"tarsum.dev+sha256:2aaacc362ac6be2b9e9ae8c6029f6f616bb50aec63746521858e47841b90fabd\",\"Size\":188097705}\n"
      },
      {
         "v1Compatibility": "{\"id\":\"511136ea3c5a64f264b78b5433614aec563103b4d4702f3ba7d4d2698e22c158\",\"comment\":\"Imported from -\",\"created\":\"2013-06-13T14:03:50.821769-07:00\",\"container_config\":{\"Hostname\":\"\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":null,\"Image\":\"\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":null},\"docker_version\":\"0.4.0\",\"architecture\":\"x86_64\",\"checksum\":\"tarsum.dev+sha256:324d4cf44ee7daa46266c1df830c61a7df615c0632176a339e7310e34723d67a\",\"Size\":0}\n"
      }
   ],
   "schemaVersion": 1,
   "signatures": [
      {
         "header": {
            "jwk": {
               "crv": "P-256",
               "kid": "OIH7:HQFS:44FK:45VB:3B53:OIAG:TPL4:ATF5:6PNE:MGHN:NHQX:2GE4",
               "kty": "EC",
               "x": "Cu_UyxwLgHzE9rvlYSmvVdqYCXY42E9eNhBb0xNv0SQ",
               "y": "zUsjWJkeKQ5tv7S-hl1Tg71cd-CqnrtiiLxSi6N_yc8"
            },
            "alg": "ES256"
         },
         "signature": "JafVC022gLJbK8fEp9UrW-GWAi53nD4Xf0-fmb766nV6zGTq7g9RutSWHcpv3OqhV8t5b-4j-bXhvnGBjXA7Yg",
         "protected": "eyJmb3JtYXRMZW5ndGgiOjEwMDc2LCJmb3JtYXRUYWlsIjoiQ24wIiwidGltZSI6IjIwMTUtMDMtMDdUMTI6Mzk6MjJaIn0"
      }
   ]
}`,
				},
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
