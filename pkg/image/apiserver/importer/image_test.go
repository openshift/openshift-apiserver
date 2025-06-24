package importer

import (
	_ "embed"
	"testing"

	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/schema2"
)

func TestManifestListToImageConversion(t *testing.T) {
	manifestList := &manifestlist.DeserializedManifestList{}
	err := manifestList.UnmarshalJSON(manifestListJSON)
	if err != nil {
		t.Fatal(err)
	}

	image, err := manifestToImage(manifestList, "sha256:5020d54ec2de60c4e187128b5a03adda261a7fe78c9c500ffd24ff4af476fb41")
	if err != nil {
		t.Fatal(err)
	}
	if image == nil {
		t.Fatal("Image unexpectedly nil")
	}

	// the raw manifest json takes a lot of storage space, and we have no further
	// use for it once manifestListToImage finishes creating the Image object,
	// so we simply discard it.
	// furthermore we also avoid running internalimageutil.InternalImageWithMetadata
	// for manifest lists - that function is for manifests only.
	if image.DockerImageManifest != "" {
		t.Error("DockerImageManifest should not be set, got non-empty string")
		t.Log(image.DockerImageManifest)
	}

	if image.DockerImageManifests == nil {
		t.Fatal("Expected `image.DockerImageManifests` field to be populated with sub-manifests, was nil")
	}

	linuxAMD64 := image.DockerImageManifests[0]
	if linuxAMD64.MediaType != schema2.MediaTypeManifest {
		t.Error("Sub-manifest media type did not match expected")
	}
	expectedDigest := "sha256:ca013ac5c09f9a9f6db8370c1b759a29fe997d64d6591e9a75b71748858f7da0"
	if linuxAMD64.Digest != expectedDigest {
		t.Error("Sub-manifest digest did not match expected")
		t.Logf("expected: '%s'", expectedDigest)
		t.Logf("got:      '%s'", linuxAMD64.Digest)
	}
	expectedArch := "amd64"
	if linuxAMD64.Architecture != expectedArch {
		t.Error("Architecture did not match expected")
		t.Logf("expected: '%s'", expectedArch)
		t.Logf("got:      '%s'", linuxAMD64.Architecture)
	}
	expectedOS := "linux"
	if linuxAMD64.OS != expectedOS {
		t.Error("OS did not match expected")
		t.Logf("expected: '%s'", expectedOS)
		t.Logf("got:      '%s'", linuxAMD64.OS)
	}
}
