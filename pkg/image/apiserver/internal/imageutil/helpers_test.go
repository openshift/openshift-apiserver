package imageutil

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreapi "k8s.io/kubernetes/pkg/apis/core"

	imagev1 "github.com/openshift/api/image/v1"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
)

func TestImageWithMetadata(t *testing.T) {
	tests := map[string]struct {
		image         imageapi.Image
		expectedImage imageapi.Image
		expectError   bool
	}{
		"no manifest data": {
			image:         imageapi.Image{},
			expectedImage: imageapi.Image{},
		},
		"error unmarshalling manifest data": {
			image: imageapi.Image{
				DockerImageManifest: "{ no {{{ json here!!!",
			},
			expectedImage: imageapi.Image{},
			expectError:   true,
		},
		"no history": {
			image: imageapi.Image{
				DockerImageManifest: `{"name": "library/ubuntu", "tag": "latest"}`,
			},
			expectedImage: imageapi.Image{
				DockerImageManifest: `{"name": "library/ubuntu", "tag": "latest"}`,
			},
			expectError: true,
		},
		"happy path schema v2": {
			image: validImageWithManifestV2Data(),
			expectedImage: imageapi.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name: "id",
					Annotations: map[string]string{
						imagev1.DockerImageLayersOrderAnnotation: imagev1.DockerImageLayersOrderAscending,
					},
				},
				DockerImageConfig:            validImageWithManifestV2Data().DockerImageConfig,
				DockerImageManifest:          validImageWithManifestV2Data().DockerImageManifest,
				DockerImageManifestMediaType: "application/vnd.docker.distribution.manifest.v2+json",
				DockerImageLayers: []imageapi.ImageLayer{
					{Name: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip", LayerSize: 5312},
					{Name: "sha256:86e0e091d0da6bde2456dbb48306f3956bbeb2eae1b5b9a43045843f69fe4aaa", MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip", LayerSize: 235231},
					{Name: "sha256:86e0e091d0da6bde2456dbb48306f3956bbeb2eae1b5b9a43045843f69fe4aaa", MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip", LayerSize: 235231},
					{Name: "sha256:b4ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip", LayerSize: 639152},
				},
				DockerImageMetadata: imageapi.DockerImage{
					ID:            "sha256:815d06b56f4138afacd0009b8e3799fcdce79f0507bf8d0588e219b93ab6fd4d",
					Parent:        "",
					Comment:       "",
					Created:       metav1.Date(2015, 2, 21, 2, 11, 6, 735146646, time.UTC),
					Container:     "e91032eb0403a61bfe085ff5a5a48e3659e5a6deae9f4d678daa2ae399d5a001",
					DockerVersion: "1.9.0-dev",
					Author:        "",
					Architecture:  "amd64",
					Size:          882848,
					ContainerConfig: imageapi.DockerConfig{
						Hostname:        "23304fc829f9",
						Domainname:      "",
						User:            "",
						Memory:          0,
						MemorySwap:      0,
						CPUShares:       0,
						CPUSet:          "",
						AttachStdin:     false,
						AttachStdout:    false,
						AttachStderr:    false,
						PortSpecs:       nil,
						ExposedPorts:    nil,
						Tty:             false,
						OpenStdin:       false,
						StdinOnce:       false,
						Env:             []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "derived=true", "asdf=true"},
						Cmd:             []string{"/bin/sh", "-c", "#(nop) CMD [\"/bin/sh\" \"-c\" \"echo hi\"]"},
						Image:           "sha256:4ab15c48b859c2920dd5224f92aabcd39a52794c5b3cf088fb3bbb438756c246",
						Volumes:         nil,
						WorkingDir:      "",
						Entrypoint:      nil,
						NetworkDisabled: false,
						SecurityOpts:    nil,
						OnBuild:         []string{},
						Labels:          map[string]string{},
					},
					Config: &imageapi.DockerConfig{
						Hostname:        "23304fc829f9",
						Domainname:      "",
						User:            "",
						Memory:          0,
						MemorySwap:      0,
						CPUShares:       0,
						CPUSet:          "",
						AttachStdin:     false,
						AttachStdout:    false,
						AttachStderr:    false,
						PortSpecs:       nil,
						ExposedPorts:    nil,
						Tty:             false,
						OpenStdin:       false,
						StdinOnce:       false,
						Env:             []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "derived=true", "asdf=true"},
						Cmd:             []string{"/bin/sh", "-c", "echo hi"},
						Image:           "sha256:4ab15c48b859c2920dd5224f92aabcd39a52794c5b3cf088fb3bbb438756c246",
						Volumes:         nil,
						WorkingDir:      "",
						Entrypoint:      nil,
						NetworkDisabled: false,
						OnBuild:         []string{},
						Labels:          map[string]string{},
					},
				},
			},
		},
		"happy path OCI": {
			image: validImageWithManifestOCIData(),
			expectedImage: imageapi.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name: "id",
					Annotations: map[string]string{
						imagev1.DockerImageLayersOrderAnnotation: imagev1.DockerImageLayersOrderAscending,
					},
				},
				DockerImageConfig:            validImageWithManifestOCIData().DockerImageConfig,
				DockerImageManifest:          validImageWithManifestOCIData().DockerImageManifest,
				DockerImageManifestMediaType: "application/vnd.oci.image.manifest.v1+json",
				DockerImageLayers: []imageapi.ImageLayer{
					{
						Name:      "sha256:d9d352c11bbd3880007953ed6eec1cbace76898828f3434984a0ca60672fdf5a",
						LayerSize: 29715337,
						MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					},
				},
				DockerImageMetadata: imageapi.DockerImage{
					ID:            "sha256:bf16bdcff9c96b76a6d417bd8f0a3abe0e55c0ed9bdb3549e906834e2592fd5f",
					Parent:        "",
					Comment:       "",
					Created:       metav1.Date(2025, 5, 29, 4, 21, 1, 971275965, time.UTC),
					Container:     "57d2303e19c80641e487894fdb01e8e26ab42726f45e72624efe9d812e1c8889",
					DockerVersion: "24.0.7",
					Author:        "",
					Architecture:  "amd64",
					Size:          29718508,
					ContainerConfig: imageapi.DockerConfig{
						Hostname:        "57d2303e19c8",
						Domainname:      "",
						User:            "",
						Memory:          0,
						MemorySwap:      0,
						CPUShares:       0,
						CPUSet:          "",
						AttachStdin:     false,
						AttachStdout:    false,
						AttachStderr:    false,
						PortSpecs:       nil,
						ExposedPorts:    nil,
						Tty:             false,
						OpenStdin:       false,
						StdinOnce:       false,
						Env:             []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
						Cmd:             []string{"/bin/sh", "-c", "#(nop) ", `CMD ["/bin/bash"]`},
						Image:           "sha256:825befda5d2b1a76b71f4e1d6d31f5d82d4488b8337b1ad42e29b1340d766647",
						Volumes:         nil,
						WorkingDir:      "",
						Entrypoint:      nil,
						NetworkDisabled: false,
						SecurityOpts:    nil,
						OnBuild:         nil,
						Labels: map[string]string{
							"org.opencontainers.image.ref.name": "ubuntu",
							"org.opencontainers.image.version":  "24.04",
						},
					},
					Config: &imageapi.DockerConfig{
						Hostname:        "",
						Domainname:      "",
						User:            "",
						Memory:          0,
						MemorySwap:      0,
						CPUShares:       0,
						CPUSet:          "",
						AttachStdin:     false,
						AttachStdout:    false,
						AttachStderr:    false,
						PortSpecs:       nil,
						ExposedPorts:    nil,
						Tty:             false,
						OpenStdin:       false,
						StdinOnce:       false,
						Env:             []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
						Cmd:             []string{"/bin/bash"},
						Image:           "sha256:825befda5d2b1a76b71f4e1d6d31f5d82d4488b8337b1ad42e29b1340d766647",
						Volumes:         nil,
						WorkingDir:      "",
						Entrypoint:      nil,
						NetworkDisabled: false,
						OnBuild:         nil,
						Labels: map[string]string{
							"org.opencontainers.image.ref.name": "ubuntu",
							"org.opencontainers.image.version":  "24.04",
						},
					},
				},
			},
		},
		"happy path OCI multiple layers": {
			image: validImageWithManifestOCIDataMultipleLayers(),
			expectedImage: imageapi.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sha256:99a93c48631257c32751cc64aaa73adc2bcc1e95ef5011d975f71a04b970646b",
					Annotations: map[string]string{
						imagev1.DockerImageLayersOrderAnnotation: imagev1.DockerImageLayersOrderAscending,
					},
				},
				DockerImageConfig:            validImageWithManifestOCIDataMultipleLayers().DockerImageConfig,
				DockerImageManifest:          validImageWithManifestOCIDataMultipleLayers().DockerImageManifest,
				DockerImageManifestMediaType: "application/vnd.oci.image.manifest.v1+json",
				DockerImageLayers: []imageapi.ImageLayer{
					{
						Name:      "sha256:0c01110621e0ec1eded421406c9f117f7ae5486c8f7b0a0d1a37cc7bc9317226",
						LayerSize: 48494272,
						MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					},
					{
						Name:      "sha256:3b1eb73e993990490aa137c00e60ff4ca9d1715bafb8e888dbb0986275edb13f",
						LayerSize: 24015708,
						MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					},
					{
						Name:      "sha256:b1b8a0660a31403a35d70b276c3c86b1200b8683e83cd77a92ec98744017684a",
						LayerSize: 64399794,
						MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					},
					{
						Name:      "sha256:420c602e8633734081b143b180e927a7c4b2993e514b8b19d91b935983d0dc88",
						LayerSize: 92355229,
						MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					},
					{
						Name:      "sha256:a8f67fead7e33763b5fa924cb2e4644bbf5332ed056eb32ba0bcd3bdb68eea3b",
						LayerSize: 78981811,
						MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					},
					{
						Name:      "sha256:1f64d3b080beb286622037cab1eea6b66361f7824c5935c00e96deac1a3dadbc",
						LayerSize: 125,
						MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					},
					{
						Name:      "sha256:4f4fb700ef54461cfa02571ae0db9a0dc1e0cdb5577484a6d75e68dc38e8acc1",
						LayerSize: 32,
						MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					},
				},
				DockerImageMetadata: imageapi.DockerImage{
					ID:              "sha256:99a93c48631257c32751cc64aaa73adc2bcc1e95ef5011d975f71a04b970646b",
					Parent:          "",
					Comment:         "",
					Created:         metav1.Date(2025, 6, 5, 18, 53, 13, 0, time.UTC),
					Container:       "",
					DockerVersion:   "",
					Author:          "",
					Architecture:    "amd64",
					Size:            308250765,
					ContainerConfig: imageapi.DockerConfig{},
					Config: &imageapi.DockerConfig{
						Hostname:     "",
						Domainname:   "",
						User:         "",
						Memory:       0,
						MemorySwap:   0,
						CPUShares:    0,
						CPUSet:       "",
						AttachStdin:  false,
						AttachStdout: false,
						AttachStderr: false,
						PortSpecs:    nil,
						ExposedPorts: nil,
						Tty:          false,
						OpenStdin:    false,
						StdinOnce:    false,
						Env: []string{
							"PATH=/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
							"GOLANG_VERSION=1.24.4",
							"GOTOOLCHAIN=local",
							"GOPATH=/go",
						},
						Cmd:             []string{"bash"},
						Image:           "",
						Volumes:         nil,
						WorkingDir:      "/go",
						Entrypoint:      nil,
						NetworkDisabled: false,
						OnBuild:         nil,
						Labels:          nil,
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			imageWithMetadata := test.image
			err := InternalImageWithMetadata(&imageWithMetadata)
			gotError := err != nil
			if e, a := test.expectError, gotError; e != a {
				t.Fatalf("expectError=%t, gotError=%t: %s", e, a, err)
			}
			if test.expectError {
				return
			}
			if e, a := test.expectedImage, imageWithMetadata; !cmp.Equal(e, a) {
				t.Errorf("image: %s", cmp.Diff(e, a))
			}
		})
	}
}

