package importer

import (
	_ "embed"
	"testing"

	"github.com/distribution/distribution/v3/manifest/ocischema"
)

// imageIndexJSON actually contains the same manifests as manifestListJSON,
// so the same manifests can be used during testing.
//
//go:embed testdata/image-index.json
var imageIndexJSON []byte

const imageIndexDigest = "sha256:6f9fa17c8be41ca496faf21fcbaba3974f11bc85ecc970fa572a2a18333e681f"

func newImageIndex(t *testing.T) *ocischema.DeserializedImageIndex {
	imageIndex := &ocischema.DeserializedImageIndex{}
	if err := imageIndex.UnmarshalJSON(imageIndexJSON); err != nil {
		t.Fatal(err)
	}
	return imageIndex
}

func TestImportImageIndexWithError(t *testing.T) {
	testImportRootManifestWithError(t, newImageIndex(t), imageIndexDigest)
}

func TestImportImageIndex(t *testing.T) {
	testImportRootManifest(t, newImageIndex(t), imageIndexDigest)
}

func TestImportImageIndexSingleManifest(t *testing.T) {
	testImportRootManifestSingleManifest(t, newImageIndex(t))
}
