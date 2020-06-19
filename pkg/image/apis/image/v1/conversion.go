package v1

import (
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/openshift/api/image/docker10"
	"github.com/openshift/api/image/dockerpre012"
	v1 "github.com/openshift/api/image/v1"
	"github.com/openshift/library-go/pkg/image/reference"
	newer "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
)

var (
	dockerImageScheme = runtime.NewScheme()
	dockerImageCodecs = serializer.NewCodecFactory(dockerImageScheme)
)

func init() {
	docker10.AddToSchemeInCoreGroup(dockerImageScheme)
	dockerpre012.AddToSchemeInCoreGroup(dockerImageScheme)
	Install(dockerImageScheme)
}

// The docker metadata must be cast to a version
func Convert_image_Image_To_v1_Image(in *newer.Image, out *v1.Image, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.DockerImageReference = in.DockerImageReference
	out.DockerImageManifest = in.DockerImageManifest
	out.DockerImageManifestMediaType = in.DockerImageManifestMediaType
	out.DockerImageConfig = in.DockerImageConfig

	gvString := in.DockerImageMetadataVersion
	if len(gvString) == 0 {
		gvString = "1.0"
	}
	if !strings.Contains(gvString, "/") {
		gvString = "/" + gvString
	}

	version, err := schema.ParseGroupVersion(gvString)
	if err != nil {
		return err
	}
	data, err := runtime.Encode(dockerImageCodecs.LegacyCodec(version), &in.DockerImageMetadata)
	if err != nil {
		return err
	}
	out.DockerImageMetadata.Raw = data
	out.DockerImageMetadataVersion = version.Version

	if in.DockerImageLayers != nil {
		out.DockerImageLayers = make([]v1.ImageLayer, len(in.DockerImageLayers))
		for i := range in.DockerImageLayers {
			out.DockerImageLayers[i].MediaType = in.DockerImageLayers[i].MediaType
			out.DockerImageLayers[i].Name = in.DockerImageLayers[i].Name
			out.DockerImageLayers[i].LayerSize = in.DockerImageLayers[i].LayerSize
		}
	} else {
		out.DockerImageLayers = nil
	}

	if in.Signatures != nil {
		out.Signatures = make([]v1.ImageSignature, len(in.Signatures))
		for i := range in.Signatures {
			if err := s.Convert(&in.Signatures[i], &out.Signatures[i], 0); err != nil {
				return err
			}
		}
	} else {
		out.Signatures = nil
	}

	if in.DockerImageSignatures != nil {
		out.DockerImageSignatures = make([][]byte, 0, len(in.DockerImageSignatures))
		out.DockerImageSignatures = append(out.DockerImageSignatures, in.DockerImageSignatures...)
	} else {
		out.DockerImageSignatures = nil
	}

	return nil
}

func Convert_v1_Image_To_image_Image(in *v1.Image, out *newer.Image, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.DockerImageReference = in.DockerImageReference
	out.DockerImageManifest = in.DockerImageManifest
	out.DockerImageManifestMediaType = in.DockerImageManifestMediaType
	out.DockerImageConfig = in.DockerImageConfig

	version := in.DockerImageMetadataVersion
	if len(version) == 0 {
		version = "1.0"
	}
	if len(in.DockerImageMetadata.Raw) > 0 {
		// TODO: add a way to default the expected kind and version of an object if not set
		obj, err := dockerImageScheme.New(schema.GroupVersionKind{Version: version, Kind: "DockerImage"})
		if err != nil {
			return err
		}
		if err := runtime.DecodeInto(dockerImageCodecs.UniversalDecoder(), in.DockerImageMetadata.Raw, obj); err != nil {
			return err
		}
		if err := s.Convert(obj, &out.DockerImageMetadata, 0); err != nil {
			return err
		}
	}
	out.DockerImageMetadataVersion = version

	if in.DockerImageLayers != nil {
		out.DockerImageLayers = make([]newer.ImageLayer, len(in.DockerImageLayers))
		for i := range in.DockerImageLayers {
			out.DockerImageLayers[i].MediaType = in.DockerImageLayers[i].MediaType
			out.DockerImageLayers[i].Name = in.DockerImageLayers[i].Name
			out.DockerImageLayers[i].LayerSize = in.DockerImageLayers[i].LayerSize
		}
	} else {
		out.DockerImageLayers = nil
	}

	if in.Signatures != nil {
		out.Signatures = make([]newer.ImageSignature, len(in.Signatures))
		for i := range in.Signatures {
			if err := s.Convert(&in.Signatures[i], &out.Signatures[i], 0); err != nil {
				return err
			}
		}
	} else {
		out.Signatures = nil
	}

	if in.DockerImageSignatures != nil {
		out.DockerImageSignatures = nil
		for _, v := range in.DockerImageSignatures {
			out.DockerImageSignatures = append(out.DockerImageSignatures, v)
		}
	} else {
		out.DockerImageSignatures = nil
	}

	return nil
}