func TestImageWithMetadataWithManifestList(t *testing.T) {
	image := validImageWithManifestListData()
	err := InternalImageWithMetadata(&image)
	if err != nil {
		t.Fatalf("error getting metadata for image: %#v", err)
	}

	if image.DockerImageMetadata.ID != image.Name {
		t.Error("expected image metadata ID to match image name")
		t.Logf("want: %q", image.Name)
		t.Logf("got:  %q", image.DockerImageMetadata.ID)
	}
	if image.DockerImageMetadata.Created.IsZero() {
		t.Error("expected image metadata created field to not be zero")
	}
}

func validImageWithManifestV2Data() imageapi.Image {
	return imageapi.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: "id",
		},
		DockerImageConfig: `{
    "architecture": "amd64",
    "config": {
        "AttachStderr": false,
        "AttachStdin": false,
        "AttachStdout": false,
        "Cmd": [
            "/bin/sh",
            "-c",
            "echo hi"
        ],
        "Domainname": "",
        "Entrypoint": null,
        "Env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "derived=true",
            "asdf=true"
        ],
        "Hostname": "23304fc829f9",
        "Image": "sha256:4ab15c48b859c2920dd5224f92aabcd39a52794c5b3cf088fb3bbb438756c246",
        "Labels": {},
        "OnBuild": [],
        "OpenStdin": false,
        "StdinOnce": false,
        "Tty": false,
        "User": "",
        "Volumes": null,
        "WorkingDir": ""
    },
    "container": "e91032eb0403a61bfe085ff5a5a48e3659e5a6deae9f4d678daa2ae399d5a001",
    "container_config": {
        "AttachStderr": false,
        "AttachStdin": false,
        "AttachStdout": false,
        "Cmd": [
            "/bin/sh",
            "-c",
            "#(nop) CMD [\"/bin/sh\" \"-c\" \"echo hi\"]"
        ],
        "Domainname": "",
        "Entrypoint": null,
        "Env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "derived=true",
            "asdf=true"
        ],
        "Hostname": "23304fc829f9",
        "Image": "sha256:4ab15c48b859c2920dd5224f92aabcd39a52794c5b3cf088fb3bbb438756c246",
        "Labels": {},
        "OnBuild": [],
        "OpenStdin": false,
        "StdinOnce": false,
        "Tty": false,
        "User": "",
        "Volumes": null,
        "WorkingDir": ""
    },
    "created": "2015-02-21T02:11:06.735146646Z",
    "docker_version": "1.9.0-dev",
    "history": [
        {
            "created": "2015-10-31T22:22:54.690851953Z",
            "created_by": "/bin/sh -c #(nop) ADD file:a3bc1e842b69636f9df5256c49c5374fb4eef1e281fe3f282c65fb853ee171c5 in /"
        },
        {
            "created": "2015-10-31T22:22:55.613815829Z",
            "created_by": "/bin/sh -c #(nop) CMD [\"sh\"]"
        },
        {
            "created": "2015-11-04T23:06:30.934316144Z",
            "created_by": "/bin/sh -c #(nop) ENV derived=true",
            "empty_layer": true
        },
        {
            "created": "2015-11-04T23:06:31.192097572Z",
            "created_by": "/bin/sh -c #(nop) ENV asdf=true",
            "empty_layer": true
        },
        {
            "created": "2015-11-04T23:06:32.083868454Z",
            "created_by": "/bin/sh -c dd if=/dev/zero of=/file bs=1024 count=1024"
        },
        {
            "created": "2015-11-04T23:06:32.365666163Z",
            "created_by": "/bin/sh -c #(nop) CMD [\"/bin/sh\" \"-c\" \"echo hi\"]",
            "empty_layer": true
        }
    ],
    "os": "linux",
    "rootfs": {
        "diff_ids": [
            "sha256:c6f988f4874bb0add23a778f753c65efe992244e148a1d2ec2a8b664fb66bbd1",
            "sha256:5f70bf18a086007016e948b04aed3b82103a36bea41755b6cddfaf10ace3c6ef",
            "sha256:13f53e08df5a220ab6d13c58b2bf83a59cbdc2e04d0a3f041ddf4b0ba4112d49"
        ],
        "type": "layers"
    }
}`,
		DockerImageManifest: `{
    "schemaVersion": 2,
    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
    "config": {
        "mediaType": "application/vnd.docker.container.image.v1+json",
        "size": 7023,
        "digest": "sha256:815d06b56f4138afacd0009b8e3799fcdce79f0507bf8d0588e219b93ab6fd4d"
    },
    "layers": [
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 5312,
            "digest": "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4"
        },
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 235231,
            "digest": "sha256:86e0e091d0da6bde2456dbb48306f3956bbeb2eae1b5b9a43045843f69fe4aaa"
        },
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 235231,
            "digest": "sha256:86e0e091d0da6bde2456dbb48306f3956bbeb2eae1b5b9a43045843f69fe4aaa"
        },
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 639152,
            "digest": "sha256:b4ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4"
        }
    ]
}`,
	}
}

