package validation

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/distribution/reference"

	kmeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/validation/path"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	kapi "k8s.io/kubernetes/pkg/apis/core"
	kapihelper "k8s.io/kubernetes/pkg/apis/core/helper"
	"k8s.io/kubernetes/pkg/apis/core/validation"

	imagev1 "github.com/openshift/api/image/v1"
	"github.com/openshift/library-go/pkg/image/imageutil"
	imageref "github.com/openshift/library-go/pkg/image/reference"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation/whitelist"
)

// RepositoryNameComponentRegexp restricts registry path component names to
// start with at least one letter or number, with following parts able to
// be separated by one period, dash or underscore.
// Copied from github.com/distribution/distribution/v3/registry/api/v2/names.go v2.1.1
var RepositoryNameComponentRegexp = regexp.MustCompile(`[a-z0-9]+(?:[._-][a-z0-9]+)*`)

// RepositoryNameComponentAnchoredRegexp is the version of
// RepositoryNameComponentRegexp which must completely match the content
// Copied from github.com/distribution/distribution/v3/registry/api/v2/names.go v2.1.1
var RepositoryNameComponentAnchoredRegexp = regexp.MustCompile(`^` + RepositoryNameComponentRegexp.String() + `$`)

// RepositoryNameRegexp builds on RepositoryNameComponentRegexp to allow
// multiple path components, separated by a forward slash.
// Copied from github.com/distribution/distribution/v3/registry/api/v2/names.go v2.1.1
var RepositoryNameRegexp = regexp.MustCompile(`(?:` + RepositoryNameComponentRegexp.String() + `/)*` + RepositoryNameComponentRegexp.String())

