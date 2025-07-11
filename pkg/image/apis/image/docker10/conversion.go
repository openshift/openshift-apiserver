package docker10

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/openshift-apiserver/pkg/image/apis/image"
)

// Convert_DockerImageConfig_to_image_DockerImage takes a container image
// registry digest (schema 2.2) and converts it to the internal API version of
// Image.
func Convert_DockerImageConfig_to_image_DockerImage(in *DockerImageConfig, out *image.DockerImage) error {
	*out = image.DockerImage{
		ID:            in.ID,
		Parent:        in.Parent,
		Comment:       in.Comment,
		Created:       metav1.Time{Time: in.Created},
		Container:     in.Container,
		DockerVersion: in.DockerVersion,
		Author:        in.Author,
		Architecture:  in.Architecture,
		Size:          in.Size,
	}
	if err := Convert_docker10_DockerConfig_To_image_DockerConfig(&in.ContainerConfig, &out.ContainerConfig, nil); err != nil {
		return err
	}
	if in.Config != nil {
		out.Config = &image.DockerConfig{}
		if err := Convert_docker10_DockerConfig_To_image_DockerConfig(in.Config, out.Config, nil); err != nil {
			return err
		}
	}
	return nil
}