func validImageWithManifestOCIData() imageapi.Image {
	return imageapi.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: "id",
		},
		DockerImageConfig: `{
    "architecture": "amd64",
    "config": {
        "Hostname": "",
        "Domainname": "",
        "User": "",
        "AttachStdin": false,
        "AttachStdout": false,
        "AttachStderr": false,
        "Tty": false,
        "OpenStdin": false,
        "StdinOnce": false,
        "Env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
        ],
        "Cmd": [
            "/bin/bash"
        ],
        "Image": "sha256:825befda5d2b1a76b71f4e1d6d31f5d82d4488b8337b1ad42e29b1340d766647",
        "Volumes": null,
        "WorkingDir": "",
        "Entrypoint": null,
        "OnBuild": null,
        "Labels": {
            "org.opencontainers.image.ref.name": "ubuntu",
            "org.opencontainers.image.version": "24.04"
        }
    },
    "container": "57d2303e19c80641e487894fdb01e8e26ab42726f45e72624efe9d812e1c8889",
    "container_config": {
        "Hostname": "57d2303e19c8",
        "Domainname": "",
        "User": "",
        "AttachStdin": false,
        "AttachStdout": false,
        "AttachStderr": false,
        "Tty": false,
        "OpenStdin": false,
        "StdinOnce": false,
        "Env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
        ],
        "Cmd": [
            "/bin/sh",
            "-c",
            "#(nop) ",
            "CMD [\"/bin/bash\"]"
        ],
        "Image": "sha256:825befda5d2b1a76b71f4e1d6d31f5d82d4488b8337b1ad42e29b1340d766647",
        "Volumes": null,
        "WorkingDir": "",
        "Entrypoint": null,
        "OnBuild": null,
        "Labels": {
            "org.opencontainers.image.ref.name": "ubuntu",
            "org.opencontainers.image.version": "24.04"
        }
    },
    "created": "2025-05-29T04:21:01.971275965Z",
    "docker_version": "24.0.7",
    "history": [
        {
            "created": "2025-05-29T04:20:59.390476489Z",
            "created_by": "/bin/sh -c #(nop)  ARG RELEASE",
            "empty_layer": true
        },
        {
            "created": "2025-05-29T04:20:59.425928067Z",
            "created_by": "/bin/sh -c #(nop)  ARG LAUNCHPAD_BUILD_ARCH",
            "empty_layer": true
        },
        {
            "created": "2025-05-29T04:20:59.461048974Z",
            "created_by": "/bin/sh -c #(nop)  LABEL org.opencontainers.image.ref.name=ubuntu",
            "empty_layer": true
        },
        {
            "created": "2025-05-29T04:20:59.498669132Z",
            "created_by": "/bin/sh -c #(nop)  LABEL org.opencontainers.image.version=24.04",
            "empty_layer": true
        },
        {
            "created": "2025-05-29T04:21:01.6549815Z",
            "created_by": "/bin/sh -c #(nop) ADD file:598ca0108009b5c2e9e6f4fc4bd19a6bcd604fccb5b9376fac14a75522a5cfa3 in / "
        },
        {
            "created": "2025-05-29T04:21:01.971275965Z",
            "created_by": "/bin/sh -c #(nop)  CMD [\"/bin/bash\"]",
            "empty_layer": true
        }
    ],
    "os": "linux",
    "rootfs": {
        "type": "layers",
        "diff_ids": [
            "sha256:a8346d259389bc6221b4f3c61bad4e48087c5b82308e8f53ce703cfc8333c7b3"
        ]
    }
}`,
		DockerImageManifest: `{
    "schemaVersion": 2,
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "config": {
        "mediaType": "application/vnd.oci.image.config.v1+json",
        "size": 2295,
        "digest": "sha256:bf16bdcff9c96b76a6d417bd8f0a3abe0e55c0ed9bdb3549e906834e2592fd5f"
    },
    "layers": [
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "size": 29715337,
            "digest": "sha256:d9d352c11bbd3880007953ed6eec1cbace76898828f3434984a0ca60672fdf5a"
        }
    ]
}`,
	}
}

