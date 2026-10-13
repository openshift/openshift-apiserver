package imagestreamimport

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/containers/image/v5/pkg/sysregistriesv2"

	authorizationapi "k8s.io/api/authorization/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	kapi "k8s.io/kubernetes/pkg/apis/core"
	kapihelper "k8s.io/kubernetes/pkg/apis/core/helper"

	"github.com/openshift/api/image"
	imagev1 "github.com/openshift/api/image/v1"
	configclientv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	configv1lister "github.com/openshift/client-go/config/listers/config/v1"
	imageclientv1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	operatorv1lister "github.com/openshift/client-go/operator/listers/operator/v1alpha1"
	"github.com/openshift/library-go/pkg/authorization/authorizationutil"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/library-go/pkg/quota/quotautil"

	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation"
	"github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation/whitelist"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/importer"
	internalimageutil "github.com/openshift/openshift-apiserver/pkg/image/apiserver/internal/imageutil"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registry/imagestream"
	"github.com/openshift/runtime-utils/pkg/registries"
)

// ImporterFunc returns an instance of the importer that should be used per invocation.
type ImporterFunc func(r importer.RepositoryRetriever, regConf *sysregistriesv2.V2RegistriesConf) importer.Interface

// REST implements the RESTStorage interface for ImageStreamImport
type REST struct {
	importFn          ImporterFunc
	streams           imagestream.Registry
	internalStreams   rest.CreaterUpdater
	images            rest.Creater
	isV1Client        imageclientv1.ImageStreamsGetter
	transport         http.RoundTripper
	insecureTransport http.RoundTripper
	strategy          *strategy
	sarClient         authorizationclient.SubjectAccessReviewInterface
	icspLister        operatorv1lister.ImageContentSourcePolicyLister
	idmsLister        configv1lister.ImageDigestMirrorSetLister
	itmsLister        configv1lister.ImageTagMirrorSetLister
	imageCfgV1Client  configclientv1.ImagesGetter
}

var _ rest.Creater = &REST{}
var _ rest.Scoper = &REST{}
var _ rest.Storage = &REST{}
var _ rest.SingularNameProvider = &REST{}

// NewREST returns a REST storage implementation that handles importing images.
// Insecure transport is optional, and both transports should not include
// client certs unless you wish to allow the entire cluster to import using
// those certs.
func NewREST(importFn ImporterFunc, streams imagestream.Registry, internalStreams rest.CreaterUpdater,
	images rest.Creater,
	isV1Client imageclientv1.ImageStreamsGetter,
	transport, insecureTransport http.RoundTripper,
	registryWhitelister whitelist.RegistryWhitelister,
	sarClient authorizationclient.SubjectAccessReviewInterface,
	icspLister operatorv1lister.ImageContentSourcePolicyLister,
	idmsLister configv1lister.ImageDigestMirrorSetLister,
	itmsLister configv1lister.ImageTagMirrorSetLister,
	imageCfgV1Client configclientv1.ImagesGetter,
) *REST {
	return &REST{
		importFn:          importFn,
		streams:           streams,
		internalStreams:   internalStreams,
		images:            images,
		isV1Client:        isV1Client,
		transport:         transport,
		insecureTransport: insecureTransport,
		strategy:          NewStrategy(registryWhitelister),
		sarClient:         sarClient,
		icspLister:        icspLister,
		idmsLister:        idmsLister,
		itmsLister:        itmsLister,
		imageCfgV1Client:  imageCfgV1Client,
	}
}

// New is only implemented to make REST implement RESTStorage
func (r *REST) New() runtime.Object {
	return &imageapi.ImageStreamImport{}
}

func (r *REST) Destroy() {}

func (r *REST) NamespaceScoped() bool {
	return true
}

func (r *REST) GetSingularName() string {
	return "imagestreamimport"
}