var imageDigestRegexp = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][[:xdigit:]]{32,}`)
var imageMediaTypeRegexp = regexp.MustCompile(`^ *([A-Za-z0-9][A-Za-z0-9!#$&^_-]{0,126})\/([A-Za-z0-9][A-Za-z0-9!#$&^_.+-]{0,126}) *$`)

func ValidateImageStreamName(name string, prefix bool) []string {
	if reasons := path.ValidatePathSegmentName(name, prefix); len(reasons) != 0 {
		return reasons
	}

	if !RepositoryNameComponentAnchoredRegexp.MatchString(name) {
		return []string{fmt.Sprintf("must match %q", RepositoryNameComponentRegexp.String())}
	}
	return nil
}

// ValidateImage tests required fields for an Image.
func ValidateImage(image *imageapi.Image) field.ErrorList {
	return validateImage(image, nil)
}

func validateImage(image *imageapi.Image, fldPath *field.Path) field.ErrorList {
	result := validation.ValidateObjectMeta(&image.ObjectMeta, false, path.ValidatePathSegmentName, fldPath.Child("metadata"))

	if len(image.DockerImageReference) == 0 {
		result = append(result, field.Required(fldPath.Child("dockerImageReference"), ""))
	} else {
		if _, err := imageref.Parse(image.DockerImageReference); err != nil {
			result = append(result, field.Invalid(fldPath.Child("dockerImageReference"), image.DockerImageReference, err.Error()))
		}
	}

	for i, sig := range image.Signatures {
		result = append(result, validateImageSignature(&sig, fldPath.Child("signatures").Index(i))...)
	}

	if len(image.DockerImageManifests) > 0 {
		result = append(
			result,
			validateImageManifests(image.DockerImageManifests, fldPath.Child("dockerImageManifests"))...,
		)
		if len(image.DockerImageLayers) > 0 {
			result = append(
				result,
				field.Invalid(
					fldPath.Child("dockerImageLayers"),
					image.DockerImageLayers,
					"dockerImageLayers should not be set when dockerImageManifests is set",
				),
			)
		}
	}

	return result
}

func validateImageManifests(imageManifests []imageapi.ImageManifest, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList

	for i, m := range imageManifests {
		if valid := imageDigestRegexp.MatchString(m.Digest); !valid {
			msg := "digest does not conform with OCI image specification"
			errs = append(
				errs,
				field.Invalid(
					fldPath.Index(i).Child("digest"),
					m.Digest,
					msg,
				),
			)
		}

		if valid := imageMediaTypeRegexp.MatchString(m.MediaType); !valid {
			errs = append(
				errs,
				field.Invalid(
					fldPath.Index(i).Child("mediaType"),
					m.MediaType,
					"media type does not conform to RFC6838",
				),
			)
		}

		if len(m.Architecture) == 0 {
			errs = append(
				errs,
				field.Required(
					fldPath.Index(i).Child("architecture"),
					m.Architecture,
				),
			)
		}

		if len(m.OS) == 0 {
			errs = append(
				errs,
				field.Required(
					fldPath.Index(i).Child("os"),
					m.OS,
				),
			)
		}

		if m.ManifestSize < 0 {
			errs = append(
				errs,
				field.Invalid(
					fldPath.Index(i).Child("size"),
					m.ManifestSize,
					"manifest size cannot be negative",
				),
			)
		}
	}

	return errs
}

func ValidateImageUpdate(newImage, oldImage *imageapi.Image) field.ErrorList {
	result := validation.ValidateObjectMetaUpdate(&newImage.ObjectMeta, &oldImage.ObjectMeta, field.NewPath("metadata"))
	result = append(result, ValidateImage(newImage)...)

	return result
}

// ValidateImageSignature ensures that given signatures is valid.
func ValidateImageSignature(signature *imageapi.ImageSignature) field.ErrorList {
	return validateImageSignature(signature, nil)
}

// splitImageSignatureName splits given signature name into image name and signature name.
func splitImageSignatureName(imageSignatureName string) (imageName, signatureName string, err error) {
	segments := strings.Split(imageSignatureName, "@")
	switch len(segments) {
	case 2:
		signatureName = segments[1]
		imageName = segments[0]
		if len(imageName) == 0 || len(signatureName) == 0 {
			err = fmt.Errorf("image signature name %q must have an image name and signature name", imageSignatureName)
		}
	default:
		err = fmt.Errorf("expected exactly one @ in the image signature name %q", imageSignatureName)
	}
	return
}

func validateImageSignature(signature *imageapi.ImageSignature, fldPath *field.Path) field.ErrorList {
	allErrs := validation.ValidateObjectMeta(&signature.ObjectMeta, false, path.ValidatePathSegmentName, fldPath.Child("metadata"))
	if len(signature.Labels) > 0 {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("metadata").Child("labels"), "signature labels cannot be set"))
	}
	if len(signature.Annotations) > 0 {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("metadata").Child("annotations"), "signature annotations cannot be set"))
	}

	if _, _, err := splitImageSignatureName(signature.Name); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("metadata").Child("name"), signature.Name, "name must be of format <imageName>@<signatureName>"))
	}
	if len(signature.Type) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("type"), ""))
	}
	if len(signature.Content) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("content"), ""))
	}

	var trustedCondition, forImageCondition *imageapi.SignatureCondition
	for i := range signature.Conditions {
		cond := &signature.Conditions[i]
		if cond.Type == imageapi.SignatureTrusted && (trustedCondition == nil || !cond.LastProbeTime.Before(&trustedCondition.LastProbeTime)) {
			trustedCondition = cond
		} else if cond.Type == imageapi.SignatureForImage && forImageCondition == nil || !cond.LastProbeTime.Before(&forImageCondition.LastProbeTime) {
			forImageCondition = cond
		}
	}

	if trustedCondition != nil && forImageCondition == nil {
		msg := fmt.Sprintf("missing %q condition type", imageapi.SignatureForImage)
		allErrs = append(allErrs, field.Invalid(fldPath.Child("conditions"), signature.Conditions, msg))
	} else if forImageCondition != nil && trustedCondition == nil {
		msg := fmt.Sprintf("missing %q condition type", imageapi.SignatureTrusted)
		allErrs = append(allErrs, field.Invalid(fldPath.Child("conditions"), signature.Conditions, msg))
	}

	if trustedCondition == nil || trustedCondition.Status == kapi.ConditionUnknown {
		if len(signature.ImageIdentity) != 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("imageIdentity"), signature.ImageIdentity, "must be unset for unknown signature state"))
		}
		if len(signature.SignedClaims) != 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("signedClaims"), signature.SignedClaims, "must be unset for unknown signature state"))
		}
		if signature.IssuedBy != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("issuedBy"), signature.IssuedBy, "must be unset for unknown signature state"))
		}
		if signature.IssuedTo != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("issuedTo"), signature.IssuedTo, "must be unset for unknown signature state"))
		}
	}

	return allErrs
}

// ValidateImageSignatureUpdate ensures that the new ImageSignature is valid.
func ValidateImageSignatureUpdate(newImageSignature, oldImageSignature *imageapi.ImageSignature) field.ErrorList {
	allErrs := validation.ValidateObjectMetaUpdate(&newImageSignature.ObjectMeta, &oldImageSignature.ObjectMeta, field.NewPath("metadata"))
	allErrs = append(allErrs, ValidateImageSignature(newImageSignature)...)

	if newImageSignature.Type != oldImageSignature.Type {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("type"), "cannot change signature type"))
	}
	if !bytes.Equal(newImageSignature.Content, oldImageSignature.Content) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("content"), "cannot change signature content"))
	}

	return allErrs
}