func validImageWithManifestOCIDataMultipleLayers() imageapi.Image {
	return imageapi.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sha256:99a93c48631257c32751cc64aaa73adc2bcc1e95ef5011d975f71a04b970646b",
		},
		DockerImageConfig: `{
    "architecture": "amd64",
    "config": {
        "Env": [
            "PATH=/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "GOLANG_VERSION=1.24.4",
            "GOTOOLCHAIN=local",
            "GOPATH=/go"
        ],
        "Cmd": [
            "bash"
        ],
        "WorkingDir": "/go"
    },
    "created": "2025-06-05T18:53:13Z",
    "history": [
        {
            "created": "2023-05-10T23:29:59Z",
            "created_by": "# debian.sh --arch 'amd64' out/ 'bookworm' '@1749513600'",
            "comment": "debuerreotype 0.15"
        },
        {
            "created": "2023-05-10T23:29:59Z",
            "created_by": "RUN /bin/sh -c set -eux; \tapt-get update; \tapt-get install -y --no-install-recommends \t\tca-certificates \t\tcurl \t\tgnupg \t\tnetbase \t\tsq \t\twget \t; \trm -rf /var/lib/apt/lists/* # buildkit",
            "comment": "buildkit.dockerfile.v0"
        },
        {
            "created": "2024-01-09T01:14:25Z",
            "created_by": "RUN /bin/sh -c set -eux; \tapt-get update; \tapt-get install -y --no-install-recommends \t\tgit \t\tmercurial \t\topenssh-client \t\tsubversion \t\t\t\tprocps \t; \trm -rf /var/lib/apt/lists/* # buildkit",
            "comment": "buildkit.dockerfile.v0"
        },
        {
            "created": "2025-06-05T18:53:13Z",
            "created_by": "RUN /bin/sh -c set -eux; \tapt-get update; \tapt-get install -y --no-install-recommends \t\tg++ \t\tgcc \t\tlibc6-dev \t\tmake \t\tpkg-config \t; \trm -rf /var/lib/apt/lists/* # buildkit",
            "comment": "buildkit.dockerfile.v0"
        },
        {
            "created": "2025-06-05T18:53:13Z",
            "created_by": "ENV GOLANG_VERSION=1.24.4",
            "comment": "buildkit.dockerfile.v0",
            "empty_layer": true
        },
        {
            "created": "2025-06-05T18:53:13Z",
            "created_by": "ENV GOTOOLCHAIN=local",
            "comment": "buildkit.dockerfile.v0",
            "empty_layer": true
        },
        {
            "created": "2025-06-05T18:53:13Z",
            "created_by": "ENV GOPATH=/go",
            "comment": "buildkit.dockerfile.v0",
            "empty_layer": true
        },
        {
            "created": "2025-06-05T18:53:13Z",
            "created_by": "ENV PATH=/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "comment": "buildkit.dockerfile.v0",
            "empty_layer": true
        },
        {
            "created": "2025-06-05T18:53:13Z",
            "created_by": "COPY /target/ / # buildkit",
            "comment": "buildkit.dockerfile.v0"
        },
        {
            "created": "2025-06-05T18:53:13Z",
            "created_by": "RUN /bin/sh -c mkdir -p \"$GOPATH/src\" \"$GOPATH/bin\" && chmod -R 1777 \"$GOPATH\" # buildkit",
            "comment": "buildkit.dockerfile.v0"
        },
        {
            "created": "2025-06-05T18:53:13Z",
            "created_by": "WORKDIR /go",
            "comment": "buildkit.dockerfile.v0"
        }
    ],
    "os": "linux",
    "rootfs": {
        "type": "layers",
        "diff_ids": [
            "sha256:8f003894a7efc4178494f1e133497ed2f325ae53b6a65869e54c04d1c51d588f",
            "sha256:f5b8fb1def00d5f185660b75bac1eed1fce467d44cebd868dae2f344711321ef",
            "sha256:1c49688bd8ebe54298be2b61f7d5efd32467862f115b5439b87c19e56e57c6b4",
            "sha256:c291adf4681b803d1a3fdd12233811fb566c89cf1d423bd62f853d35aeb2c32f",
            "sha256:86d0740ea51f822cea0316cc9b0aaf705545175ab281342f460f0f14e8742502",
            "sha256:7c5761aef9e0522152cc129c12b38d65073013ae75888975e1b8469556f3af70",
            "sha256:5f70bf18a086007016e948b04aed3b82103a36bea41755b6cddfaf10ace3c6ef"
        ]
    }
}`,
		DockerImageManifest: `{
    "schemaVersion": 2,
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "config": {
        "mediaType": "application/vnd.oci.image.config.v1+json",
        "digest": "sha256:99a93c48631257c32751cc64aaa73adc2bcc1e95ef5011d975f71a04b970646b",
        "size": 2803
    },
    "layers": [
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:0c01110621e0ec1eded421406c9f117f7ae5486c8f7b0a0d1a37cc7bc9317226",
            "size": 48494272
        },
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:3b1eb73e993990490aa137c00e60ff4ca9d1715bafb8e888dbb0986275edb13f",
            "size": 24015708
        },
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:b1b8a0660a31403a35d70b276c3c86b1200b8683e83cd77a92ec98744017684a",
            "size": 64399794
        },
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:420c602e8633734081b143b180e927a7c4b2993e514b8b19d91b935983d0dc88",
            "size": 92355229
        },
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:a8f67fead7e33763b5fa924cb2e4644bbf5332ed056eb32ba0bcd3bdb68eea3b",
            "size": 78981811
        },
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:1f64d3b080beb286622037cab1eea6b66361f7824c5935c00e96deac1a3dadbc",
            "size": 125
        },
        {
            "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "digest": "sha256:4f4fb700ef54461cfa02571ae0db9a0dc1e0cdb5577484a6d75e68dc38e8acc1",
            "size": 32
        }
    ],
    "annotations": {
        "com.docker.official-images.bashbrew.arch": "amd64",
        "org.opencontainers.image.base.digest": "sha256:fab1b6389a07a117b06169a1c2bc5a4a3d3f5d7315dea5ebf4d7ad49606d7f32",
        "org.opencontainers.image.base.name": "buildpack-deps:bookworm-scm",
        "org.opencontainers.image.created": "2025-06-05T18:53:13Z",
        "org.opencontainers.image.revision": "205cf586b0d0c7200e0fd642feaf738ddb382da0",
        "org.opencontainers.image.source": "https://github.com/docker-library/golang.git#205cf586b0d0c7200e0fd642feaf738ddb382da0:1.24/bookworm",
        "org.opencontainers.image.url": "https://hub.docker.com/_/golang",
        "org.opencontainers.image.version": "1.24.4-bookworm"
    }
}`,
	}
}

