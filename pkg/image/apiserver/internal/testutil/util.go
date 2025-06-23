package testutil

import (
	"fmt"
)

// InternalRegistryURL is an url of internal container image registry for testing purposes.
const InternalRegistryURL = "172.30.12.34:5000"

// MakeDockerImageReference makes a container image reference string referencing testing internal docker
// registry.
func MakeDockerImageReference(ns, isName, imageID string) string {
	return fmt.Sprintf("%s/%s/%s@%s", InternalRegistryURL, ns, isName, imageID)
}

// BaseImageWith1Layer contains a single layer.
const BaseImageWith1Layer = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
   "config": {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "size": 1512,
      "digest": "sha256:6c6084ed97e5851b5d216b20ed1852301278584c3c6aff915272b231593f6f98"
   },
   "layers": [
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "size": 1970140,
         "digest": "sha256:550fe1bea624a5c62551cf09f3aa10886eed133794844af1dfb775118309387e"
      }
   ]
}`

// BaseImageWith1LayerDigest is the digest associated with BaseImageWith1Layer.
//
// This is actually docksal/empty.
const BaseImageWith1LayerDigest = `sha256:f853843b26903da94dd1cdf9e39ff7e2ba7a754388341895d557dbe913f5a915`

// BaseImageWith1LayerConfig is the config associated with BaseImageWith1Layer.
const BaseImageWith1LayerConfig = `{
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
      "/bin/sh"
    ],
    "ArgsEscaped": true,
    "Image": "sha256:01b85c6717c3b1f5379864199c541cecabb81be758b4fec6ef0b66cbfb6e11a5",
    "Volumes": null,
    "WorkingDir": "",
    "Entrypoint": null,
    "OnBuild": null,
    "Labels": null
  },
  "container": "f8a4df32c288f30c6d641c3945c88b64490e1e029be516209955023786cf1727",
  "container_config": {
    "Hostname": "f8a4df32c288",
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
      "CMD [\"/bin/sh\"]"
    ],
    "ArgsEscaped": true,
    "Image": "sha256:01b85c6717c3b1f5379864199c541cecabb81be758b4fec6ef0b66cbfb6e11a5",
    "Volumes": null,
    "WorkingDir": "",
    "Entrypoint": null,
    "OnBuild": null,
    "Labels": {}
  },
  "created": "2018-01-09T21:13:01.402230769Z",
  "docker_version": "17.06.2-ce",
  "history": [
    {
      "created": "2018-01-09T21:13:01.165340448Z",
      "created_by": "/bin/sh -c #(nop) ADD file:df48d6d6df42a01380557aebd4ca02807fc08a76a1d1b36d957e59a41c69db0b in / "
    },
    {
      "created": "2018-01-09T21:13:01.402230769Z",
      "created_by": "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]",
      "empty_layer": true
    }
  ],
  "os": "linux",
  "rootfs": {
    "type": "layers",
    "diff_ids": [
      "sha256:d39d92664027be502c35cf1bf464c726d15b8ead0e3084be6e252a161730bc82"
    ]
  }
}`

// The following digests are actually random SHA256 hashes.

const BaseImageWith2LayersDigest = "sha256:5bb720a64ecc8f5285cda9d899db4a79f2fc73b4533e4d7d7ffd9b7b6720c159"

const ChildImageWith2LayersDigest = "sha256:4ec0a236b636e898d557205b01683560c07ce0edc949706c03e7cb2e7037093e"

const ChildImageWith3LayersDigest = "sha256:9237f69ed1eb6221da9d28569669ae5e73173d6a67f88265726d2fad47e31df2"

const MiscImageDigest = "sha256:e07072af8e05843efde5e4f2f23ee12b96029cf6d5685fa9f4cbc45f2196011e"

const ManifestList = `{
  "manifests": [
    {
      "digest": "sha256:96a76fa48db5fca24271fe1565d88a4453e759b365dbaaeeb5a4e41049293e77",
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "platform": {
        "architecture": "amd64",
        "os": "linux"
      },
      "size": 429
    },
    {
      "digest": "sha256:6c3d8fec1c50ff78997e13a8352b030d4b290f656081c974373753fd5a3496f1",
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "platform": {
        "architecture": "arm64",
        "os": "linux"
      },
      "size": 429
    },
    {
      "digest": "sha256:520a368f78807947b96ea773cc62b14e380f4af08bbfd8ed18f0ebc70dedef68",
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "platform": {
        "architecture": "ppc64le",
        "os": "linux"
      },
      "size": 429
    },
    {
      "digest": "sha256:50b0c55990fe1b48c4b026fb6b49b4377e36c52b291d434977793dc0c8998ba4",
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
}`