func ValidateImportPolicy(importPolicy imageapi.TagImportPolicy, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList

	im := importPolicy.ImportMode
	if len(im) != 0 && im != imageapi.ImportModeLegacy && im != imageapi.ImportModePreserveOriginal {
		msg := fmt.Sprintf(
			"invalid import mode, valid modes are '', '%s', '%s'",
			imageapi.ImportModeLegacy,
			imageapi.ImportModePreserveOriginal,
		)
		errs = append(errs, field.Invalid(fldPath.Child("importMode"), importPolicy.ImportMode, msg))
	}

	return errs
}

// ValidateImageStream tests required fields for an ImageStream.
func ValidateImageStream(stream *imageapi.ImageStream) field.ErrorList {
	// "normal" object validators aren't supposed to be querying the API server so passing
	// context is not expected, but we are special. Don't try to wire it up.
	return ValidateImageStreamWithWhitelister(context.TODO(), nil, stream)
}

// ValidateImageStreamWithWhitelister tests required fields for an ImageStream. Additionally, it validates
// each new image reference against registry whitelist.
func ValidateImageStreamWithWhitelister(
	ctx context.Context,
	whitelister whitelist.RegistryWhitelister,
	stream *imageapi.ImageStream,
) field.ErrorList {
	result := validation.ValidateObjectMeta(&stream.ObjectMeta, true, ValidateImageStreamName, field.NewPath("metadata"))

	// Ensure we can generate a valid container image repository from namespace/name
	if len(stream.Namespace+"/"+stream.Name) > reference.NameTotalLengthMax {
		result = append(result, field.Invalid(field.NewPath("metadata", "name"), stream.Name, fmt.Sprintf("'namespace/name' cannot be longer than %d characters", reference.NameTotalLengthMax)))
	}

	insecureRepository := isRepositoryInsecure(stream)

	if len(stream.Spec.DockerImageRepository) != 0 {
		dockerImageRepositoryPath := field.NewPath("spec", "dockerImageRepository")
		isValid := true
		ref, err := imageref.Parse(stream.Spec.DockerImageRepository)
		if err != nil {
			result = append(result, field.Invalid(dockerImageRepositoryPath, stream.Spec.DockerImageRepository, err.Error()))
			isValid = false
		} else {
			if len(ref.Tag) > 0 {
				result = append(result, field.Invalid(dockerImageRepositoryPath, stream.Spec.DockerImageRepository, "the repository name may not contain a tag"))
				isValid = false
			}
			if len(ref.ID) > 0 {
				result = append(result, field.Invalid(dockerImageRepositoryPath, stream.Spec.DockerImageRepository, "the repository name may not contain an ID"))
				isValid = false
			}
		}
		if isValid && whitelister != nil {
			if err := whitelister.AdmitDockerImageReference(ctx, ref, getWhitelistTransportForFlag(insecureRepository, true)); err != nil {
				result = append(result, field.Forbidden(dockerImageRepositoryPath, err.Error()))
			}
		}
	}
	for tag, tagRef := range stream.Spec.Tags {
		path := field.NewPath("spec", "tags").Key(tag)
		result = append(result, ValidateImageStreamTagReference(ctx, whitelister, insecureRepository, tagRef, path)...)
	}
	for tag, history := range stream.Status.Tags {
		for i, tagEvent := range history.Items {
			if len(tagEvent.DockerImageReference) == 0 {
				result = append(result, field.Required(field.NewPath("status", "tags").Key(tag).Child("items").Index(i).Child("dockerImageReference"), ""))
				continue
			}
			ref, err := imageref.Parse(tagEvent.DockerImageReference)
			if err != nil {
				result = append(result, field.Invalid(field.NewPath("status", "tags").Key(tag).Child("items").Index(i).Child("dockerImageReference"), tagEvent.DockerImageReference, err.Error()))
				continue
			}
			if whitelister != nil {
				insecure := false
				if tr, ok := stream.Spec.Tags[tag]; ok {
					insecure = tr.ImportPolicy.Insecure
				}
				transport := getWhitelistTransportForFlag(insecure || insecureRepository, true)
				if err := whitelister.AdmitDockerImageReference(ctx, ref, transport); err != nil {
					result = append(result, field.Forbidden(field.NewPath("status", "tags").Key(tag).Child("items").Index(i).Child("dockerImageReference"), err.Error()))
				}
			}
		}
	}

	return result
}