// createImages creates image objects in etcd and update related conditions
func (r *REST) createImages(
	ctx context.Context,
	isi *imageapi.ImageStreamImport,
	stream *imageapi.ImageStream,
	nextGeneration int64,
	now metav1.Time,
) error {
	imageCreater := newCachedImageCreater(r.strategy, r.images)

	if spec := isi.Spec.Repository; spec != nil {
		for i, status := range isi.Status.Repository.Images {
			importFailed := status.Image == nil || status.Status.Status != metav1.StatusSuccess
			if importFailed {
				setFailureCondition(status, stream, status.Tag, nextGeneration, now)
				continue
			}

			image := status.Image
			ref, err := reference.Parse(image.DockerImageReference)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("unable to parse image reference during import: %v", err))
				continue
			}
			from, err := reference.Parse(spec.From.Name)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("unable to parse from reference during import: %v", err))
				continue
			}

			tag := ref.Tag
			if len(status.Tag) > 0 {
				tag = status.Tag
			}
			// we've imported a set of tags, ensure spec tag will point to this for later imports
			from.ID, from.Tag = "", tag

			updated, updatedSubmanifests, err := r.importSuccessful(ctx, image, status.Manifests, stream, tag,
				from.Exact(), nextGeneration, now, spec.ImportPolicy, spec.ReferencePolicy, imageCreater)
			if err != nil {
				return err
			}

			isi.Status.Repository.Images[i].Image = updated
			isi.Status.Repository.Images[i].Manifests = updatedSubmanifests
		}
	}

	for i, spec := range isi.Spec.Images {
		klog.V(4).Infof("creating image for %#v", spec)
		if spec.To == nil {
			continue
		}
		tag := spec.To.Name

		status := isi.Status.Images[i]
		importFailed := status.Image == nil || status.Status.Status != metav1.StatusSuccess
		if importFailed {
			setFailureCondition(status, stream, tag, nextGeneration, now)
			ensureSpecTag(stream, tag, spec.From.Name, spec.ImportPolicy, spec.ReferencePolicy, false)
			continue
		}

		image := status.Image
		updated, updatedSubmanifests, err := r.importSuccessful(ctx, image, status.Manifests, stream, tag, spec.From.Name, nextGeneration,
			now, spec.ImportPolicy, spec.ReferencePolicy, imageCreater)
		if err != nil {
			klog.V(4).Infof("import error %q", err)
			return err
		}
		klog.V(4).Infof("imported %#v", image)

		isi.Status.Images[i].Image = updated
		isi.Status.Images[i].Manifests = updatedSubmanifests
	}

	return nil
}

