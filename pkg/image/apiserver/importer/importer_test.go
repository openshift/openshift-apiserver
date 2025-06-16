package importer

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/containers/image/v5/pkg/sysregistriesv2"
	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/reference"
	godigest "github.com/opencontainers/go-digest"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	kapi "k8s.io/kubernetes/pkg/apis/core"

	userapi "github.com/openshift/api/user/v1"
	imageref "github.com/openshift/library-go/pkg/image/reference"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
)

func init() {
	runtime.Must(userapi.Install(legacyscheme.Scheme))
}

type mockRetrieverFunc func(registry *url.URL, repoName string, insecure bool) (distribution.Repository, error)

func (r mockRetrieverFunc) Repository(ctx context.Context, ref imageref.DockerImageReference, insecure bool) (distribution.Repository, error) {
	return r(ref.RegistryURL(), ref.RepositoryName(), insecure)
}

type mockRetriever struct {
	repo     distribution.Repository
	insecure bool
	err      error
}

func (r *mockRetriever) Repository(ctx context.Context, ref imageref.DockerImageReference, insecure bool) (distribution.Repository, error) {
	r.insecure = insecure
	return r.repo, r.err
}

type mockRepository struct {
	repoErr, getErr, getByTagErr, getTagErr, tagErr, untagErr, allTagErr, err error

	blobs *mockBlobStore

	manifest       distribution.Manifest
	manifestReqs   []godigest.Digest
	extraManifests map[godigest.Digest]distribution.Manifest
	tags           map[string]string
}

func (r *mockRepository) Name() string { return "test" }
func (r *mockRepository) Named() reference.Named {
	named, _ := reference.WithName("test")
	return named
}

func (r *mockRepository) Manifests(ctx context.Context, options ...distribution.ManifestServiceOption) (distribution.ManifestService, error) {
	return r, r.repoErr
}
func (r *mockRepository) Blobs(ctx context.Context) distribution.BlobStore { return r.blobs }
func (r *mockRepository) Exists(ctx context.Context, dgst godigest.Digest) (bool, error) {
	return false, r.getErr
}

func (r *mockRepository) Get(ctx context.Context, dgst godigest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	r.manifestReqs = append(r.manifestReqs, dgst)
	for d, manifest := range r.extraManifests {
		if dgst == d {
			return manifest, nil
		}
	}
	for _, option := range options {
		if _, ok := option.(distribution.WithTagOption); ok {
			return r.manifest, r.getByTagErr
		}
	}
	return r.manifest, r.getErr
}

func (r *mockRepository) Delete(ctx context.Context, dgst godigest.Digest) error {
	return fmt.Errorf("not implemented")
}

func (r *mockRepository) Put(ctx context.Context, manifest distribution.Manifest, options ...distribution.ManifestServiceOption) (godigest.Digest, error) {
	return "", fmt.Errorf("not implemented")
}

func (r *mockRepository) Tags(ctx context.Context) distribution.TagService {
	return &mockTagService{repo: r}
}

type mockBlobStore struct {
	distribution.BlobStore

	blobs map[godigest.Digest][]byte

	statErr, serveErr, openErr error
}

func (r *mockBlobStore) Stat(ctx context.Context, dgst godigest.Digest) (distribution.Descriptor, error) {
	return distribution.Descriptor{}, r.statErr
}

func (r *mockBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, req *http.Request, dgst godigest.Digest) error {
	return r.serveErr
}

func (r *mockBlobStore) Open(ctx context.Context, dgst godigest.Digest) (distribution.ReadSeekCloser, error) {
	return nil, r.openErr
}

func (r *mockBlobStore) Get(ctx context.Context, dgst godigest.Digest) ([]byte, error) {
	b, exists := r.blobs[dgst]
	if !exists {
		return nil, distribution.ErrBlobUnknown
	}
	return b, nil
}

type mockTagService struct {
	distribution.TagService

	repo *mockRepository
}

func (r *mockTagService) Get(ctx context.Context, tag string) (distribution.Descriptor, error) {
	v, ok := r.repo.tags[tag]
	if !ok {
		return distribution.Descriptor{}, r.repo.getTagErr
	}
	dgst, err := godigest.Parse(v)
	if err != nil {
		panic(err)
	}
	return distribution.Descriptor{Digest: dgst}, r.repo.getTagErr
}