// ValidateImageStreamTagReference ensures that a given tag reference is valid.
func ValidateImageStreamTagReference(
	ctx context.Context,
	whitelister whitelist.RegistryWhitelister,
	insecureRepository bool,
	tagRef imageapi.TagReference,
	fldPath *field.Path,
) field.ErrorList {
	var errs = field.ErrorList{}

	if tagRef.From != nil {
		if len(tagRef.From.Name) == 0 {
			errs = append(errs, field.Required(fldPath.Child("from", "name"), ""))
		}
		switch tagRef.From.Kind {
		case "DockerImage":
			if ref, err := imageref.Parse(tagRef.From.Name); err != nil && len(tagRef.From.Name) > 0 {
				errs = append(errs, field.Invalid(fldPath.Child("from", "name"), tagRef.From.Name, err.Error()))
			} else if whitelister != nil {
				transport := getWhitelistTransportForFlag(tagRef.ImportPolicy.Insecure || insecureRepository, true)

				if err := whitelister.AdmitDockerImageReference(ctx, ref, transport); err != nil {
					errs = append(errs, field.Forbidden(fldPath.Child("from", "name"), err.Error()))
				}
			}
		case "ImageStreamImage", "ImageStreamTag":
			if tagRef.ImportPolicy.Scheduled {
				errs = append(errs, field.Invalid(fldPath.Child("importPolicy", "scheduled"), tagRef.ImportPolicy.Scheduled, "only tags pointing to Docker repositories may be scheduled for background import"))
			}
		default:
			errs = append(errs, field.Required(fldPath.Child("from", "kind"), "valid values are 'DockerImage', 'ImageStreamImage', 'ImageStreamTag'"))
		}
	}
	switch tagRef.ReferencePolicy.Type {
	case imageapi.SourceTagReferencePolicy, imageapi.LocalTagReferencePolicy:
	default:
		errs = append(errs, field.Invalid(fldPath.Child("referencePolicy", "type"), tagRef.ReferencePolicy.Type, fmt.Sprintf("valid values are %q, %q", imageapi.SourceTagReferencePolicy, imageapi.LocalTagReferencePolicy)))
	}

	errs = append(errs, ValidateImportPolicy(tagRef.ImportPolicy, fldPath.Child("importPolicy"))...)

	return errs
}

// ValidateImageStreamUpdate tests required fields for an ImageStream update.
func ValidateImageStreamUpdate(newStream, oldStream *imageapi.ImageStream) field.ErrorList {
	// "normal" object validators aren't supposed to be querying the API server so passing
	// context is not expected, but we are special. Don't try to wire it up.
	return ValidateImageStreamUpdateWithWhitelister(context.TODO(), nil, newStream, oldStream)
}

// ValidateImageStreamUpdateWithWhitelister tests required fields for an ImageStream update. Additionally, it
// validates each new image reference against registry whitelist.
func ValidateImageStreamUpdateWithWhitelister(
	ctx context.Context,
	whitelister whitelist.RegistryWhitelister,
	newStream, oldStream *imageapi.ImageStream,
) field.ErrorList {
	result := validation.ValidateObjectMetaUpdate(&newStream.ObjectMeta, &oldStream.ObjectMeta, field.NewPath("metadata"))

	if whitelister != nil {
		// whitelist old pull specs no longer present on the whitelist
		whitelister = whitelister.Copy()
		for pullSpec := range collectImageStreamSpecImageReferences(oldStream) {
			err := whitelister.WhitelistRepository(pullSpec)
			if err != nil {
				// We should allow users to delete invalid image references
				// from image streams, therefore we cannot return this error.
				klog.V(4).Infof("image stream %s/%s: unable to add pull spec %q to whitelist: %s", oldStream.Namespace, oldStream.Name, pullSpec, err)
				continue
			}
		}
		for pullSpec := range collectImageStreamStatusImageReferences(oldStream) {
			err := whitelister.WhitelistRepository(pullSpec)
			if err != nil {
				klog.V(4).Infof("image stream %s/%s: unable to add pull spec %q to whitelist: %s", oldStream.Namespace, oldStream.Name, pullSpec, err)
				continue
			}
		}
	}

	result = append(result, ValidateImageStreamWithWhitelister(ctx, whitelister, newStream)...)

	return result
}