func validImageWithManifestListData() imageapi.Image {
	return imageapi.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sha256:8b8cc63bcc10374ef349ec4f27a3aa1eb2dcd5a098d4f5f51fafac4df5db3fd7",
		},
		DockerImageManifest: `{
  "manifests": [
    {
      "digest": "sha256:d4ea9af4372bd4c4973725c727f0718e68de3d37452d9f5a1abe77c64907d6c2",
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "platform": {
        "architecture": "amd64",
        "os": "linux"
      },
      "size": 429
    },
    {
      "digest": "sha256:31630ae562f99165f42e98cb079d64d970cfec203bb0d0973bafb4fdac72650c",
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "platform": {
        "architecture": "arm64",
        "os": "linux"
      },
      "size": 429
    },
    {
      "digest": "sha256:9e2c03e2fdcaf6c5f9df005ec56ab767615d260be6816a3727ee31f5b3055803",
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "platform": {
        "architecture": "ppc64le",
        "os": "linux"
      },
      "size": 429
    },
    {
      "digest": "sha256:9712ef29952de0c6f9d3c5ef5913fe49515191c2396740c6e937d5a02bfb5d1c",
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "platform": {
        "architecture": "s390x",
        "os": "linux"
      },
      "size": 429
    }
  ],
  "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
  "schemaVersion": 2
}`,
		DockerImageManifests: []imageapi.ImageManifest{
			{
				Digest:       "sha256:d4ea9af4372bd4c4973725c727f0718e68de3d37452d9f5a1abe77c64907d6c2",
				MediaType:    "application/vnd.docker.distribution.manifest.v2+json",
				ManifestSize: 429,
				Architecture: "amd64",
				OS:           "linux",
			},
			{
				Digest:       "sha256:31630ae562f99165f42e98cb079d64d970cfec203bb0d0973bafb4fdac72650c",
				MediaType:    "application/vnd.docker.distribution.manifest.v2+json",
				ManifestSize: 429,
				Architecture: "arm64",
				OS:           "linux",
			},
			{
				Digest:       "sha256:9e2c03e2fdcaf6c5f9df005ec56ab767615d260be6816a3727ee31f5b3055803",
				MediaType:    "application/vnd.docker.distribution.manifest.v2+json",
				ManifestSize: 429,
				Architecture: "ppc64le",
				OS:           "linux",
			},
			{
				Digest:       "sha256:9712ef29952de0c6f9d3c5ef5913fe49515191c2396740c6e937d5a02bfb5d1c",
				MediaType:    "application/vnd.docker.distribution.manifest.v2+json",
				ManifestSize: 429,
				Architecture: "s390x",
				OS:           "linux",
			},
		},
	}
}

