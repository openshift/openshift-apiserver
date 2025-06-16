package testutil

import (
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/internal/imageutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// kindest/node:v1.33.1
// manifest digest: sha256:14ffd6ee8a3daa20cc934ba786626b181e1797268c5465f2c299a7cf54494c77`

// KindestManifest is the kindest/node manifest JSON.
const KindestManifest = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
   "config": {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "size": 1984,
      "digest": "sha256:d6b20550c77b11385dd30115ba29dbf9a9bfc98c2f28ff7d162a6ad7c9686251"
   },
   "layers": [
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "size": 132852852,
         "digest": "sha256:dc42dfa52495c90dc5b99c19534d6d4fa9cd37fa439356fcbd73e770c35f2293"
      },
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "size": 318908736,
         "digest": "sha256:841483099b542d6aeafc6ffacd59617954c409d3ebc558b7a95f43e05b1701a1"
      }
   ]
}`

// KindestConfigDigest is the kindest/node config digest.
const KindestConfigDigest = `sha256:d6b20550c77b11385dd30115ba29dbf9a9bfc98c2f28ff7d162a6ad7c9686251`

// KindestConfig is the kindest/node config JSON blob.
const KindestConfig = `{"architecture":"amd64","config":{"Hostname":"5e7483a6cf0e","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin","container=docker","HTTP_PROXY=","HTTPS_PROXY=","NO_PROXY="],"Cmd":null,"Image":"docker.io/kindest/base:v20250521-31a79fd4","Volumes":null,"WorkingDir":"/","Entrypoint":["/usr/local/bin/entrypoint","/sbin/init"],"OnBuild":null,"Labels":{},"StopSignal":"SIGRTMIN+3"},"container":"5e7483a6cf0e7958e796eee6912d1f6247394a0b914c822e47c7596b54aeac0a","container_config":{"Hostname":"5e7483a6cf0e","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin","container=docker"],"Cmd":["infinity"],"Image":"docker.io/kindest/base:v20250521-31a79fd4","Volumes":null,"WorkingDir":"/","Entrypoint":["sleep"],"OnBuild":null,"Labels":{},"StopSignal":"SIGRTMIN+3"},"created":"2025-05-21T01:04:14.093628812Z","docker_version":"20.10.21","history":[{"created":"2025-05-21T00:57:52.347930888Z","created_by":"COPY / / # buildkit","comment":"buildkit.dockerfile.v0"},{"created":"2025-05-21T00:57:52.347930888Z","created_by":"ENV container=docker","comment":"buildkit.dockerfile.v0","empty_layer":true},{"created":"2025-05-21T00:57:52.347930888Z","created_by":"STOPSIGNAL SIGRTMIN+3","comment":"buildkit.dockerfile.v0","empty_layer":true},{"created":"2025-05-21T00:57:52.347930888Z","created_by":"ENTRYPOINT [\"/usr/local/bin/entrypoint\" \"/sbin/init\"]","comment":"buildkit.dockerfile.v0","empty_layer":true},{"created":"2025-05-21T01:04:14.093628812Z","created_by":"infinity"}],"os":"linux","rootfs":{"type":"layers","diff_ids":["sha256:f13bb3f5a0b612fa8b3ee54536cff9a57cc76a084b6a399d7762283e52393778","sha256:2f6a2492037574ad66c92893c0d266ff1a74dbab0eeed7771c511a09f909b577"]}}`

// KindestBareImage returns a kindest/node image without metadata and layers.
func KindestBareImage(hooks ...func(*imageapi.Image)) *imageapi.Image {
	img := &imageapi.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:         KindestConfigDigest,
			GenerateName: "kindest",
		},
		DockerImageReference:         "kindest/node:v1.33.1",
		DockerImageManifestMediaType: "application/vnd.docker.distribution.manifest.v2+json",
		DockerImageManifest:          KindestManifest,
		DockerImageConfig:            KindestConfig,
	}
	for _, hook := range hooks {
		hook(img)
	}
	return img
}

// MustKindestCompleteImage returns a kindest/node image fully filled in.
func MustKindestCompleteImage(hooks ...func(*imageapi.Image)) *imageapi.Image {
	img := KindestBareImage()
	for _, hook := range hooks {
		hook(img)
	}
	if err := imageutil.InternalImageWithMetadata(img); err != nil {
		panic(err)
	}
	return img
}