// ValidateImageStreamStatusUpdateWithWhitelister tests required fields for an ImageStream status update.
// Additionally, it validates each new image reference against registry whitelist.
func ValidateImageStreamStatusUpdateWithWhitelister(
	ctx context.Context,
	whitelister whitelist.RegistryWhitelister,
	newStream, oldStream *imageapi.ImageStream,
) field.ErrorList {
	errs := validation.ValidateObjectMetaUpdate(&newStream.ObjectMeta, &oldStream.ObjectMeta, field.NewPath("metadata"))
	insecureRepository := isRepositoryInsecure(newStream)

	oldRefs := collectImageStreamStatusImageReferences(oldStream)
	newRefs := collectImageStreamStatusImageReferences(newStream)

	for refString, rfs := range newRefs {
		// allow to manipulate not whitelisted references if already present in the image stream
		if _, ok := oldRefs[refString]; ok {
			continue
		}
		ref, err := imageref.Parse(refString)
		if err != nil {
			for _, rf := range rfs {
				errs = append(errs, field.Invalid(rf.path, refString, err.Error()))
			}
			continue
		}

		if whitelister == nil {
			continue
		}

		insecure := insecureRepository
		if !insecure {
			for _, rf := range rfs {
				if rf.insecure {
					insecure = true
					break
				}
			}
		}
		transport := getWhitelistTransportForFlag(insecure, true)
		if err := whitelister.AdmitDockerImageReference(ctx, ref, transport); err != nil {
			// TODO: should we whitelist references imported based on whitelisted/old spec?
			// report error for each tag/history item having this reference
			for _, rf := range rfs {
				errs = append(errs, field.Forbidden(rf.path, err.Error()))
			}
		}
	}

	return errs
}

type referencePath struct {
	path     *field.Path
	insecure bool
}

func collectImageStreamSpecImageReferences(s *imageapi.ImageStream) sets.String {
	res := sets.NewString()

	if len(s.Spec.DockerImageRepository) > 0 {
		res.Insert(s.Spec.DockerImageRepository)
	}
	for _, tagRef := range s.Spec.Tags {
		if tagRef.From != nil && tagRef.From.Kind == "DockerImage" {
			res.Insert(tagRef.From.Name)
		}
	}
	return res
}

func collectImageStreamStatusImageReferences(s *imageapi.ImageStream) map[string][]referencePath {
	var (
		res      = make(map[string][]referencePath)
		insecure = isRepositoryInsecure(s)
	)

	for tag, eventList := range s.Status.Tags {
		tagInsecure := false
		if tr, ok := s.Spec.Tags[tag]; ok {
			tagInsecure = tr.ImportPolicy.Insecure
		}
		for i, item := range eventList.Items {
			rfs := res[item.DockerImageReference]
			rfs = append(rfs, referencePath{
				path:     field.NewPath("status", "tags").Key(tag).Child("items").Index(i).Child("dockerImageReference"),
				insecure: insecure || tagInsecure,
			})
			res[item.DockerImageReference] = rfs
		}
	}
	return res
}

// ValidateImageStreamMapping tests required fields for an ImageStreamMapping.
func ValidateImageStreamMapping(mapping *imageapi.ImageStreamMapping) field.ErrorList {
	result := validation.ValidateObjectMeta(&mapping.ObjectMeta, true, path.ValidatePathSegmentName, field.NewPath("metadata"))

	hasRepository := len(mapping.DockerImageRepository) != 0
	hasName := len(mapping.Name) != 0
	switch {
	case hasRepository:
		if _, err := imageref.Parse(mapping.DockerImageRepository); err != nil {
			result = append(result, field.Invalid(field.NewPath("dockerImageRepository"), mapping.DockerImageRepository, err.Error()))
		}
	case hasName:
	default:
		result = append(result, field.Required(field.NewPath("name"), ""))
		result = append(result, field.Required(field.NewPath("dockerImageRepository"), ""))
	}

	if reasons := validation.ValidateNamespaceName(mapping.Namespace, false); len(reasons) != 0 {
		result = append(result, field.Invalid(field.NewPath("metadata", "namespace"), mapping.Namespace, strings.Join(reasons, ", ")))
	}
	if len(mapping.Tag) == 0 {
		result = append(result, field.Required(field.NewPath("tag"), ""))
	}
	if errs := validateImage(&mapping.Image, field.NewPath("image")); len(errs) != 0 {
		result = append(result, errs...)
	}
	return result
}

// ValidateImageStreamTag validates a mutation of an image stream tag, which can happen on PUT.
func ValidateImageStreamTag(ist *imageapi.ImageStreamTag) field.ErrorList {
	// "normal" object validators aren't supposed to be querying the API server so passing
	// context is not expected, but we are special. Don't try to wire it up.
	return ValidateImageStreamTagWithWhitelister(context.TODO(), nil, ist)
}