func TestLatestTaggedImage(t *testing.T) {
	tests := []struct {
		tag            string
		tags           map[string]imageapi.TagEventList
		expected       string
		expectNotFound bool
	}{
		{
			tag:            "foo",
			tags:           map[string]imageapi.TagEventList{},
			expectNotFound: true,
		},
		{
			tag: "foo",
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref"},
						{DockerImageReference: "older"},
					},
				},
			},
			expectNotFound: true,
		},
		{
			tag: "",
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref"},
						{DockerImageReference: "older"},
					},
				},
			},
			expected: "latest-ref",
		},
		{
			tag: "foo",
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref"},
						{DockerImageReference: "older"},
					},
				},
				"foo": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "foo-ref"},
						{DockerImageReference: "older"},
					},
				},
			},
			expected: "foo-ref",
		},
	}

	for i, test := range tests {
		stream := &imageapi.ImageStream{}
		stream.Status.Tags = test.tags

		actual := LatestTaggedImage(stream, test.tag)
		if actual == nil {
			if !test.expectNotFound {
				t.Errorf("%d: unexpected nil result", i)
			}
			continue
		}
		if e, a := test.expected, actual.DockerImageReference; e != a {
			t.Errorf("%d: expected %q, got %q", i, e, a)
		}
	}
}

func TestResolveLatestTaggedImage(t *testing.T) {
	tests := []struct {
		tag            string
		statusRef      string
		refs           map[string]imageapi.TagReference
		tags           map[string]imageapi.TagEventList
		expected       string
		expectNotFound bool
	}{
		{
			tag:            "foo",
			tags:           map[string]imageapi.TagEventList{},
			expectNotFound: true,
		},
		{
			tag: "foo",
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref"},
						{DockerImageReference: "older"},
					},
				},
			},
			expectNotFound: true,
		},
		{
			tag: "",
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref"},
						{DockerImageReference: "older"},
					},
				},
			},
			expected: "latest-ref",
		},
		{
			tag: "foo",
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref"},
						{DockerImageReference: "older"},
					},
				},
				"foo": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "foo-ref"},
						{DockerImageReference: "older"},
					},
				},
			},
			expected: "foo-ref",
		},

		// the default reference policy does nothing
		{
			refs: map[string]imageapi.TagReference{
				"latest": {
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.SourceTagReferencePolicy},
				},
			},
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref", Image: "sha256:4ab15c48b859c2920dd5224f92aabcd39a52794c5b3cf088fb3bbb438756c246"},
						{DockerImageReference: "older"},
					},
				},
			},
			expected: "latest-ref",
		},

		// the local reference policy does nothing unless reference is set
		{
			refs: map[string]imageapi.TagReference{
				"latest": {
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
				},
			},
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref", Image: "sha256:4ab15c48b859c2920dd5224f92aabcd39a52794c5b3cf088fb3bbb438756c246"},
						{DockerImageReference: "older"},
					},
				},
			},
			expected: "latest-ref",
		},

		// the local reference policy does nothing unless the image id is set
		{
			statusRef: "test.server/a/b",
			refs: map[string]imageapi.TagReference{
				"latest": {
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
				},
			},
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref"},
						{DockerImageReference: "older"},
					},
				},
			},
			expected: "latest-ref",
		},

		// the local reference policy uses the output status reference and the image id
		// and returns a pullthrough spec
		{
			statusRef: "test.server/a/b",
			refs: map[string]imageapi.TagReference{
				"latest": {
					ReferencePolicy: imageapi.TagReferencePolicy{Type: imageapi.LocalTagReferencePolicy},
				},
			},
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{DockerImageReference: "latest-ref", Image: "sha256:4ab15c48b859c2920dd5224f92aabcd39a52794c5b3cf088fb3bbb438756c246"},
						{DockerImageReference: "older"},
					},
				},
			},
			expected: "test.server/a/b@sha256:4ab15c48b859c2920dd5224f92aabcd39a52794c5b3cf088fb3bbb438756c246",
		},
	}

	for i, test := range tests {
		stream := &imageapi.ImageStream{}
		stream.Status.DockerImageRepository = test.statusRef
		stream.Status.Tags = test.tags
		stream.Spec.Tags = test.refs

		actual, ok := ResolveLatestTaggedImage(stream, test.tag)
		if !ok {
			if !test.expectNotFound {
				t.Errorf("%d: unexpected nil result", i)
			}
			continue
		}
		if e, a := test.expected, actual; e != a {
			t.Errorf("%d: expected %q, got %q", i, e, a)
		}
	}
}

