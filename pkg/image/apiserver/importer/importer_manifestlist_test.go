package importer

import (
	_ "embed"
	"errors"
	"flag"
	"os"
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/schema2"
	godigest "github.com/opencontainers/go-digest"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"k8s.io/klog/v2"
	kapi "k8s.io/kubernetes/pkg/apis/core"
)

// manifestListJSON is the manifest list found at
// docker.io/library/ubuntu@sha256:d1d454df0f579c6be4d8161d227462d69e163a8ff9d20a847533989cf0c94d90
//
//go:embed testdata/manifest-list.json
var manifestListJSON []byte

//go:embed testdata/manifest-linux-amd64.json
var amd64ManifestJSON []byte

//go:embed testdata/config-linux-amd64.json
var amd64ConfigJSON []byte

//go:embed testdata/manifest-linux-arm64.json
var arm64ManifestJSON []byte

//go:embed testdata/config-linux-arm64.json
var arm64ConfigJSON []byte

func TestImportManifestListWithError(t *testing.T) {
	manifestList := &manifestlist.DeserializedManifestList{}
	if err := manifestList.UnmarshalJSON(manifestListJSON); err != nil {
		t.Fatal(err)
	}
	amd64Manifest := &schema2.DeserializedManifest{}
	if err := amd64Manifest.UnmarshalJSON(amd64ManifestJSON); err != nil {
		t.Fatal(err)
	}
	arm64Manifest := &schema2.DeserializedManifest{}
	if err := arm64Manifest.UnmarshalJSON(arm64ManifestJSON); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name      string
		fromName  string
		manifests map[godigest.Digest]distribution.Manifest
		blobs     map[godigest.Digest][]byte
		getErr    error
	}{
		{
			name:     "missingManifestErrorByDigest",
			fromName: "test@sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41",
			manifests: map[godigest.Digest]distribution.Manifest{
				"sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41": manifestList,
				"sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0": amd64Manifest,
			},
			blobs: map[godigest.Digest][]byte{
				"sha256:a2a15febcdf362f6115e801d37b5e60d6faaeedcb9896155e5fe9d754025be12": amd64ConfigJSON,
			},
			getErr: errors.New("the requested manifest does not exist in this fake registry"),
		},
		{
			name:     "missingConfigBlobErrorByDigest",
			fromName: "test@sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41",
			manifests: map[godigest.Digest]distribution.Manifest{
				"sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41": manifestList,
				"sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0": amd64Manifest,
				"sha256:1a06d68cb9117b52965035a5b0fa4c1470ef892e6062ffedb1af1922952e0950": arm64Manifest,
			},
			blobs: map[godigest.Digest][]byte{
				"sha256:a2a15febcdf362f6115e801d37b5e60d6faaeedcb9896155e5fe9d754025be12": amd64ConfigJSON,
			},
			getErr: errors.New("the requested blob does not exist in this fake registry"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockRepo := &mockRepository{
				extraManifests: testCase.manifests,
				blobs:          &mockBlobStore{blobs: testCase.blobs},
				getErr:         testCase.getErr,
			}
			retriever := &mockRetriever{repo: mockRepo}

			isi := imageapi.ImageStreamImport{
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							ImportPolicy: imageapi.TagImportPolicy{
								ImportMode: imageapi.ImportModePreserveOriginal,
							},
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: testCase.fromName,
							},
						},
					},
				},
			}

			im := NewImageStreamImporter(retriever, nil, 5, nil, nil)
			if err := im.Import(nil, &isi, &imageapi.ImageStream{}); err != nil {
				t.Errorf("importing manifest list returned: %v", err)
			}

			if isi.Status.Images[0].Status.Status != "Failure" {
				t.Errorf("invalid status for image import: Status=%q, Message=%q", isi.Status.Images[0].Status.Status, isi.Status.Images[0].Status.Message)
			}
		})
	}
}