func (r *mockTagService) Tag(ctx context.Context, tag string, desc distribution.Descriptor) error {
	r.repo.tags[tag] = desc.Digest.String()
	return r.repo.tagErr
}

func (r *mockTagService) Untag(ctx context.Context, tag string) error {
	if _, ok := r.repo.tags[tag]; ok {
		delete(r.repo.tags, tag)
	}
	return r.repo.untagErr
}

func (r *mockTagService) All(ctx context.Context) (res []string, err error) {
	err = r.repo.allTagErr
	for tag := range r.repo.tags {
		res = append(res, tag)
	}
	return
}

func (r *mockTagService) Lookup(ctx context.Context, digest distribution.Descriptor) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestImportNothing(t *testing.T) {
	ctx := NewStaticCredentialsContext(
		http.DefaultTransport, http.DefaultTransport, nil,
	)
	isi := &imageapi.ImageStreamImport{}
	i := NewImageStreamImporter(ctx, nil, 5, nil, nil)
	if err := i.Import(nil, isi, nil); err != nil {
		t.Fatal(err)
	}
}

func expectStatusError(status metav1.Status, message string) bool {
	if status.Status != metav1.StatusFailure || status.Message != message {
		return false
	}
	return true
}

func TestImport(t *testing.T) {
	busyboxManifestSchema2 := &schema2.DeserializedManifest{}
	if err := busyboxManifestSchema2.UnmarshalJSON([]byte(busyboxManifest)); err != nil {
		t.Fatal(err)
	}
	busyboxConfigDigest := godigest.FromBytes([]byte(busyboxManifestConfig))
	busyboxManifestSchema2.Config = distribution.Descriptor{
		Digest:    busyboxConfigDigest,
		Size:      int64(len(busyboxManifestConfig)),
		MediaType: schema2.MediaTypeImageConfig,
	}
	t.Logf("busybox manifest schema 2 digest: %q", godigest.FromBytes([]byte(busyboxManifest)))

	insecureRetriever := &mockRetriever{
		repo: &mockRepository{
			getTagErr:   fmt.Errorf("no such tag"),
			getByTagErr: fmt.Errorf("no such manifest tag"),
			getErr:      fmt.Errorf("no such digest"),
		},
	}
	testCases := []struct {
		name      string
		retriever RepositoryRetriever
		isi       imageapi.ImageStreamImport
		expect    func(*imageapi.ImageStreamImport, *testing.T)
	}{
		{
			name:      "insecure import policy",
			retriever: insecureRetriever,
			isi: imageapi.ImageStreamImport{
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{From: kapi.ObjectReference{Kind: "DockerImage", Name: "test"}, ImportPolicy: imageapi.TagImportPolicy{Insecure: true}},
					},
				},
			},
			expect: func(isi *imageapi.ImageStreamImport, t *testing.T) {
				if !insecureRetriever.insecure {
					t.Errorf("expected retriever to beset insecure: %#v", insecureRetriever)
				}
			},
		},
		{
			name: "missing tag, digest, and invalid image reference",
			retriever: &mockRetriever{
				repo: &mockRepository{
					getTagErr:   fmt.Errorf("no such tag"),
					getByTagErr: fmt.Errorf("no such manifest tag"),
					getErr:      fmt.Errorf("no such digest"),
				},
			},
			isi: imageapi.ImageStreamImport{
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{From: kapi.ObjectReference{Kind: "DockerImage", Name: "test"}},
						{From: kapi.ObjectReference{Kind: "DockerImage", Name: "test@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}},
						{From: kapi.ObjectReference{Kind: "DockerImage", Name: "test///un/parse/able/image"}},
						{From: kapi.ObjectReference{Kind: "ImageStreamTag", Name: "test:other"}},
					},
				},
			},
			expect: func(isi *imageapi.ImageStreamImport, t *testing.T) {
				if !expectStatusError(isi.Status.Images[0].Status, "Internal error occurred: docker.io/library/test:latest: no such manifest tag") {
					t.Errorf("unexpected status: %#v", isi.Status.Images[0].Status)
				}
				if !expectStatusError(isi.Status.Images[1].Status, "Internal error occurred: docker.io/library/test@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855: no such digest") {
					t.Errorf("unexpected status: %#v", isi.Status.Images[1].Status)
				}
				if !expectStatusError(isi.Status.Images[2].Status, ` "" is invalid: from.name: Invalid value: "test///un/parse/able/image": invalid name: invalid reference format`) {
					t.Errorf("unexpected status: %s", isi.Status.Images[2].Status.Message)
				}
				// non DockerImage refs are no-ops
				if status := isi.Status.Images[3].Status; status.Status != "" {
					t.Errorf("unexpected status: %#v", isi.Status.Images[3].Status)
				}
				expectedTags := []string{"latest", "", "", ""}
				for i, image := range isi.Status.Images {
					if image.Tag != expectedTags[i] {
						t.Errorf("unexpected tag of status %d (%s != %s)", i, image.Tag, expectedTags[i])
					}
				}
			},
		},
		{
			name:      "failed repository import",
			retriever: &mockRetriever{err: fmt.Errorf("error")},
			isi: imageapi.ImageStreamImport{
				Spec: imageapi.ImageStreamImportSpec{
					Repository: &imageapi.RepositoryImportSpec{
						From: kapi.ObjectReference{Kind: "DockerImage", Name: "test"},
					},
				},
			},
			expect: func(isi *imageapi.ImageStreamImport, t *testing.T) {
				if !reflect.DeepEqual(isi.Status.Repository.AdditionalTags, []string(nil)) {
					t.Errorf("unexpected additional tags: %#v", isi.Status.Repository)
				}
				if len(isi.Status.Repository.Images) != 0 {
					t.Errorf("unexpected number of images: %#v", isi.Status.Repository.Images)
				}
				if isi.Status.Repository.Status.Status != metav1.StatusFailure || isi.Status.Repository.Status.Message != "Internal error occurred: test: error" {
					t.Errorf("unexpected status: %#v", isi.Status.Repository.Status)
				}
			},
		},
		{
			name: "successful import by tag and digest",
			retriever: &mockRetriever{
				repo: &mockRepository{
					blobs: &mockBlobStore{
						blobs: map[godigest.Digest][]byte{
							busyboxConfigDigest: []byte(busyboxManifestConfig),
						},
					},
					manifest: busyboxManifestSchema2,
				},
			},
			isi: imageapi.ImageStreamImport{
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{From: kapi.ObjectReference{Kind: "DockerImage", Name: "test@" + busyboxDigest}},
						{From: kapi.ObjectReference{Kind: "DockerImage", Name: "test:tag"}},
					},
				},
			},
			expect: func(isi *imageapi.ImageStreamImport, t *testing.T) {
				if len(isi.Status.Images) != 2 {
					t.Errorf("unexpected number of images: %#v", isi.Status.Repository.Images)
				}
				expectedTags := []string{"", "tag"}
				for i, image := range isi.Status.Images {
					if image.Status.Status != metav1.StatusSuccess {
						t.Errorf("unexpected status %d: %#v", i, image.Status)
					}
					if image.Image == nil {
						t.Errorf("unexpected nil image: %#v", image)
					} else {
						// the image name is always the sha256, and size is calculated
						if image.Image.Name != busyboxDigest || image.Image.DockerImageMetadata.Size != 669049 {
							t.Errorf("unexpected image %d: %#v (size %d)", i, image.Image.Name, image.Image.DockerImageMetadata.Size)
						}
						// the most specific reference is returned
						if image.Image.DockerImageReference != "test@"+busyboxDigest {
							t.Errorf("unexpected ref %d: %#v", i, image.Image.DockerImageReference)
						}
					}
					if image.Tag != expectedTags[i] {
						t.Errorf("unexpected tag of status %d (%s != %s)", i, image.Tag, expectedTags[i])
					}
				}
			},
		},
		{
			name: "successful import by tag",
			retriever: &mockRetriever{
				repo: &mockRepository{
					blobs: &mockBlobStore{
						blobs: map[godigest.Digest][]byte{
							busyboxConfigDigest: []byte(busyboxManifestConfig),
						},
					},
					manifest: busyboxManifestSchema2,
				},
			},
			isi: imageapi.ImageStreamImport{
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{From: kapi.ObjectReference{Kind: "DockerImage", Name: "test:busybox"}},
					},
				},
			},
			expect: func(isi *imageapi.ImageStreamImport, t *testing.T) {
				if len(isi.Status.Images) != 1 {
					t.Errorf("unexpected number of images: %#v", isi.Status.Repository.Images)
				}
				image := isi.Status.Images[0]
				if image.Status.Status != metav1.StatusSuccess {
					t.Errorf("unexpected status: %#v", image.Status)
				}
				// the image name is always the sha256, and size is calculated
				if image.Image.Name != busyboxDigest {
					t.Errorf("unexpected image: %q != %q", image.Image.Name, busyboxDigest)
				}
				if image.Image.DockerImageMetadata.Size != busyboxImageSize {
					t.Errorf("unexpected image size: %d != %d", image.Image.DockerImageMetadata.Size, busyboxImageSize)
				}
				// the most specific reference is returned
				if image.Image.DockerImageReference != "test@"+busyboxDigest {
					t.Errorf("unexpected ref: %#v", image.Image.DockerImageReference)
				}
				if image.Tag != "busybox" {
					t.Errorf("unexpected tag of status: %s != busybox", image.Tag)
				}
			},
		},
		{
			name: "import repository with additional tags",
			retriever: &mockRetriever{
				repo: &mockRepository{
					manifest: busyboxManifestSchema2,
					tags: map[string]string{
						"v1":    busyboxDigest,
						"other": busyboxDigest,
						"v2":    busyboxDigest,
						"3":     busyboxDigest,
						"3.1":   busyboxDigest,
						"abc":   busyboxDigest,
					},
					getTagErr:   fmt.Errorf("no such tag"),
					getByTagErr: fmt.Errorf("no such manifest tag"),
				},
			},
			isi: imageapi.ImageStreamImport{
				Spec: imageapi.ImageStreamImportSpec{
					Repository: &imageapi.RepositoryImportSpec{
						From: kapi.ObjectReference{Kind: "DockerImage", Name: "test"},
					},
				},
			},
			expect: func(isi *imageapi.ImageStreamImport, t *testing.T) {
				if !reflect.DeepEqual(isi.Status.Repository.AdditionalTags, []string{"v2"}) {
					t.Errorf("unexpected additional tags: %#v", isi.Status.Repository)
				}
				if len(isi.Status.Repository.Images) != 5 {
					t.Errorf("unexpected number of images: %#v", isi.Status.Repository.Images)
				}
				expectedTags := []string{"3.1", "3", "abc", "other", "v1"}
				for i, image := range isi.Status.Repository.Images {
					if image.Status.Status != metav1.StatusFailure || image.Status.Message != "Internal error occurred: docker.io/library/test:"+image.Tag+": no such manifest tag" {
						t.Errorf("unexpected status %d: %#v", i, isi.Status.Repository.Images)
					}
					if image.Tag != expectedTags[i] {
						t.Errorf("unexpected tag of status %d (%s != %s)", i, image.Tag, expectedTags[i])
					}
				}
			},
		},
	}
	for i, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			im := NewImageStreamImporter(test.retriever, nil, 5, nil, nil)
			if err := im.Import(nil, &test.isi, &imageapi.ImageStream{}); err != nil {
				t.Errorf("%d: %v", i, err)
			}
			if test.expect != nil {
				test.expect(&test.isi, t)
			}
		})
	}
}