// ValidateImageStreamTag validates a mutation of an image stream tag, which can happen on PUT. Additionally,
// it validates each new image reference against registry whitelist.
func ValidateImageStreamTagWithWhitelister(
	ctx context.Context,
	whitelister whitelist.RegistryWhitelister,
	ist *imageapi.ImageStreamTag,
) field.ErrorList {
	result := validation.ValidateObjectMeta(&ist.ObjectMeta, true, path.ValidatePathSegmentName, field.NewPath("metadata"))

	if ist.Tag != nil {
		// TODO: verify that istag inherits imagestream's annotations
		insecureRepository := isRepositoryInsecure(ist)
		result = append(result, ValidateImageStreamTagReference(ctx, whitelister, insecureRepository, *ist.Tag, field.NewPath("tag"))...)
		if ist.Tag.Annotations != nil && !kapihelper.Semantic.DeepEqual(ist.Tag.Annotations, ist.ObjectMeta.Annotations) {
			result = append(result, field.Invalid(field.NewPath("tag", "annotations"), "<map>", "tag annotations must not be provided or must be equal to the object meta annotations"))
		}
	}

	return result
}

// ValidateImageStreamTagUpdate ensures that only the annotations or the image reference of the IST have changed.
func ValidateImageStreamTagUpdate(newIST, oldIST *imageapi.ImageStreamTag) field.ErrorList {
	// "normal" object validators aren't supposed to be querying the API server so passing
	// context is not expected, but we are special. Don't try to wire it through.
	return ValidateImageStreamTagUpdateWithWhitelister(context.TODO(), nil, newIST, oldIST)
}

// ValidateImageStreamTagUpdate ensures that only the annotations or the image reference of the IST have
// changed. Additionally, it validates image reference against registry whitelist if it changed.
func ValidateImageStreamTagUpdateWithWhitelister(
	ctx context.Context,
	whitelister whitelist.RegistryWhitelister,
	newIST, oldIST *imageapi.ImageStreamTag,
) field.ErrorList {
	result := validation.ValidateObjectMetaUpdate(&newIST.ObjectMeta, &oldIST.ObjectMeta, field.NewPath("metadata"))

	if whitelister != nil && oldIST.Tag != nil && oldIST.Tag.From != nil && oldIST.Tag.From.Kind == "DockerImage" {
		whitelister = whitelister.Copy()
		err := whitelister.WhitelistRepository(oldIST.Tag.From.Name)
		if err != nil {
			klog.V(4).Infof("image stream tag %s/%s: unable to add pull spec %q to whitelist: %s", oldIST.Namespace, oldIST.Name, oldIST.Tag.From.Name, err)
		}
	}

	if newIST.Tag != nil {
		result = append(result, ValidateImageStreamTagReference(ctx, whitelister, isRepositoryInsecure(newIST), *newIST.Tag, field.NewPath("tag"))...)
		if newIST.Tag.Annotations != nil && !kapihelper.Semantic.DeepEqual(newIST.Tag.Annotations, newIST.ObjectMeta.Annotations) {
			result = append(result, field.Invalid(field.NewPath("tag", "annotations"), "<map>", "tag annotations must not be provided or must be equal to the object meta annotations"))
		}
	}

	// ensure that only tag and annotations have changed
	newISTCopy := *newIST
	oldISTCopy := *oldIST
	newISTCopy.Annotations, oldISTCopy.Annotations = nil, nil
	newISTCopy.Tag, oldISTCopy.Tag = nil, nil
	newISTCopy.LookupPolicy = oldISTCopy.LookupPolicy
	newISTCopy.Generation = oldISTCopy.Generation
	if !kapihelper.Semantic.Equalities.DeepEqual(&newISTCopy, &oldISTCopy) {
		result = append(result, field.Invalid(field.NewPath("metadata"), "", "may not update fields other than metadata.annotations"))
	}

	return result
}

// ValidateImageTag validates a mutation of an image stream tag, which can happen on PUT.
func ValidateImageTag(ctx context.Context, itag *imageapi.ImageTag) field.ErrorList {
	return ValidateImageTagWithWhitelister(ctx, nil, itag)
}

// ValidateImageTagWithWhitelister validates a mutation of an image stream tag, which can happen on PUT. Additionally,
// it validates each new image reference against registry whitelist.
func ValidateImageTagWithWhitelister(
	ctx context.Context,
	whitelister whitelist.RegistryWhitelister,
	itag *imageapi.ImageTag,
) field.ErrorList {
	result := validation.ValidateObjectMeta(&itag.ObjectMeta, true, path.ValidatePathSegmentName, field.NewPath("metadata"))

	if itag.Spec == nil {
		result = append(result, field.Required(field.NewPath("spec"), "spec is a required field during creation"))
	} else {
		_, imageTag, ok := imageutil.SplitImageStreamTag(itag.Name)
		if !ok || itag.Spec.Name != imageTag {
			result = append(result, field.Invalid(field.NewPath("spec", "name"), itag.Spec.Name, "must match image tag name"))
		}

		insecureRepository := isRepositoryInsecure(itag)
		result = append(result, ValidateImageStreamTagReference(ctx, whitelister, insecureRepository, *itag.Spec, field.NewPath("spec"))...)
	}

	return result
}