func TestImportManifestList(t *testing.T) {
	manifestList := &manifestlist.DeserializedManifestList{}
	if err := manifestList.UnmarshalJSON(manifestListJSON); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name             string
		importMode       imageapi.ImportModeType
		importFromName   string
		manifestList     *manifestlist.DeserializedManifestList
		importEntireRepo bool
		// the sub manifests that will be imported, as listed in importPlatforms
		manifests []struct {
			raw              []byte
			digest           godigest.Digest
			configBlobDigest godigest.Digest
			rawConfigBlob    []byte
		}
		expectedRequests []godigest.Digest
	}{
		{
			name:             "ImportRepository",
			importMode:       imageapi.ImportModePreserveOriginal,
			importFromName:   "test",
			importEntireRepo: true,
			manifestList:     manifestList,
			manifests: []struct {
				raw              []byte
				digest           godigest.Digest
				configBlobDigest godigest.Digest
				rawConfigBlob    []byte
			}{
				{
					raw:              amd64ManifestJSON,
					digest:           "sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
					configBlobDigest: "sha256:a2a15febcdf362f6115e801d37b5e60d6faaeedcb9896155e5fe9d754025be12",
					rawConfigBlob:    amd64ConfigJSON,
				},
				{
					raw:              arm64ManifestJSON,
					digest:           "sha256:1a06d68cb9117b52965035a5b0fa4c1470ef892e6062ffedb1af1922952e0950",
					configBlobDigest: "sha256:eb8f2c2207058e4d8bb3afb85e959ff3f12d3481f3e38611de549a39935b28c4",
					rawConfigBlob:    arm64ConfigJSON,
				},
			},
			expectedRequests: []godigest.Digest{
				// manifest list digest will be empty when importing by tag
				"",
				// amd64 manifest digest
				"sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
				// arm64 manifest digest
				"sha256:1a06d68cb9117b52965035a5b0fa4c1470ef892e6062ffedb1af1922952e0950",
			},
		},
		{
			name:           "ImportByTag",
			importMode:     imageapi.ImportModePreserveOriginal,
			importFromName: "test:latest",
			manifestList:   manifestList,
			manifests: []struct {
				raw              []byte
				digest           godigest.Digest
				configBlobDigest godigest.Digest
				rawConfigBlob    []byte
			}{
				{
					raw:              amd64ManifestJSON,
					digest:           "sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
					configBlobDigest: "sha256:a2a15febcdf362f6115e801d37b5e60d6faaeedcb9896155e5fe9d754025be12",
					rawConfigBlob:    amd64ConfigJSON,
				},
				{
					raw:              arm64ManifestJSON,
					digest:           "sha256:1a06d68cb9117b52965035a5b0fa4c1470ef892e6062ffedb1af1922952e0950",
					configBlobDigest: "sha256:eb8f2c2207058e4d8bb3afb85e959ff3f12d3481f3e38611de549a39935b28c4",
					rawConfigBlob:    arm64ConfigJSON,
				},
			},
			expectedRequests: []godigest.Digest{
				// manifest list digest will be empty when importing by tag
				"",
				// amd64 manifest digest
				"sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
				// arm64 manifest digest
				"sha256:1a06d68cb9117b52965035a5b0fa4c1470ef892e6062ffedb1af1922952e0950",
			},
		},
		{
			name:           "ImportByDigest",
			importMode:     imageapi.ImportModePreserveOriginal,
			importFromName: "test@sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41",
			manifestList:   manifestList,
			manifests: []struct {
				raw              []byte
				digest           godigest.Digest
				configBlobDigest godigest.Digest
				rawConfigBlob    []byte
			}{
				{
					raw:              amd64ManifestJSON,
					digest:           "sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
					configBlobDigest: "sha256:a2a15febcdf362f6115e801d37b5e60d6faaeedcb9896155e5fe9d754025be12",
					rawConfigBlob:    amd64ConfigJSON,
				},
				{
					raw:              arm64ManifestJSON,
					digest:           "sha256:1a06d68cb9117b52965035a5b0fa4c1470ef892e6062ffedb1af1922952e0950",
					configBlobDigest: "sha256:eb8f2c2207058e4d8bb3afb85e959ff3f12d3481f3e38611de549a39935b28c4",
					rawConfigBlob:    arm64ConfigJSON,
				},
			},
			expectedRequests: []godigest.Digest{
				// manifest list digest
				"sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41",
				// amd64 manifest digest
				"sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
				// arm64 manifest digest
				"sha256:1a06d68cb9117b52965035a5b0fa4c1470ef892e6062ffedb1af1922952e0950",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			subManifests := map[godigest.Digest]distribution.Manifest{}
			configBlobs := map[godigest.Digest][]byte{}
			for _, manifest := range testCase.manifests {
				parsedManifest := &schema2.DeserializedManifest{}
				if err := parsedManifest.UnmarshalJSON(manifest.raw); err != nil {
					t.Fatal(err)
				}
				subManifests[manifest.digest] = parsedManifest
				configBlobs[manifest.configBlobDigest] = manifest.rawConfigBlob
			}

			mockRepo := &mockRepository{
				manifest:       testCase.manifestList,
				blobs:          &mockBlobStore{blobs: configBlobs},
				extraManifests: subManifests,
			}
			retriever := &mockRetriever{repo: mockRepo}

			imageStreamImport := imageapi.ImageStreamImport{Spec: imageapi.ImageStreamImportSpec{}}
			if testCase.importEntireRepo {
				mockRepo.tags = map[string]string{"latest": "foo-digest"}
				imageStreamImport.Spec.Repository = &imageapi.RepositoryImportSpec{
					ImportPolicy: imageapi.TagImportPolicy{
						ImportMode: testCase.importMode,
					},
					From: kapi.ObjectReference{
						Kind: "DockerImage",
						Name: testCase.importFromName,
					},
				}
			} else {
				imageStreamImport.Spec.Images = []imageapi.ImageImportSpec{
					{
						ImportPolicy: imageapi.TagImportPolicy{
							ImportMode: testCase.importMode,
						},
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: testCase.importFromName,
						},
					},
				}
			}

			im := NewImageStreamImporter(retriever, nil, 5, nil, nil)
			if err := im.Import(nil, &imageStreamImport, &imageapi.ImageStream{}); err != nil {
				t.Errorf("importing manifest list returned: %v", err)
			}

			if !reflect.DeepEqual(mockRepo.manifestReqs, testCase.expectedRequests) {
				t.Errorf("expected requests diverge from requests made:\nwant: %v\ngot:  %v",
					testCase.expectedRequests, mockRepo.manifestReqs)
			}
			if testCase.importEntireRepo {
				if imageStreamImport.Status.Repository.Images[0].Status.Status != "Success" {
					t.Errorf("invalid status for repository import: %+v", imageStreamImport.Status)
				}
				manifests := imageStreamImport.Status.Repository.Images[0].Manifests
				if len(manifests) != len(testCase.manifests) {
					t.Logf("want: %d", len(testCase.manifests))
					t.Logf("got:  %d", len(manifests))
					t.Fatal("failed to create image objects for sub manifests")
				}
			} else {
				if imageStreamImport.Status.Images[0].Status.Status != "Success" {
					t.Errorf("invalid status for image import: %+v", imageStreamImport.Status)
				}
				manifests := imageStreamImport.Status.Images[0].Manifests
				if len(manifests) != len(testCase.manifests) {
					t.Logf("want: %d", len(testCase.manifests))
					t.Logf("got:  %d", len(manifests))
					t.Fatal("failed to create image objects for sub manifests")
				}
			}
			// if manifests[0].Name != testCase.manifests[0].digest
		})
	}
}