func Convert_v1_ImageStreamSpec_To_image_ImageStreamSpec(in *v1.ImageStreamSpec, out *newer.ImageStreamSpec, s conversion.Scope) error {
	out.LookupPolicy = newer.ImageLookupPolicy{Local: in.LookupPolicy.Local}
	out.DockerImageRepository = in.DockerImageRepository
	out.Tags = make(map[string]newer.TagReference)
	return s.Convert(&in.Tags, &out.Tags, 0)
}

func Convert_image_ImageStreamSpec_To_v1_ImageStreamSpec(in *newer.ImageStreamSpec, out *v1.ImageStreamSpec, s conversion.Scope) error {
	out.LookupPolicy = v1.ImageLookupPolicy{Local: in.LookupPolicy.Local}
	out.DockerImageRepository = in.DockerImageRepository
	if len(in.DockerImageRepository) > 0 {
		// ensure that stored image references have no tag or ID, which was possible from 1.0.0 until 1.0.7
		if ref, err := reference.Parse(in.DockerImageRepository); err == nil {
			if len(ref.Tag) > 0 || len(ref.ID) > 0 {
				ref.Tag, ref.ID = "", ""
				out.DockerImageRepository = ref.Exact()
			}
		}
	}
	out.Tags = make([]v1.TagReference, 0, 0)
	return s.Convert(&in.Tags, &out.Tags, 0)
}

func Convert_v1_ImageStreamStatus_To_image_ImageStreamStatus(in *v1.ImageStreamStatus, out *newer.ImageStreamStatus, s conversion.Scope) error {
	out.DockerImageRepository = in.DockerImageRepository
	out.PublicDockerImageRepository = in.PublicDockerImageRepository
	out.Tags = make(map[string]newer.TagEventList)
	return s.Convert(&in.Tags, &out.Tags, 0)
}

func Convert_image_ImageStreamStatus_To_v1_ImageStreamStatus(in *newer.ImageStreamStatus, out *v1.ImageStreamStatus, s conversion.Scope) error {
	out.DockerImageRepository = in.DockerImageRepository
	out.PublicDockerImageRepository = in.PublicDockerImageRepository
	if len(in.DockerImageRepository) > 0 {
		// ensure that stored image references have no tag or ID, which was possible from 1.0.0 until 1.0.7
		if ref, err := reference.Parse(in.DockerImageRepository); err == nil {
			if len(ref.Tag) > 0 || len(ref.ID) > 0 {
				ref.Tag, ref.ID = "", ""
				out.DockerImageRepository = ref.Exact()
			}
		}
	}
	out.Tags = make([]v1.NamedTagEventList, 0, 0)
	return s.Convert(&in.Tags, &out.Tags, 0)
}