// ValidateImageTagUpdate ensures that only the annotations or the image reference of the IST have changed.
func ValidateImageTagUpdate(newITag, oldITag *imageapi.ImageTag) field.ErrorList {
	// "normal" object validators aren't supposed to be querying the API server so passing
	// context is not expected, but we are special. Don't try to wire it up.
	return ValidateImageTagUpdateWithWhitelister(context.TODO(), nil, newITag, oldITag)
}

// ValidateImageTagUpdateWithWhitelister ensures that only the annotations or the image reference of the IST have
// changed. Additionally, it validates image reference against registry whitelist if it changed.
func ValidateImageTagUpdateWithWhitelister(
	ctx context.Context,
	whitelister whitelist.RegistryWhitelister,
	newITag, oldITag *imageapi.ImageTag,
) field.ErrorList {
	result := validation.ValidateObjectMetaUpdate(&newITag.ObjectMeta, &oldITag.ObjectMeta, field.NewPath("metadata"))

	if whitelister != nil && oldITag.Spec != nil && oldITag.Spec.From != nil && oldITag.Spec.From.Kind == "DockerImage" {
		whitelister = whitelister.Copy()
		err := whitelister.WhitelistRepository(oldITag.Spec.From.Name)
		if err != nil {
			klog.V(4).Infof("image stream tag %s/%s: unable to add pull spec %q to whitelist: %s", oldITag.Namespace, oldITag.Name, oldITag.Spec.From.Name, err)
		}
	}

	if newITag.Spec != nil {
		_, imageTag, ok := imageutil.SplitImageStreamTag(newITag.Name)
		if !ok || newITag.Spec.Name != imageTag {
			result = append(result, field.Invalid(field.NewPath("spec", "name"), newITag.Spec.Name, "must match image tag name"))
		}

		result = append(result, ValidateImageStreamTagReference(ctx, whitelister, isRepositoryInsecure(newITag), *newITag.Spec, field.NewPath("spec"))...)
	}

	// ensure that only spec has changed
	if !kapihelper.Semantic.Equalities.DeepEqual(&newITag.ObjectMeta, &oldITag.ObjectMeta) {
		result = append(result, field.Invalid(field.NewPath("metadata"), "", "may not update fields other than spec"))
	}
	if !kapihelper.Semantic.Equalities.DeepEqual(newITag.Status, oldITag.Status) {
		result = append(result, field.Invalid(field.NewPath("status"), "", "may not update fields other than spec"))
	}

	return result
}

func ValidateRegistryAllowedForImport(
	ctx context.Context,
	whitelister whitelist.RegistryWhitelister,
	path *field.Path,
	name, registryHost,
	registryPort string,
) field.ErrorList {
	hostname := net.JoinHostPort(registryHost, registryPort)
	err := whitelister.AdmitHostname(ctx, hostname, whitelist.WhitelistTransportSecure)
	if err != nil {
		return field.ErrorList{field.Forbidden(path, fmt.Sprintf("importing images from registry %q is forbidden: %v", hostname, err))}
	}
	return nil
}