func TestImportFromMirror(t *testing.T) {
	busyboxManifestSchema2 := &schema2.DeserializedManifest{}
	if err := busyboxManifestSchema2.UnmarshalJSON([]byte(busyboxManifest)); err != nil {
		t.Fatal(err)
	}
	busyboxConfigDigest := godigest.FromBytes([]byte(busyboxManifestConfig))
	busyboxManifestSchema2.Config = distribution.Descriptor{
		Digest:    busyboxConfigDigest,
		Size:      int64(len(busyboxManifestConfig)),
		MediaType: schema2.MediaTypeImageConfig,
	}
	t.Logf("busybox manifest schema 2 digest: %q", godigest.FromBytes([]byte(busyboxManifest)))

	regConf := &sysregistriesv2.V2RegistriesConf{
		Registries: []sysregistriesv2.Registry{
			{
				Prefix: "quay.io/openshift",
				Endpoint: sysregistriesv2.Endpoint{
					Location: "quay.io/openshift",
				},
				Mirrors: []sysregistriesv2.Endpoint{
					{
						Location: "mirror.example.com/openshift4",
					},
				},
				MirrorByDigestOnly: true,
			},
		},
	}

	t.Run("by digest from mirrored repo", func(t *testing.T) {
		testRetriever := mockRetrieverFunc(func(registry *url.URL, repoName string, insecure bool) (distribution.Repository, error) {
			if registry.String() == "https://mirror.example.com" && repoName == "openshift4/test" && !insecure {
				return &mockRepository{
					blobs: &mockBlobStore{
						blobs: map[godigest.Digest][]byte{
							busyboxConfigDigest: []byte(busyboxManifestConfig),
						},
					},
					manifest: busyboxManifestSchema2,
				}, nil
			}
			err := fmt.Errorf("unexpected call to the repository retriever: %v %v %v", registry, repoName, insecure)
			t.Error(err)
			return nil, err
		})

		isi := imageapi.ImageStreamImport{
			Spec: imageapi.ImageStreamImportSpec{
				Images: []imageapi.ImageImportSpec{
					{
						From: kapi.ObjectReference{Kind: "DockerImage", Name: "quay.io/openshift/test@" + busyboxDigest},
					},
				},
			},
		}

		im := NewImageStreamImporter(testRetriever, regConf, 5, nil, nil)
		if err := im.Import(nil, &isi, &imageapi.ImageStream{}); err != nil {
			t.Fatalf("%v", err)
		}
		if len(isi.Status.Images) != 1 {
			t.Fatalf("unexpected number of images: %#v", isi.Status.Repository.Images)
		}
		image := isi.Status.Images[0]
		if image.Status.Status != metav1.StatusSuccess {
			t.Fatalf("unexpected status: %#v", image.Status)
		}
		if image.Image.Name != busyboxDigest {
			t.Errorf("unexpected image: %q != %q", image.Image.Name, busyboxDigest)
		}
		if image.Image.DockerImageMetadata.Size != busyboxImageSize {
			t.Errorf("unexpected image size: %d != %d", image.Image.DockerImageMetadata.Size, busyboxImageSize)
		}
		if image.Image.DockerImageReference != "quay.io/openshift/test@"+busyboxDigest {
			t.Errorf("unexpected ref: %#v", image.Image.DockerImageReference)
		}
	})

	t.Run("by digest from another repo", func(t *testing.T) {
		testRetriever := mockRetrieverFunc(func(registry *url.URL, repoName string, insecure bool) (distribution.Repository, error) {
			if registry.String() == "https://registry-1.docker.io" && repoName == "openshift/test" && !insecure {
				return &mockRepository{
					blobs: &mockBlobStore{
						blobs: map[godigest.Digest][]byte{
							busyboxConfigDigest: []byte(busyboxManifestConfig),
						},
					},
					manifest: busyboxManifestSchema2,
				}, nil
			}
			err := fmt.Errorf("unexpected call to the repository retriever: %v %v %v", registry, repoName, insecure)
			t.Error(err)
			return nil, err
		})

		isi := imageapi.ImageStreamImport{
			Spec: imageapi.ImageStreamImportSpec{
				Images: []imageapi.ImageImportSpec{
					{
						From: kapi.ObjectReference{Kind: "DockerImage", Name: "docker.io/openshift/test@" + busyboxDigest},
					},
				},
			},
		}

		im := NewImageStreamImporter(testRetriever, regConf, 5, nil, nil)
		if err := im.Import(nil, &isi, &imageapi.ImageStream{}); err != nil {
			t.Fatalf("%v", err)
		}
		if len(isi.Status.Images) != 1 {
			t.Fatalf("unexpected number of images: %#v", isi.Status.Repository.Images)
		}
		image := isi.Status.Images[0]
		if image.Status.Status != metav1.StatusSuccess {
			t.Fatalf("unexpected status: %#v", image.Status)
		}
		if image.Image.Name != busyboxDigest {
			t.Errorf("unexpected image: %q != %q", image.Image.Name, busyboxDigest)
		}
		if image.Image.DockerImageMetadata.Size != busyboxImageSize {
			t.Errorf("unexpected image size: %d != %d", image.Image.DockerImageMetadata.Size, busyboxImageSize)
		}
		if image.Image.DockerImageReference != "docker.io/openshift/test@"+busyboxDigest {
			t.Errorf("unexpected ref: %#v", image.Image.DockerImageReference)
		}
	})

	regBlockedSourceConf := &sysregistriesv2.V2RegistriesConf{
		Registries: []sysregistriesv2.Registry{
			{
				Prefix: "quay.io",
				Endpoint: sysregistriesv2.Endpoint{
					Location: "quay.io",
				},
				Blocked: true,
				Mirrors: []sysregistriesv2.Endpoint{
					{
						Location:       "mirror.example.com",
						PullFromMirror: sysregistriesv2.MirrorByDigestOnly,
					},
				},
			},
		},
	}

	t.Run("source blocked pull by digest only from mirrored repo", func(t *testing.T) {
		testRetriever := mockRetrieverFunc(func(registry *url.URL, repoName string, insecure bool) (distribution.Repository, error) {
			if registry.String() == "https://mirror.example.com" && repoName == "openshift/test" && !insecure {
				return &mockRepository{
					blobs: &mockBlobStore{
						blobs: map[godigest.Digest][]byte{
							busyboxConfigDigest: []byte(busyboxManifestConfig),
						},
					},
					manifest: busyboxManifestSchema2,
				}, nil
			}
			err := fmt.Errorf("unexpected call to the repository retriever: %v %v %v", registry, repoName, insecure)
			t.Error(err)
			return nil, err
		})

		isi := imageapi.ImageStreamImport{
			Spec: imageapi.ImageStreamImportSpec{
				Images: []imageapi.ImageImportSpec{
					{
						From: kapi.ObjectReference{Kind: "DockerImage", Name: "quay.io/openshift/test@" + busyboxDigest},
					},
				},
			},
		}

		im := NewImageStreamImporter(testRetriever, regBlockedSourceConf, 5, nil, nil)
		if err := im.Import(nil, &isi, &imageapi.ImageStream{}); err != nil {
			t.Fatalf("%v", err)
		}
		if len(isi.Status.Images) != 1 {
			t.Fatalf("unexpected number of images: %#v", isi.Status.Repository.Images)
		}
		image := isi.Status.Images[0]
		if image.Status.Status != metav1.StatusSuccess {
			t.Fatalf("unexpected status: %#v", image.Status)
		}
		if image.Image.Name != busyboxDigest {
			t.Errorf("unexpected image: %q != %q", image.Image.Name, busyboxDigest)
		}
		if image.Image.DockerImageMetadata.Size != busyboxImageSize {
			t.Errorf("unexpected image size: %d != %d", image.Image.DockerImageMetadata.Size, busyboxImageSize)
		}
		if image.Image.DockerImageReference != "quay.io/openshift/test@"+busyboxDigest {
			t.Errorf("unexpected ref: %#v", image.Image.DockerImageReference)
		}
	})
}