func Convert_v1_NamedTagEventListArray_to_api_TagEventListArray(in *[]v1.NamedTagEventList, out *map[string]newer.TagEventList, s conversion.Scope) error {
	for _, curr := range *in {
		newTagEventList := newer.TagEventList{}
		if err := s.Convert(&curr.Conditions, &newTagEventList.Conditions, 0); err != nil {
			return err
		}
		if err := s.Convert(&curr.Items, &newTagEventList.Items, 0); err != nil {
			return err
		}
		(*out)[curr.Tag] = newTagEventList
	}

	return nil
}
func Convert_image_TagEventListArray_to_v1_NamedTagEventListArray(in *map[string]newer.TagEventList, out *[]v1.NamedTagEventList, s conversion.Scope) error {
	allKeys := make([]string, 0, len(*in))
	for key := range *in {
		allKeys = append(allKeys, key)
	}
	sort.Strings(allKeys)

	for _, key := range allKeys {
		newTagEventList := (*in)[key]
		oldTagEventList := &v1.NamedTagEventList{Tag: key}
		if err := s.Convert(&newTagEventList.Conditions, &oldTagEventList.Conditions, 0); err != nil {
			return err
		}
		if err := s.Convert(&newTagEventList.Items, &oldTagEventList.Items, 0); err != nil {
			return err
		}

		*out = append(*out, *oldTagEventList)
	}

	return nil
}
func Convert_v1_TagReferenceArray_to_api_TagReferenceMap(in *[]v1.TagReference, out *map[string]newer.TagReference, s conversion.Scope) error {
	for _, curr := range *in {
		r := newer.TagReference{}
		if err := s.Convert(&curr, &r, 0); err != nil {
			return err
		}
		(*out)[curr.Name] = r
	}
	return nil
}
func Convert_image_TagReferenceMap_to_v1_TagReferenceArray(in *map[string]newer.TagReference, out *[]v1.TagReference, s conversion.Scope) error {
	allTags := make([]string, 0, len(*in))
	for tag := range *in {
		allTags = append(allTags, tag)
	}
	sort.Strings(allTags)

	for _, tag := range allTags {
		newTagReference := (*in)[tag]
		oldTagReference := v1.TagReference{}
		if err := s.Convert(&newTagReference, &oldTagReference, 0); err != nil {
			return err
		}
		oldTagReference.Name = tag
		*out = append(*out, oldTagReference)
	}
	return nil
}

func Convert_dockerpre012_DockerConfig_to_docker10_DockerConfig(in *dockerpre012.DockerConfig, out *docker10.DockerConfig, s conversion.Scope) error {
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
	out.PortSpecs = in.PortSpecs
	out.ExposedPorts = in.ExposedPorts
	out.Tty = in.Tty
	out.OpenStdin = in.OpenStdin
	out.StdinOnce = in.StdinOnce
	out.Env = in.Env
	out.Cmd = in.Cmd
	out.DNS = in.DNS
	out.Image = in.Image
	out.Volumes = in.Volumes
	out.VolumesFrom = in.VolumesFrom
	out.WorkingDir = in.WorkingDir
	out.Entrypoint = in.Entrypoint
	out.NetworkDisabled = in.NetworkDisabled
	out.SecurityOpts = in.SecurityOpts
	out.OnBuild = in.OnBuild
	out.Labels = in.Labels
	return nil
}

func Convert_docker10_DockerConfig_to_dockerpre012_DockerConfig(in *docker10.DockerConfig, out *dockerpre012.DockerConfig, s conversion.Scope) error {
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
	out.PortSpecs = in.PortSpecs
	out.ExposedPorts = in.ExposedPorts
	out.Tty = in.Tty
	out.OpenStdin = in.OpenStdin
	out.StdinOnce = in.StdinOnce
	out.Env = in.Env
	out.Cmd = in.Cmd
	out.DNS = in.DNS
	out.Image = in.Image
	out.Volumes = in.Volumes
	out.VolumesFrom = in.VolumesFrom
	out.WorkingDir = in.WorkingDir
	out.Entrypoint = in.Entrypoint
	out.NetworkDisabled = in.NetworkDisabled
	out.SecurityOpts = in.SecurityOpts
	out.OnBuild = in.OnBuild
	out.Labels = in.Labels
	return nil
}