func TestAddTagEventToImageStream(t *testing.T) {
	tests := map[string]struct {
		tags           map[string]imageapi.TagEventList
		nextRef        string
		nextImage      string
		expectedTags   map[string]imageapi.TagEventList
		expectedUpdate bool
	}{
		"nil entry for tag": {
			tags:      map[string]imageapi.TagEventList{},
			nextRef:   "ref",
			nextImage: "image",
			expectedTags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "ref",
							Image:                "image",
						},
					},
				},
			},
			expectedUpdate: true,
		},
		"empty items for tag": {
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{},
				},
			},
			nextRef:   "ref",
			nextImage: "image",
			expectedTags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "ref",
							Image:                "image",
						},
					},
				},
			},
			expectedUpdate: true,
		},
		"same ref and image": {
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "ref",
							Image:                "image",
						},
					},
				},
			},
			nextRef:   "ref",
			nextImage: "image",
			expectedTags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "ref",
							Image:                "image",
						},
					},
				},
			},
			expectedUpdate: false,
		},
		"same ref, different image": {
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "ref",
							Image:                "image",
						},
					},
				},
			},
			nextRef:   "ref",
			nextImage: "newimage",
			expectedTags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "ref",
							Image:                "newimage",
						},
					},
				},
			},
			expectedUpdate: true,
		},
		"different ref, same image": {
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "ref",
							Image:                "image",
						},
					},
				},
			},
			nextRef:   "newref",
			nextImage: "image",
			expectedTags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "newref",
							Image:                "image",
						},
					},
				},
			},
			expectedUpdate: true,
		},
		"different ref, different image": {
			tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "ref",
							Image:                "image",
						},
					},
				},
			},
			nextRef:   "newref",
			nextImage: "newimage",
			expectedTags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "newref",
							Image:                "newimage",
						},
						{
							DockerImageReference: "ref",
							Image:                "image",
						},
					},
				},
			},
			expectedUpdate: true,
		},
	}

	for name, test := range tests {
		stream := &imageapi.ImageStream{}
		stream.Status.Tags = test.tags
		updated := AddTagEventToImageStream(stream, "latest", imageapi.TagEvent{DockerImageReference: test.nextRef, Image: test.nextImage})
		if e, a := test.expectedUpdate, updated; e != a {
			t.Errorf("%s: expected updated=%t, got %t", name, e, a)
		}
		if e, a := test.expectedTags, stream.Status.Tags; !reflect.DeepEqual(e, a) {
			t.Errorf("%s: expected\ntags=%#v\ngot=%#v", name, e, a)
		}
	}
}

func TestUpdateTrackingTags(t *testing.T) {
	tests := map[string]struct {
		fromNil               bool
		fromKind              string
		fromNamespace         string
		fromName              string
		trackingTags          []string
		nonTrackingTags       []string
		statusTags            []string
		updatedImageReference string
		updatedImage          string
		expectedUpdates       []string
	}{
		"nil from": {
			fromNil: true,
		},
		"from kind not ImageStreamTag": {
			fromKind: "ImageStreamImage",
		},
		"from namespace different": {
			fromNamespace: "other",
		},
		"from name different": {
			trackingTags: []string{"otherstream:2.0"},
		},
		"no tracking": {
			trackingTags: []string{},
			statusTags:   []string{"2.0", "3.0"},
		},
		"stream name in from name": {
			trackingTags:    []string{"latest"},
			fromName:        "ruby:2.0",
			statusTags:      []string{"2.0", "3.0"},
			expectedUpdates: []string{"latest"},
		},
		"1 tracking, 1 not": {
			trackingTags:    []string{"latest"},
			nonTrackingTags: []string{"other"},
			statusTags:      []string{"2.0", "3.0"},
			expectedUpdates: []string{"latest"},
		},
		"multiple tracking, multiple not": {
			trackingTags:    []string{"latest1", "latest2"},
			nonTrackingTags: []string{"other1", "other2"},
			statusTags:      []string{"2.0", "3.0"},
			expectedUpdates: []string{"latest1", "latest2"},
		},
		"no change to tracked tag": {
			trackingTags:          []string{"latest"},
			statusTags:            []string{"2.0", "3.0"},
			updatedImageReference: "ns/ruby@id",
			updatedImage:          "id",
		},
	}

	for name, test := range tests {
		stream := &imageapi.ImageStream{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "ruby",
			},
			Spec: imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{},
			},
			Status: imageapi.ImageStreamStatus{
				Tags: map[string]imageapi.TagEventList{},
			},
		}

		if len(test.fromNamespace) > 0 {
			stream.Namespace = test.fromNamespace
		}

		fromName := test.fromName
		if len(fromName) == 0 {
			fromName = "2.0"
		}

		for _, tag := range test.trackingTags {
			stream.Spec.Tags[tag] = imageapi.TagReference{
				From: &coreapi.ObjectReference{
					Kind: "ImageStreamTag",
					Name: fromName,
				},
			}
		}

		for _, tag := range test.nonTrackingTags {
			stream.Spec.Tags[tag] = imageapi.TagReference{}
		}

		for _, tag := range test.statusTags {
			stream.Status.Tags[tag] = imageapi.TagEventList{
				Items: []imageapi.TagEvent{
					{
						DockerImageReference: "ns/ruby@id",
						Image:                "id",
					},
				},
			}
		}

		if test.fromNil {
			stream.Spec.Tags = map[string]imageapi.TagReference{
				"latest": {},
			}
		}

		if len(test.fromKind) > 0 {
			stream.Spec.Tags = map[string]imageapi.TagReference{
				"latest": {
					From: &coreapi.ObjectReference{
						Kind: test.fromKind,
						Name: "asdf",
					},
				},
			}
		}

		updatedImageReference := test.updatedImageReference
		if len(updatedImageReference) == 0 {
			updatedImageReference = "ns/ruby@newid"
		}

		updatedImage := test.updatedImage
		if len(updatedImage) == 0 {
			updatedImage = "newid"
		}

		newTagEvent := imageapi.TagEvent{
			DockerImageReference: updatedImageReference,
			Image:                updatedImage,
		}

		UpdateTrackingTags(stream, "2.0", newTagEvent)
		for _, tag := range test.expectedUpdates {
			tagEventList, ok := stream.Status.Tags[tag]
			if !ok {
				t.Errorf("%s: expected update for tag %q", name, tag)
				continue
			}
			if e, a := updatedImageReference, tagEventList.Items[0].DockerImageReference; e != a {
				t.Errorf("%s: dockerImageReference: expected %q, got %q", name, e, a)
			}
			if e, a := updatedImage, tagEventList.Items[0].Image; e != a {
				t.Errorf("%s: image: expected %q, got %q", name, e, a)
			}
		}
	}
}