func TestImportManifestListSingleManifest(t *testing.T) {
	testCases := []struct {
		name             string
		fromName         string
		importMode       imageapi.ImportModeType
		expectedRequests []godigest.Digest
	}{
		{
			name:       "testDigestImportModeEmptyStringDefaultsToSingleManifestMode",
			fromName:   "test@sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41",
			importMode: "",
			expectedRequests: []godigest.Digest{
				"sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41",
				"sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
			},
		},
		{
			name:       "testTagImportModeEmptyStringDefaultsToSingleManifestMode",
			fromName:   "test:latest",
			importMode: "",
			expectedRequests: []godigest.Digest{
				"", // when requesting by tag, the digest is empty
				"sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
			},
		},
		{
			name:       "testDigestImportModelSingleManifest",
			fromName:   "test@sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41",
			importMode: imageapi.ImportModeLegacy,
			expectedRequests: []godigest.Digest{
				"sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41",
				"sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
			},
		},
		{
			name:       "testTagImportModelSingleManifest",
			fromName:   "test:latest",
			importMode: imageapi.ImportModeLegacy,
			expectedRequests: []godigest.Digest{
				"", // when requesting by tag, the digest is empty
				"sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			manifestList := &manifestlist.DeserializedManifestList{}
			if err := manifestList.UnmarshalJSON(manifestListJSON); err != nil {
				t.Fatal(err)
			}
			innerManifest := &schema2.DeserializedManifest{}
			if err := innerManifest.UnmarshalJSON(amd64ManifestJSON); err != nil {
				t.Fatal(err)
			}

			mockRepo := &mockRepository{
				manifest: manifestList,
				blobs: &mockBlobStore{
					blobs: map[godigest.Digest][]byte{
						"sha256:a2a15febcdf362f6115e801d37b5e60d6faaeedcb9896155e5fe9d754025be12": amd64ConfigJSON,
					},
				},
				extraManifests: map[godigest.Digest]distribution.Manifest{
					manifestList.Manifests[0].Descriptor.Digest: innerManifest,
				},
			}
			retriever := &mockRetriever{repo: mockRepo}

			isi := imageapi.ImageStreamImport{
				Spec: imageapi.ImageStreamImportSpec{
					Images: []imageapi.ImageImportSpec{
						{
							ImportPolicy: imageapi.TagImportPolicy{
								ImportMode: testCase.importMode,
							},
							From: kapi.ObjectReference{
								Kind: "DockerImage",
								Name: testCase.fromName,
							},
						},
					},
				},
			}

			im := NewImageStreamImporter(retriever, nil, 5, nil, nil)
			if err := im.Import(nil, &isi, &imageapi.ImageStream{}); err != nil {
				t.Errorf("importing manifest list returned: %v", err)
			}

			if !reflect.DeepEqual(mockRepo.manifestReqs, testCase.expectedRequests) {
				t.Errorf("expected requests diverge from requests made: %v, %v", testCase.expectedRequests, mockRepo.manifestReqs)
			}
			if isi.Status.Images[0].Status.Status != "Success" {
				t.Errorf("invalid status for image import: %+v", isi.Status)
			}
		})
	}

}

// TestMain starting point for all tests.
// Surfaces klog flags by default to enable
// go test -v ./ --args <klog flags>
func TestMain(m *testing.M) {
	klog.InitFlags(flag.CommandLine)
	os.Exit(m.Run())
}