func Convert_docker10_DockerImage_to_dockerpre012_DockerImage(in *docker10.DockerImage, out *dockerpre012.DockerImage, s conversion.Scope) error {
	out.ID = in.ID
	out.Parent = in.Parent
	out.Comment = in.Comment
	out.Created = in.Created
	out.Container = in.Container
	out.DockerVersion = in.DockerVersion
	out.Author = in.Author
	out.Architecture = in.Architecture
	out.Size = in.Size

	if err := s.Convert(&in.ContainerConfig, &out.ContainerConfig, 0); err != nil {
		return err
	}

	if in.Config != nil {
		out.Config = &dockerpre012.DockerConfig{}
		if err := s.Convert(in.Config, out.Config, 0); err != nil {
			return err
		}
	}

	return nil
}

func Convert_dockerpre012_DockerImage_to_docker10_DockerImage(in *dockerpre012.DockerImage, out *docker10.DockerImage, s conversion.Scope) error {
	out.ID = in.ID
	out.Parent = in.Parent
	out.Comment = in.Comment
	out.Created = in.Created
	out.Container = in.Container
	out.DockerVersion = in.DockerVersion
	out.Author = in.Author
	out.Architecture = in.Architecture
	out.Size = in.Size

	if err := s.Convert(&in.ContainerConfig, &out.ContainerConfig, 0); err != nil {
		return err
	}

	if in.Config != nil {
		out.Config = &docker10.DockerConfig{}
		if err := s.Convert(in.Config, out.Config, 0); err != nil {
			return err
		}
	}

	return nil
}

func AddConversionFuncs(s *runtime.Scheme) error {
	if err := s.AddConversionFunc((*[]v1.NamedTagEventList)(nil), (*map[string]newer.TagEventList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_NamedTagEventListArray_to_api_TagEventListArray(a.(*[]v1.NamedTagEventList), b.(*map[string]newer.TagEventList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*map[string]newer.TagEventList)(nil), (*[]v1.NamedTagEventList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_image_TagEventListArray_to_v1_NamedTagEventListArray(a.(*map[string]newer.TagEventList), b.(*[]v1.NamedTagEventList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*[]v1.TagReference)(nil), (*map[string]newer.TagReference)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_TagReferenceArray_to_api_TagReferenceMap(a.(*[]v1.TagReference), b.(*map[string]newer.TagReference), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*map[string]newer.TagReference)(nil), (*[]v1.TagReference)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_image_TagReferenceMap_to_v1_TagReferenceArray(a.(*map[string]newer.TagReference), b.(*[]v1.TagReference), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*docker10.DockerConfig)(nil), (*dockerpre012.DockerConfig)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_docker10_DockerConfig_to_dockerpre012_DockerConfig(a.(*docker10.DockerConfig), b.(*dockerpre012.DockerConfig), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*dockerpre012.DockerConfig)(nil), (*docker10.DockerConfig)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_dockerpre012_DockerConfig_to_docker10_DockerConfig(a.(*dockerpre012.DockerConfig), b.(*docker10.DockerConfig), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*docker10.DockerImage)(nil), (*dockerpre012.DockerImage)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_docker10_DockerImage_to_dockerpre012_DockerImage(a.(*docker10.DockerImage), b.(*dockerpre012.DockerImage), scope)
	}); err != nil {
		return err
	}
	if err := s.AddConversionFunc((*dockerpre012.DockerImage)(nil), (*docker10.DockerImage)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_dockerpre012_DockerImage_to_docker10_DockerImage(a.(*dockerpre012.DockerImage), b.(*docker10.DockerImage), scope)
	}); err != nil {
		return err
	}
	return nil
}

func addFieldSelectorKeyConversions(scheme *runtime.Scheme) error {
	if err := scheme.AddFieldLabelConversionFunc(v1.GroupVersion.WithKind("ImageStream"), imageStreamFieldSelectorKeyConversionFunc); err != nil {
		return err
	}
	return nil
}

// because field selectors can vary in support by version they are exposed under, we have one function for each
// groupVersion we're registering for

func imageStreamFieldSelectorKeyConversionFunc(label, value string) (internalLabel, internalValue string, err error) {
	switch label {
	case "spec.dockerImageRepository",
		"status.dockerImageRepository":
		return label, value, nil
	default:
		return runtime.DefaultMetaV1FieldSelectorConversion(label, value)
	}
}
