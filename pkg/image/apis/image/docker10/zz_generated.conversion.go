//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Code generated by conversion-gen. DO NOT EDIT.

package docker10

import (
	unsafe "unsafe"

	imagedocker10 "github.com/openshift/api/image/docker10"
	image "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*imagedocker10.DockerConfig)(nil), (*image.DockerConfig)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_docker10_DockerConfig_To_image_DockerConfig(a.(*imagedocker10.DockerConfig), b.(*image.DockerConfig), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*image.DockerConfig)(nil), (*imagedocker10.DockerConfig)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_image_DockerConfig_To_docker10_DockerConfig(a.(*image.DockerConfig), b.(*imagedocker10.DockerConfig), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*imagedocker10.DockerImage)(nil), (*image.DockerImage)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_docker10_DockerImage_To_image_DockerImage(a.(*imagedocker10.DockerImage), b.(*image.DockerImage), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*image.DockerImage)(nil), (*imagedocker10.DockerImage)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_image_DockerImage_To_docker10_DockerImage(a.(*image.DockerImage), b.(*imagedocker10.DockerImage), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_docker10_DockerConfig_To_image_DockerConfig(in *imagedocker10.DockerConfig, out *image.DockerConfig, s conversion.Scope) error {
	out.Hostname = in.Hostname
	out.Domainname = in.Domainname
	out.User = in.User
	out.Memory = in.Memory
	out.MemorySwap = in.MemorySwap
	out.CPUShares = in.CPUShares
	out.CPUSet = in.CPUSet
	out.AttachStdin = in.AttachStdin
	out.AttachStdout = in.AttachStdout
	out.AttachStderr = in.AttachStderr
	out.PortSpecs = *(*[]string)(unsafe.Pointer(&in.PortSpecs))
	out.ExposedPorts = *(*map[string]struct{})(unsafe.Pointer(&in.ExposedPorts))
	out.Tty = in.Tty
	out.OpenStdin = in.OpenStdin
	out.StdinOnce = in.StdinOnce
	out.Env = *(*[]string)(unsafe.Pointer(&in.Env))
	out.Cmd = *(*[]string)(unsafe.Pointer(&in.Cmd))
	out.DNS = *(*[]string)(unsafe.Pointer(&in.DNS))
	out.Image = in.Image
	out.Volumes = *(*map[string]struct{})(unsafe.Pointer(&in.Volumes))
	out.VolumesFrom = in.VolumesFrom
	out.WorkingDir = in.WorkingDir
	out.Entrypoint = *(*[]string)(unsafe.Pointer(&in.Entrypoint))
	out.NetworkDisabled = in.NetworkDisabled
	out.SecurityOpts = *(*[]string)(unsafe.Pointer(&in.SecurityOpts))
	out.OnBuild = *(*[]string)(unsafe.Pointer(&in.OnBuild))
	out.Labels = *(*map[string]string)(unsafe.Pointer(&in.Labels))
	return nil
}

// Convert_docker10_DockerConfig_To_image_DockerConfig is an autogenerated conversion function.
func Convert_docker10_DockerConfig_To_image_DockerConfig(in *imagedocker10.DockerConfig, out *image.DockerConfig, s conversion.Scope) error {
	return autoConvert_docker10_DockerConfig_To_image_DockerConfig(in, out, s)
}

func autoConvert_image_DockerConfig_To_docker10_DockerConfig(in *image.DockerConfig, out *imagedocker10.DockerConfig, s conversion.Scope) error {
	out.Hostname = in.Hostname
	out.Domainname = in.Domainname
	out.User = in.User
	out.Memory = in.Memory
	out.MemorySwap = in.MemorySwap
	out.CPUShares = in.CPUShares
	out.CPUSet = in.CPUSet
	out.AttachStdin = in.AttachStdin
	out.AttachStdout = in.AttachStdout
	out.AttachStderr = in.AttachStderr
	out.PortSpecs = *(*[]string)(unsafe.Pointer(&in.PortSpecs))
	out.ExposedPorts = *(*map[string]struct{})(unsafe.Pointer(&in.ExposedPorts))
	out.Tty = in.Tty
	out.OpenStdin = in.OpenStdin
	out.StdinOnce = in.StdinOnce
	out.Env = *(*[]string)(unsafe.Pointer(&in.Env))
	out.Cmd = *(*[]string)(unsafe.Pointer(&in.Cmd))
	out.DNS = *(*[]string)(unsafe.Pointer(&in.DNS))
	out.Image = in.Image
	out.Volumes = *(*map[string]struct{})(unsafe.Pointer(&in.Volumes))
	out.VolumesFrom = in.VolumesFrom
	out.WorkingDir = in.WorkingDir
	out.Entrypoint = *(*[]string)(unsafe.Pointer(&in.Entrypoint))
	out.NetworkDisabled = in.NetworkDisabled
	out.SecurityOpts = *(*[]string)(unsafe.Pointer(&in.SecurityOpts))
	out.OnBuild = *(*[]string)(unsafe.Pointer(&in.OnBuild))
	out.Labels = *(*map[string]string)(unsafe.Pointer(&in.Labels))
	return nil
}

// Convert_image_DockerConfig_To_docker10_DockerConfig is an autogenerated conversion function.
func Convert_image_DockerConfig_To_docker10_DockerConfig(in *image.DockerConfig, out *imagedocker10.DockerConfig, s conversion.Scope) error {
	return autoConvert_image_DockerConfig_To_docker10_DockerConfig(in, out, s)
}

func autoConvert_docker10_DockerImage_To_image_DockerImage(in *imagedocker10.DockerImage, out *image.DockerImage, s conversion.Scope) error {
	out.ID = in.ID
	out.Parent = in.Parent
	out.Comment = in.Comment
	out.Created = in.Created
	out.Container = in.Container
	if err := Convert_docker10_DockerConfig_To_image_DockerConfig(&in.ContainerConfig, &out.ContainerConfig, s); err != nil {
		return err
	}
	out.DockerVersion = in.DockerVersion
	out.Author = in.Author
	out.Config = (*image.DockerConfig)(unsafe.Pointer(in.Config))
	out.Architecture = in.Architecture
	out.Size = in.Size
	return nil
}

// Convert_docker10_DockerImage_To_image_DockerImage is an autogenerated conversion function.
func Convert_docker10_DockerImage_To_image_DockerImage(in *imagedocker10.DockerImage, out *image.DockerImage, s conversion.Scope) error {
	return autoConvert_docker10_DockerImage_To_image_DockerImage(in, out, s)
}

func autoConvert_image_DockerImage_To_docker10_DockerImage(in *image.DockerImage, out *imagedocker10.DockerImage, s conversion.Scope) error {
	out.ID = in.ID
	out.Parent = in.Parent
	out.Comment = in.Comment
	out.Created = in.Created
	out.Container = in.Container
	if err := Convert_image_DockerConfig_To_docker10_DockerConfig(&in.ContainerConfig, &out.ContainerConfig, s); err != nil {
		return err
	}
	out.DockerVersion = in.DockerVersion
	out.Author = in.Author
	out.Config = (*imagedocker10.DockerConfig)(unsafe.Pointer(in.Config))
	out.Architecture = in.Architecture
	out.Size = in.Size
	return nil
}

// Convert_image_DockerImage_To_docker10_DockerImage is an autogenerated conversion function.
func Convert_image_DockerImage_To_docker10_DockerImage(in *image.DockerImage, out *imagedocker10.DockerImage, s conversion.Scope) error {
	return autoConvert_image_DockerImage_To_docker10_DockerImage(in, out, s)
}