func TestResolveImageID(t *testing.T) {
	tests := map[string]struct {
		tags     map[string]imageapi.TagEventList
		imageID  string
		expErr   string
		expEvent imageapi.TagEvent
	}{
		"single tag, match ID prefix": {
			tags: map[string]imageapi.TagEventList{
				"tag1": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "repo@sha256:3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25",
							Image:                "sha256:3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25",
						},
					},
				},
			},
			imageID: "3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25",
			expErr:  "",
			expEvent: imageapi.TagEvent{
				DockerImageReference: "repo@sha256:3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25",
				Image:                "sha256:3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25",
			},
		},
		"single tag, match string prefix": {
			tags: map[string]imageapi.TagEventList{
				"tag1": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "repo:mytag",
							Image:                "mytag",
						},
					},
				},
			},
			imageID: "mytag",
			expErr:  "",
			expEvent: imageapi.TagEvent{
				DockerImageReference: "repo:mytag",
				Image:                "mytag",
			},
		},
		"single tag, ID error": {
			tags: map[string]imageapi.TagEventList{
				"tag1": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "repo@sha256:3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b2",
							Image:                "sha256:3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b2",
						},
					},
				},
			},
			imageID:  "3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25",
			expErr:   "not found",
			expEvent: imageapi.TagEvent{},
		},
		"no tag": {
			tags:     map[string]imageapi.TagEventList{},
			imageID:  "3c87c572822935df60f0f5d3665bd376841a7fcfeb806b5f212de6a00e9a7b25",
			expErr:   "not found",
			expEvent: imageapi.TagEvent{},
		},
		"multiple match": {
			tags: map[string]imageapi.TagEventList{
				"tag1": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "repo@mytag",
							Image:                "mytag",
						},
						{
							DockerImageReference: "repo@mytag",
							Image:                "mytag2",
						},
					},
				},
			},
			imageID:  "mytag",
			expErr:   "multiple images match the prefix",
			expEvent: imageapi.TagEvent{},
		},
		"find match out of multiple tags in first position": {
			tags: map[string]imageapi.TagEventList{
				"tag1": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000001",
							Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000001",
						},
						{
							DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000002",
							Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000002",
						},
					},
				},
				"tag2": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000003",
							Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000003",
						},
						{
							DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000004",
							Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000004",
						},
					},
				},
			},
			imageID: "sha256:0000000000000000000000000000000000000000000000000000000000000001",
			expEvent: imageapi.TagEvent{
				DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000001",
				Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000001",
			},
		},
		"find match out of multiple tags in last position": {
			tags: map[string]imageapi.TagEventList{
				"tag1": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000001",
							Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000001",
						},
						{
							DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000002",
							Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000002",
						},
					},
				},
				"tag2": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000003",
							Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000003",
						},
						{
							DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000004",
							Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000004",
						},
					},
				},
			},
			imageID: "sha256:0000000000000000000000000000000000000000000000000000000000000004",
			expEvent: imageapi.TagEvent{
				DockerImageReference: "repo@sha256:0000000000000000000000000000000000000000000000000000000000000004",
				Image:                "sha256:0000000000000000000000000000000000000000000000000000000000000004",
			},
		},
	}

	for name, test := range tests {
		stream := &imageapi.ImageStream{}
		stream.Status.Tags = test.tags
		event, err := ResolveImageID(stream, test.imageID)
		if len(test.expErr) > 0 {
			if err == nil || !strings.Contains(err.Error(), test.expErr) {
				t.Errorf("%s: unexpected error, expected %v, got %v", name, test.expErr, err)
			}
			continue
		} else if err != nil {
			t.Errorf("%s: unexpected error, got %v", name, err)
			continue
		}
		if test.expEvent.Image != event.Image || test.expEvent.DockerImageReference != event.DockerImageReference {
			t.Errorf("%s: unexpected tag, expected %#v, got %#v", name, test.expEvent, event)
		}
	}
}