func (r *REST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	isi, ok := obj.(*imageapi.ImageStreamImport)
	if !ok {
		return nil, kapierrors.NewBadRequest(fmt.Sprintf("obj is not an ImageStreamImport: %#v", obj))
	}

	inputMeta := isi.ObjectMeta

	objectMeta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	rest.FillObjectMetaSystemFields(objectMeta)

	if err := rest.BeforeCreate(r.strategy, ctx, obj); err != nil {
		return nil, err
	}
	if err := createValidation(ctx, obj.DeepCopyObject()); err != nil {
		return nil, err
	}

	// Check if the user is allowed to create Images or ImageStreamMappings.
	// In case the user is allowed to create them, do not validate the ImageStreamImport
	// registry location against the registry whitelist, but instead allow to create any
	// image from any registry.
	user, ok := apirequest.UserFrom(ctx)
	if !ok {
		return nil, kapierrors.NewBadRequest("unable to get user from context")
	}
	createImageSAR := authorizationutil.AddUserToSAR(user, &authorizationapi.SubjectAccessReview{
		Spec: authorizationapi.SubjectAccessReviewSpec{
			ResourceAttributes: &authorizationapi.ResourceAttributes{
				Verb:     "create",
				Group:    imageapi.GroupName,
				Resource: "images",
			},
		},
	})
	isCreateImage, err := r.sarClient.Create(ctx, createImageSAR, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	createImageStreamMappingSAR := authorizationutil.AddUserToSAR(user, &authorizationapi.SubjectAccessReview{
		Spec: authorizationapi.SubjectAccessReviewSpec{
			ResourceAttributes: &authorizationapi.ResourceAttributes{
				Verb:     "create",
				Group:    imageapi.GroupName,
				Resource: "imagestreammapping",
			},
		},
	})
	isCreateImageStreamMapping, err := r.sarClient.Create(ctx, createImageStreamMappingSAR, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	if !isCreateImage.Status.Allowed && !isCreateImageStreamMapping.Status.Allowed {
		if errs := r.strategy.ValidateAllowedRegistries(ctx, isi); len(errs) != 0 {
			return nil, kapierrors.NewInvalid(image.Kind("ImageStreamImport"), isi.Name, errs)
		}
	}

	namespace, ok := apirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, kapierrors.NewBadRequest("a namespace must be specified to import images")
	}

	create := false
	stream, err := r.streams.GetImageStream(ctx, isi.Name, &metav1.GetOptions{})
	if err != nil {
		if !kapierrors.IsNotFound(err) {
			return nil, err
		}
		// consistency check, stream must exist
		if len(inputMeta.ResourceVersion) > 0 || len(inputMeta.UID) > 0 {
			return nil, err
		}
		create = true
		stream = &imageapi.ImageStream{
			ObjectMeta: metav1.ObjectMeta{
				Name:       isi.Name,
				Namespace:  namespace,
				Generation: 0,
			},
		}
	} else {
		if len(inputMeta.ResourceVersion) > 0 && inputMeta.ResourceVersion != stream.ResourceVersion {
			klog.V(4).Infof("DEBUG: mismatch between requested ResourceVersion %s and located ResourceVersion %s", inputMeta.ResourceVersion, stream.ResourceVersion)
			return nil, kapierrors.NewConflict(image.Resource("imagestream"), inputMeta.Name, fmt.Errorf("the image stream was updated from %q to %q", inputMeta.ResourceVersion, stream.ResourceVersion))
		}
		if len(inputMeta.UID) > 0 && inputMeta.UID != stream.UID {
			klog.V(4).Infof("DEBUG: mismatch between requested UID %s and located UID %s", inputMeta.UID, stream.UID)
			return nil, kapierrors.NewNotFound(image.Resource("imagestream"), inputMeta.Name)
		}
	}

	icspRules, err := r.icspLister.List(labels.Everything())
	if err != nil {
		klog.Warningf("%s/%s: failed to load ImageContentSourcePolicy resources, mirrored images will not be found: %v", namespace, inputMeta.Name, err)
	}
	idmsRules, err := r.idmsLister.List(labels.Everything())
	if err != nil {
		klog.Warningf("%s/%s: failed to load ImageDigestMirrorSet resources, mirrored images will not be found: %v", namespace, inputMeta.Name, err)

	}
	itmsRules, err := r.itmsLister.List(labels.Everything())
	if err != nil {
		klog.Warningf("%s/%s: failed to load ImageTagMirrorSet resources, mirrored images will not be found: %v", namespace, inputMeta.Name, err)

	}

	imageConfig, err := r.imageCfgV1Client.Images().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, kapierrors.NewInternalError(err)
	}

	v2regConf := &sysregistriesv2.V2RegistriesConf{}
	if err = registries.EditRegistriesConfig(
		v2regConf,
		imageConfig.Spec.RegistrySources.InsecureRegistries,
		imageConfig.Spec.RegistrySources.BlockedRegistries,
		icspRules, idmsRules, itmsRules,
	); err != nil {
		klog.Warningf("failed to merge ImageContentSourcePolicy, ImageDigestMirrorSet, ImageTagMirrorSet resources, mirrored images will not be found: %v", err)
	}
	for i, reg := range v2regConf.Registries {
		v2regConf.Registries[i].Prefix = reg.Location
	}

	secretsList, err := r.isV1Client.ImageStreams(namespace).Secrets(ctx, isi.Name, metav1.GetOptions{})
	if err != nil {
		return nil, kapierrors.NewInternalError(err)
	}

	importCtx := importer.NewStaticCredentialsContext(
		r.transport, r.insecureTransport, secretsList.Items,
	)
	imports := r.importFn(importCtx, v2regConf)
	if err := imports.Import(ctx, isi, stream); err != nil {
		return nil, kapierrors.NewInternalError(err)
	}

	// check imported images status. If we get authentication error (401), try import same image without authentication.
	// container image registry gives 401 on public images if you have wrong secret in your secret list.
	// this block was introduced by PR #18012
	// TODO: remove this blocks when smarter auth client gets done with retries
	var imageStatus []metav1.Status
	importFailed := false
	for _, image := range isi.Status.Images {
		//cache all imports status
		imageStatus = append(imageStatus, image.Status)
		if image.Status.Reason == metav1.StatusReasonUnauthorized && strings.Contains(strings.ToLower(image.Status.Message), "username or password") {
			importFailed = true
		}
	}
	// try import IS without auth if it failed before
	if importFailed {
		importCtx := importer.NewStaticCredentialsContext(
			r.transport, r.insecureTransport, nil,
		)
		imports := r.importFn(importCtx, v2regConf)
		if err := imports.Import(ctx, isi, stream); err != nil {
			return nil, kapierrors.NewInternalError(err)
		}
	}
	//cycle through status and set old messages so not to confuse users
	for key, image := range isi.Status.Images {
		if image.Status.Reason == metav1.StatusReasonUnauthorized {
			isi.Status.Images[key].Status = imageStatus[key]
		}
	}

	// TODO: perform the transformation of the image stream and return it with the ISI if import is false
	//   so that clients can see what the resulting object would look like.
	if !isi.Spec.Import {
		clearManifests(isi)
		return isi, nil
	}

	if stream.Annotations == nil {
		stream.Annotations = make(map[string]string)
	}
	now := metav1.Now()
	_, hasAnnotation := stream.Annotations[imagev1.DockerImageRepositoryCheckAnnotation]
	nextGeneration := stream.Generation + 1

	original := stream.DeepCopy()

	err = r.createImages(ctx, isi, stream, nextGeneration, now)
	if err != nil {
		return nil, err
	}

	clearManifests(isi)

	// ensure defaulting is applied by round trip converting
	// TODO: convert to using versioned types.
	external, err := legacyscheme.Scheme.ConvertToVersion(stream, imagev1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	legacyscheme.Scheme.Default(external)
	internal, err := legacyscheme.Scheme.ConvertToVersion(external, imageapi.GroupVersion)
	if err != nil {
		return nil, err
	}
	stream = internal.(*imageapi.ImageStream)

	// if and only if we have changes between the original and the imported stream, trigger
	// an import
	hasChanges := !kapihelper.Semantic.DeepEqual(original, stream)
	if create {
		stream.Annotations[imagev1.DockerImageRepositoryCheckAnnotation] = now.UTC().Format(time.RFC3339)
		klog.V(4).Infof("create new stream: %#v", stream)
		obj, err = r.internalStreams.Create(ctx, stream, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	} else {
		if hasAnnotation && !hasChanges {
			klog.V(4).Infof("stream did not change: %#v", stream)
			obj, err = original, nil
		} else {
			if klog.V(4).Enabled() {
				klog.V(4).Infof("updating stream %s", diff.Diff(original, stream))
			}
			stream.Annotations[imagev1.DockerImageRepositoryCheckAnnotation] = now.UTC().Format(time.RFC3339)
			obj, _, err = r.internalStreams.Update(ctx, stream.Name, rest.DefaultUpdatedObjectInfo(stream), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
		}
	}

	if err != nil {
		// if we have am admission limit error then record the conditions on the original stream.  Quota errors
		// will be recorded by the importer.
		if quotautil.IsErrorLimitExceeded(err) {
			originalStream := original
			recordLimitExceededStatus(originalStream, stream, err, now, nextGeneration)
			var limitErr error
			obj, _, limitErr = r.internalStreams.Update(ctx, stream.Name, rest.DefaultUpdatedObjectInfo(originalStream), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
			if limitErr != nil {
				utilruntime.HandleError(fmt.Errorf("failed to record limit exceeded status in image stream %s/%s: %v", stream.Namespace, stream.Name, limitErr))
			}
		}

		return nil, err
	}
	isi.Status.Import = obj.(*imageapi.ImageStream)

	if errs := validation.ValidateImageStreamImport(isi); len(errs) != 0 {
		return nil, kapierrors.NewInvalid(image.Kind("ImageStreamImport"), isi.Name, errs)
	}
	return isi, nil
}

// recordLimitExceededStatus adds the limit err to any new tag.
func recordLimitExceededStatus(originalStream *imageapi.ImageStream, newStream *imageapi.ImageStream, err error, now metav1.Time, nextGeneration int64) {
	for tag := range newStream.Status.Tags {
		if _, ok := originalStream.Status.Tags[tag]; !ok {
			setTagConditions(originalStream, tag, newImportFailedCondition(err, nextGeneration, now))
		}
	}
}

func setFailureCondition(
	status imageapi.ImageImportStatus,
	stream *imageapi.ImageStream,
	tag string,
	nextGeneration int64,
	now metav1.Time,
) {
	message := status.Status.Message
	if len(message) == 0 {
		message = "unknown error prevented import"
	}
	condition := imageapi.TagEventCondition{
		Type:               imageapi.ImportSuccess,
		Status:             kapi.ConditionFalse,
		Message:            message,
		Reason:             string(status.Status.Reason),
		Generation:         nextGeneration,
		LastTransitionTime: now,
	}

	if tag == "" {
		if len(status.Tag) > 0 {
			tag = status.Tag
		} else if status.Image != nil {
			if ref, err := reference.Parse(status.Image.DockerImageReference); err == nil {
				tag = ref.Tag
			}
		}
	}

	if !hasTagCondition(stream, tag, condition) {
		setTagConditions(stream, tag, condition)
		if tagRef, ok := stream.Spec.Tags[tag]; ok {
			zero := int64(0)
			tagRef.Generation = &zero
			stream.Spec.Tags[tag] = tagRef
		}
	}
}

// hasTagCondition is an alternative to the public HasTagCondition in the internalimageutil package of this repo;
// unlike that public method, it factors in changes to the message of the image stream import in case image stream
// config changes (like which registry is used) still lead to an import InternalError. See
// https://bugzilla.redhat.com/show_bug.cgi?id=1788700
// So this returns true if the specified image stream tag has a condition with the same type, status, reason, and
// message (but still does not check generation or date).
func hasTagCondition(stream *imageapi.ImageStream, tag string, condition imageapi.TagEventCondition) bool {
	for _, existing := range stream.Status.Tags[tag].Conditions {
		if condition.Type == existing.Type && condition.Status == existing.Status && condition.Reason == existing.Reason &&
			condition.Message == existing.Message {
			return true
		}
	}
	return false
}

// SetTagConditions applies the specified conditions to the status of the given tag.
func setTagConditions(stream *imageapi.ImageStream, tag string, conditions ...imageapi.TagEventCondition) {
	tagEvents := stream.Status.Tags[tag]
	tagEvents.Conditions = conditions
	if stream.Status.Tags == nil {
		stream.Status.Tags = make(map[string]imageapi.TagEventList)
	}
	stream.Status.Tags[tag] = tagEvents
}

// ensureSpecTag guarantees that the spec tag is set with the provided from, importPolicy and referencePolicy.
// If reset is passed, the tag will be overwritten.
func ensureSpecTag(
	stream *imageapi.ImageStream,
	tag, from string,
	importPolicy imageapi.TagImportPolicy,
	referencePolicy imageapi.TagReferencePolicy,
	reset bool,
) imageapi.TagReference {
	if stream.Spec.Tags == nil {
		stream.Spec.Tags = make(map[string]imageapi.TagReference)
	}
	specTag, ok := stream.Spec.Tags[tag]
	if ok && !reset {
		return specTag
	}
	specTag.From = &kapi.ObjectReference{
		Kind: "DockerImage",
		Name: from,
	}

	zero := int64(0)
	specTag.Generation = &zero
	specTag.ImportPolicy = importPolicy

	// Only set the reference policy if it's not already explicitly
	// set on this tag.  Importing an image should not change the reference
	// policy.
	if len(specTag.ReferencePolicy.Type) == 0 {
		specTag.ReferencePolicy = referencePolicy
	}
	stream.Spec.Tags[tag] = specTag
	return specTag
}

// importSuccessful records a successful import into an image stream, setting the spec tag, status tag or conditions, and ensuring
// the image is created in etcd. Images are cached so they are not created multiple times in a row (when multiple tags point to the
// same image), and a failure to persist the image will be summarized before we update the stream. If an image was imported by this
// operation, it *replaces* the imported image (from the remote repository) with the updated image.
func (r *REST) importSuccessful(
	ctx context.Context,
	image *imageapi.Image,
	submanifests []imageapi.Image,
	stream *imageapi.ImageStream,
	tag string,
	from string,
	nextGeneration int64,
	now metav1.Time,
	importPolicy imageapi.TagImportPolicy,
	referencePolicy imageapi.TagReferencePolicy,
	imageCreater *cachedImageCreater,
) (*imageapi.Image, []imageapi.Image, error) {
	pullSpec, _ := mostAccuratePullSpec(image.DockerImageReference, image.Name, "")
	tagEvent := imageapi.TagEvent{
		Created:              now,
		DockerImageReference: pullSpec,
		Image:                image.Name,
		Generation:           nextGeneration,
	}

	if stream.Spec.Tags == nil {
		stream.Spec.Tags = make(map[string]imageapi.TagReference)
	}

	// ensure the spec and status tag match the imported image
	if hasTagChanged(stream, tag, tagEvent) {
		ensureSpecTag(stream, tag, from, importPolicy, referencePolicy, true)
		internalimageutil.AddTagEventToImageStream(stream, tag, tagEvent)
	}

	specTag := stream.Spec.Tags[tag]
	specTag.ImportPolicy = importPolicy
	stream.Spec.Tags[tag] = specTag

	setTagConditions(stream, tag) // clear error conditions from previous import attempts

	var updatedSubmanifests []imageapi.Image
	for _, submanifest := range submanifests {
		submanifestCopy := submanifest

		updatedSubmanifest, err := imageCreater.Create(ctx, &submanifestCopy)
		if err != nil {
			return nil, nil, err
		}

		updatedSubmanifests = append(updatedSubmanifests, *updatedSubmanifest)
	}

	updatedImage, err := imageCreater.Create(ctx, image)
	if err != nil {
		return nil, nil, err
	}

	return updatedImage, updatedSubmanifests, nil
}

// mostAccuratePullSpec returns a container image reference that uses the current ID if possible, the current tag otherwise, and
// returns false if the reference if the spec could not be parsed. The returned spec has all client defaults applied.
func mostAccuratePullSpec(pullSpec string, id, tag string) (string, bool) {
	ref, err := reference.Parse(pullSpec)
	if err != nil {
		return pullSpec, false
	}
	if len(id) > 0 {
		ref.ID = id
	}
	if len(tag) > 0 {
		ref.Tag = tag
	}
	return ref.MostSpecific().Exact(), true
}

func hasTagChanged(stream *imageapi.ImageStream, tag string, newEvent imageapi.TagEvent) bool {
	tagEvent, ok := stream.Status.Tags[tag]
	if !ok || len(tagEvent.Items) == 0 {
		return true
	}
	specTag, ok := stream.Spec.Tags[tag]
	if !ok || specTag.Generation == nil {
		return true
	}

	previousEvent := tagEvent.Items[0]
	sameRef := previousEvent.DockerImageReference == newEvent.DockerImageReference
	sameImage := previousEvent.Image == newEvent.Image
	return !sameRef || !sameImage || *specTag.Generation > previousEvent.Generation
}

// clearManifests unsets the manifest for each object that does not request it
func clearManifests(isi *imageapi.ImageStreamImport) {
	for i := range isi.Status.Images {
		if !isi.Spec.Images[i].IncludeManifest {
			if isi.Status.Images[i].Image != nil {
				isi.Status.Images[i].Image.DockerImageManifest = ""
				isi.Status.Images[i].Image.DockerImageConfig = ""
			}
		}
	}
	if isi.Spec.Repository != nil && !isi.Spec.Repository.IncludeManifest {
		for i := range isi.Status.Repository.Images {
			if isi.Status.Repository.Images[i].Image != nil {
				isi.Status.Repository.Images[i].Image.DockerImageManifest = ""
				isi.Status.Repository.Images[i].Image.DockerImageConfig = ""
			}
		}
	}
}

func newImportFailedCondition(err error, gen int64, now metav1.Time) imageapi.TagEventCondition {
	c := imageapi.TagEventCondition{
		Type:       imageapi.ImportSuccess,
		Status:     kapi.ConditionFalse,
		Message:    err.Error(),
		Generation: gen,

		LastTransitionTime: now,
	}
	if status, ok := err.(kapierrors.APIStatus); ok {
		s := status.Status()
		c.Reason, c.Message = string(s.Reason), s.Message
	}
	return c
}