func ValidateImageStreamImport(isi *imageapi.ImageStreamImport) field.ErrorList {
	specPath := field.NewPath("spec")
	imagesPath := specPath.Child("images")
	repoPath := specPath.Child("repository")

	errs := field.ErrorList{}
	for i, spec := range isi.Spec.Images {
		from := spec.From
		switch from.Kind {
		case "DockerImage":
			if spec.To != nil && len(spec.To.Name) == 0 {
				errs = append(errs, field.Invalid(imagesPath.Index(i).Child("to", "name"), spec.To.Name, "the name of the target tag must be specified"))
			}
			if len(spec.From.Name) == 0 {
				errs = append(errs, field.Required(imagesPath.Index(i).Child("from", "name"), ""))
			} else {
				// The ParseDockerImageReference qualifies '*' as a wrong name.
				// The legacy clients use this character to look up imagestreams.
				// TODO: This should be removed in 1.6
				// See for more info: https://github.com/openshift/origin/pull/11774#issuecomment-258905994
				if spec.From.Name == "*" {
					continue
				}
				if _, err := imageref.Parse(spec.From.Name); err != nil {
					errs = append(errs, field.Invalid(imagesPath.Index(i).Child("from", "name"), spec.From.Name, err.Error()))
				}
			}
		default:
			errs = append(errs, field.Invalid(imagesPath.Index(i).Child("from", "kind"), from.Kind, "only DockerImage is supported"))
		}

		errs = append(errs, ValidateImportPolicy(spec.ImportPolicy, imagesPath.Index(i).Child("importPolicy"))...)
	}

	if spec := isi.Spec.Repository; spec != nil {
		from := spec.From
		switch from.Kind {
		case "DockerImage":
			if len(spec.From.Name) == 0 {
				errs = append(errs, field.Required(repoPath.Child("from", "name"), "container image references require a name"))
			} else {
				if ref, err := imageref.Parse(from.Name); err != nil {
					errs = append(errs, field.Invalid(repoPath.Child("from", "name"), from.Name, err.Error()))
				} else {
					if len(ref.ID) > 0 || len(ref.Tag) > 0 {
						errs = append(errs, field.Invalid(repoPath.Child("from", "name"), from.Name, "you must specify an image repository, not a tag or ID"))
					}
				}
			}
		default:
			errs = append(errs, field.Invalid(repoPath.Child("from", "kind"), from.Kind, "only DockerImage is supported"))
		}

		errs = append(errs, ValidateImportPolicy(spec.ImportPolicy, repoPath.Child("importPolicy"))...)
	}
	if len(isi.Spec.Images) == 0 && isi.Spec.Repository == nil {
		errs = append(errs, field.Invalid(imagesPath, nil, "you must specify at least one image or a repository import"))
	}

	errs = append(errs, validation.ValidateObjectMeta(&isi.ObjectMeta, true, ValidateImageStreamName, field.NewPath("metadata"))...)
	return errs
}

func isRepositoryInsecure(obj runtime.Object) bool {
	accessor, err := kmeta.Accessor(obj)
	if err != nil {
		klog.V(4).Infof("Error getting accessor for %#v", obj)
		return false
	}
	return accessor.GetAnnotations()[imagev1.InsecureRepositoryAnnotation] == "true"
}

func getWhitelistTransportForFlag(insecure, allowSecureFallback bool) whitelist.WhitelistTransport {
	if insecure {
		if allowSecureFallback {
			return whitelist.WhitelistTransportAny
		}
		return whitelist.WhitelistTransportInsecure
	}
	return whitelist.WhitelistTransportSecure
}

// ValidateImageStreamLayers ensures that given object is valid.
func ValidateImageStreamLayers(isl *imageapi.ImageStreamLayers) field.ErrorList {
	allErrs := field.ErrorList{}
	for name, ibr := range isl.Images {
		if ibr.ImageMissing {
			if len(ibr.Layers) > 0 {
				allErrs = append(allErrs, field.Invalid(field.NewPath("images").Key(name).Child("layers"), ibr.Layers, "layers must be empty if image is missing"))
			}
			if ibr.Config != nil {
				allErrs = append(allErrs, field.Invalid(field.NewPath("images").Key(name).Child("config"), ibr.Config, "config must be null if image is missing"))
			}
			if len(ibr.Manifests) > 0 {
				allErrs = append(allErrs, field.Invalid(field.NewPath("images").Key(name).Child("manifest"), ibr.Manifests, "manifests must be empty if image is missing"))
			}
		} else if len(ibr.Manifests) == 0 {
			for i, layer := range ibr.Layers {
				if len(layer) == 0 {
					allErrs = append(allErrs, field.Invalid(field.NewPath("images").Key(name).Child("layers").Index(i), layer, "layer cannot be empty"))
				}
			}
			if ibr.Config != nil && *ibr.Config == "" {
				allErrs = append(allErrs, field.Invalid(field.NewPath("images").Key(name).Child("config"), ibr.Config, "config cannot be an empty string"))
			}
		} else {
			if len(ibr.Layers) > 0 {
				allErrs = append(allErrs, field.Invalid(field.NewPath("images").Key(name).Child("layers"), ibr.Layers, "layers must be empty if manifests are present"))
			}
			if ibr.Config != nil {
				allErrs = append(allErrs, field.Invalid(field.NewPath("images").Key(name).Child("config"), ibr.Config, "config must be null if manifests are present"))
			}
			for i, manifest := range ibr.Manifests {
				if len(manifest) == 0 {
					allErrs = append(allErrs, field.Invalid(field.NewPath("images").Key(name).Child("manifests").Index(i), manifest, "manifest cannot be empty"))
				}
			}
		}
	}
	return allErrs
}