const busyboxDigest = "sha256:a59906e33509d14c036c8678d687bd4eec81ed7c4b8ce907b888c607f6a1e0e6"

const busyboxManifest = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
   "config": {
      "mediaType": "application/octet-stream",
      "size": 1459,
      "digest": "sha256:2b8fd9751c4c0f5dd266fcae00707e67a2545ef34f9a29354585f93dac906749"
   },
   "layers": [
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "size": 667590,
         "digest": "sha256:8ddc19f16526912237dd8af81971d5e4dd0587907234be2b83e249518d5b673f"
      }
   ]
}`

const busyboxManifestConfig = `{"architecture":"amd64","config":{"Hostname":"55cd1f8f6e5b","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"],"Cmd":["sh"],"Image":"sha256:e732471cb81a564575aad46b9510161c5945deaf18e9be3db344333d72f0b4b2","Volumes":null,"WorkingDir":"","Entrypoint":null,"OnBuild":null,"Labels":{}},"container":"764ef4448baa9a1ce19e4ae95f8cdd4eda7a1186c512773e56dc634dff208a59","container_config":{"Hostname":"55cd1f8f6e5b","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"],"Cmd":["/bin/sh","-c","#(nop) CMD [\"sh\"]"],"Image":"sha256:e732471cb81a564575aad46b9510161c5945deaf18e9be3db344333d72f0b4b2","Volumes":null,"WorkingDir":"","Entrypoint":null,"OnBuild":null,"Labels":{}},"created":"2016-06-23T23:23:37.198943461Z","docker_version":"1.10.3","history":[{"created":"2016-06-23T23:23:36.73131105Z","created_by":"/bin/sh -c #(nop) ADD file:9ca60502d646bdd815bb51e612c458e2d447b597b95cf435f9673f0966d41c1a in /"},{"created":"2016-06-23T23:23:37.198943461Z","created_by":"/bin/sh -c #(nop) CMD [\"sh\"]","empty_layer":true}],"os":"linux","rootfs":{"type":"layers","diff_ids":["sha256:8ac8bfaff55af948c796026ee867448c5b5b5d9dd3549f4006d9759b25d4a893"]}}`

const busyboxImageSize int64 = int64(1459 + 667590)
