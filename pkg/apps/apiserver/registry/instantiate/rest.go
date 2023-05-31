package instantiate

import (
	"context"
	"fmt"

	v1 "github.com/openshift/openshift-apiserver/pkg/apps/apis/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/admission"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/helper"

	"github.com/openshift/api/apps"
	appsv1 "github.com/openshift/api/apps/v1"
	imagev1client "github.com/openshift/client-go/image/clientset/versioned"
	imagev1typedclient "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	"github.com/openshift/library-go/pkg/apps/appsserialization"
	"github.com/openshift/library-go/pkg/apps/appsutil"
	"github.com/openshift/library-go/pkg/image/imageutil"
	appsapi "github.com/openshift/openshift-apiserver/pkg/apps/apis/apps"
	"github.com/openshift/openshift-apiserver/pkg/apps/apis/apps/validation"
)

// NewREST provides new REST storage for the apps API group.
func NewREST(store registry.Store, imagesclient imagev1client.Interface, kc kubernetes.Interface, admission admission.Interface) *REST {
	store.UpdateStrategy = Strategy
	return &REST{store: &store, is: imagesclient.ImageV1(), rn: kc.CoreV1(), admit: admission}
}

// REST implements the Creater & Storage interfaces.
var _ rest.Creater = &REST{}
var _ rest.Storage = &REST{}
var _ rest.SingularNameProvider = &REST{}

type REST struct {
	store *registry.Store
	is    imagev1typedclient.ImageStreamsGetter
	rn    corev1client.ReplicationControllersGetter
	admit admission.Interface
}

func (s *REST) New() runtime.Object {
	return &appsapi.DeploymentRequest{}
}

// Create instantiates a deployment config
func (s *REST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	req, ok := obj.(*appsapi.DeploymentRequest)
	if !ok {
		return nil, errors.NewInternalError(fmt.Errorf("wrong object passed for requesting a new rollout: %#v", obj))
	}
	var ret runtime.Object
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		configObj, err := s.store.Get(ctx, req.Name, &metav1.GetOptions{})
		if err != nil {
			return err
		}
		config := configObj.(*appsapi.DeploymentConfig)
		old := config

		if errs := validation.ValidateRequestForDeploymentConfig(req, config); len(errs) > 0 {
			return errors.NewInvalid(apps.Kind("DeploymentRequest"), req.Name, errs)
		}

		// We need to process the deployment config before we can determine if it is possible to trigger
		// a deployment.
		if req.Latest {
			if err := processTriggers(ctx, config, s.is, req.Force, req.ExcludeTriggers); err != nil {
				return err
			}
		}

		canTrigger, causes, err := canTrigger(ctx, config, s.rn, req.Force)
		if err != nil {
			return err
		}
		// If we cannot trigger then there is nothing to do here.
		if !canTrigger {
			ret = &metav1.Status{
				Message: fmt.Sprintf("deployment config %q cannot be instantiated", config.Name),
				Code:    int32(204),
			}
			return nil
		}
		klog.V(4).Infof("New deployment for %q caused by %#v", config.Name, causes)

		config.Status.Details = new(appsapi.DeploymentDetails)
		config.Status.Details.Causes = causes
		switch causes[0].Type {
		case appsapi.DeploymentTriggerOnConfigChange:
			config.Status.Details.Message = "config change"
		case appsapi.DeploymentTriggerOnImageChange:
			config.Status.Details.Message = "image change"
		case appsapi.DeploymentTriggerManual:
			config.Status.Details.Message = "manual change"
		}
		config.Status.LatestVersion++

		userInfo, _ := apirequest.UserFrom(ctx)
		attrs := admission.NewAttributesRecord(config, old, apps.Kind("DeploymentConfig").WithVersion("v1"), config.Namespace, config.Name, apps.Resource("DeploymentConfig").WithVersion("v1"), "", admission.Update,
			options, false, userInfo)
		objectInterfaces := admission.NewObjectInterfacesFromScheme(legacyscheme.Scheme)
		if err := s.admit.(admission.MutationInterface).Admit(ctx, attrs, objectInterfaces); err != nil {
			return err
		}
		if err := s.admit.(admission.ValidationInterface).Validate(ctx, attrs, objectInterfaces); err != nil {
			return err
		}

		ret, _, err = s.store.Update(
			ctx,
			config.Name,
			rest.DefaultUpdatedObjectInfo(config),
			rest.AdmissionToValidateObjectFunc(s.admit, attrs, objectInterfaces),
			rest.AdmissionToValidateObjectUpdateFunc(s.admit, attrs, objectInterfaces),
			false,
			&metav1.UpdateOptions{},
		)
		return err
	})

	return ret, err
}

func (s *REST) Destroy() {
	s.store.Destroy()
}

func (s *REST) GetSingularName() string {
	return "deploymentrequest"
}

// processTriggers will go over all deployment triggers that require processing and update
// the deployment config accordingly. This contains the work that the image change controller
// had been doing up to the point we got the /instantiate endpoint.
func processTriggers(ctx context.Context, config *appsapi.DeploymentConfig, is imagev1typedclient.ImageStreamsGetter, force bool, exclude []appsapi.DeploymentTriggerType) error {
	errs := []error{}

	// Process any image change triggers.
	for _, trigger := range config.Spec.Triggers {
		if trigger.Type != appsapi.DeploymentTriggerOnImageChange {
			continue
		}

		params := trigger.ImageChangeParams

		// Forced deployments should always try to resolve the images in the template.
		// On the other hand, paused deployments or non-automatic triggers shouldn't.
		if !force && (config.Spec.Paused || !params.Automatic) {
			continue
		}

		if containsTriggerType(exclude, trigger.Type) {
			continue
		}

		// Tag references are already validated
		name, tag, _ := imageutil.SplitImageStreamTag(params.From.Name)
		stream, err := is.ImageStreams(params.From.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				errs = append(errs, err)
			}
			continue
		}

		// Find the latest tag event for the trigger reference.
		latestReference, ok := imageutil.ResolveLatestTaggedImage(stream, tag)
		if !ok {
			continue
		}

		// Ensure a change occurred
		if len(latestReference) == 0 || latestReference == params.LastTriggeredImage {
			continue
		}

		// Update containers
		names := sets.NewString(params.ContainerNames...)
		for i := range config.Spec.Template.Spec.Containers {
			container := &config.Spec.Template.Spec.Containers[i]
			if !names.Has(container.Name) {
				continue
			}
			if container.Image != latestReference || params.LastTriggeredImage != latestReference {
				// Update the image
				container.Image = latestReference
				// Log the last triggered image ID
				params.LastTriggeredImage = latestReference
			}
		}
		for i := range config.Spec.Template.Spec.InitContainers {
			container := &config.Spec.Template.Spec.InitContainers[i]
			if !names.Has(container.Name) {
				continue
			}
			if container.Image != latestReference || params.LastTriggeredImage != latestReference {
				// Update the image
				container.Image = latestReference
				// Log the last triggered image ID
				params.LastTriggeredImage = latestReference
			}
		}
	}

	if err := utilerrors.NewAggregate(errs); err != nil {
		return errors.NewInternalError(err)
	}

	return nil
}

func containsTriggerType(types []appsapi.DeploymentTriggerType, triggerType appsapi.DeploymentTriggerType) bool {
	for _, t := range types {
		if t == triggerType {
			return true
		}
	}
	return false
}

// canTrigger determines if we can trigger a new deployment for config based on the various deployment triggers.
func canTrigger(
	ctx context.Context,
	config *appsapi.DeploymentConfig,
	rn corev1client.ReplicationControllersGetter,
	force bool,
) (bool, []appsapi.DeploymentCause, error) {

	decoded, err := decodeFromLatestDeployment(ctx, config, rn)
	if err != nil {
		return false, nil, err
	}

	ictCount, resolved, canTriggerByImageChange := 0, 0, false
	var causes []appsapi.DeploymentCause

	for _, t := range config.Spec.Triggers {
		if t.Type != appsapi.DeploymentTriggerOnImageChange {
			continue
		}
		ictCount++

		// If the image is yet to be resolved then we cannot process this trigger.
		lastTriggered := t.ImageChangeParams.LastTriggeredImage
		if len(lastTriggered) == 0 {
			continue
		}
		resolved++

		// Non-automatic triggers should not be able to trigger deployments.
		if !t.ImageChangeParams.Automatic {
			continue
		}

		// We need stronger checks in order to validate that this template
		// change is an image change. Look at the deserialized config's
		// triggers and compare with the present trigger. Initial deployments
		// should always trigger - there is no previous config to use for the
		// comparison. Also configs with new/updated triggers should always trigger.
		if config.Status.LatestVersion == 0 || hasUpdatedTriggers(*config, *decoded) || triggeredByDifferentImage(*t.ImageChangeParams, *decoded) {
			canTriggerByImageChange = true
		}

		if !canTriggerByImageChange {
			continue
		}

		causes = append(causes, appsapi.DeploymentCause{
			Type: appsapi.DeploymentTriggerOnImageChange,
			ImageTrigger: &appsapi.DeploymentCauseImageTrigger{
				From: core.ObjectReference{
					Name:      t.ImageChangeParams.From.Name,
					Namespace: t.ImageChangeParams.From.Namespace,
					Kind:      "ImageStreamTag",
				},
			},
		})
	}

	if ictCount != resolved {
		err = errors.NewBadRequest(fmt.Sprintf("cannot trigger a deployment for %q because it contains unresolved images", config.Name))
		return false, nil, err
	}

	if force {
		return true, []appsapi.DeploymentCause{{Type: appsapi.DeploymentTriggerManual}}, nil
	}

	canTriggerByConfigChange := false
	externalConfig := &appsv1.DeploymentConfig{}
	if err := v1.Convert_apps_DeploymentConfig_To_v1_DeploymentConfig(config, externalConfig, nil); err != nil {
		return false, nil, err
	}
	if appsutil.HasChangeTrigger(externalConfig) && // Our deployment config has a config change trigger
		len(causes) == 0 && // and no other trigger has triggered.
		(config.Status.LatestVersion == 0 || // Either it's the initial deployment
			!helper.Semantic.DeepEqual(config.Spec.Template, decoded.Spec.Template)) /* or a config change happened so we need to trigger */ {

		canTriggerByConfigChange = true
		causes = []appsapi.DeploymentCause{{Type: appsapi.DeploymentTriggerOnConfigChange}}
	}

	return canTriggerByConfigChange || canTriggerByImageChange, causes, nil
}

// decodeFromLatestDeployment will try to return the decoded version of the current deploymentconfig
// found in the annotations of its latest deployment. If there is no previous deploymentconfig (ie.
// latestVersion == 0), the returned deploymentconfig will be the same.
func decodeFromLatestDeployment(ctx context.Context, config *appsapi.DeploymentConfig, rn corev1client.ReplicationControllersGetter) (*appsapi.DeploymentConfig, error) {
	if config.Status.LatestVersion == 0 {
		return config, nil
	}
	externalConfig := &appsv1.DeploymentConfig{}
	if err := v1.Convert_apps_DeploymentConfig_To_v1_DeploymentConfig(config, externalConfig, nil); err != nil {
		return nil, err
	}
	latestDeploymentName := appsutil.LatestDeploymentNameForConfig(externalConfig)
	deployment, err := rn.ReplicationControllers(config.Namespace).Get(ctx, latestDeploymentName, metav1.GetOptions{})
	if err != nil {
		// If there's no deployment for the latest config, we have no basis of
		// comparison. It's the responsibility of the deployment config controller
		// to make the deployment for the config, so return early.
		return nil, err
	}
	decoded, err := appsserialization.DecodeDeploymentConfig(deployment)
	if err != nil {
		return nil, errors.NewInternalError(err)
	}
	internalConfig := &appsapi.DeploymentConfig{}
	if err := v1.Convert_v1_DeploymentConfig_To_apps_DeploymentConfig(decoded, internalConfig, nil); err != nil {
		return nil, err
	}
	return internalConfig, nil
}

// hasUpdatedTriggers checks if there is an diffence between previous deployment config
// trigger configuration and current one.
func hasUpdatedTriggers(current, previous appsapi.DeploymentConfig) bool {
	for _, ct := range current.Spec.Triggers {
		found := false
		if ct.Type != appsapi.DeploymentTriggerOnImageChange {
			continue
		}
		for _, pt := range previous.Spec.Triggers {
			if pt.Type != appsapi.DeploymentTriggerOnImageChange {
				continue
			}
			if found = ct.ImageChangeParams.From.Namespace == pt.ImageChangeParams.From.Namespace &&
				ct.ImageChangeParams.From.Name == pt.ImageChangeParams.From.Name; found {
				break
			}
		}
		if !found {
			klog.V(4).Infof("Deployment config %s/%s current version contains new trigger %#v", current.Namespace, current.Name, ct)
			return true
		}
	}
	return false
}

// triggeredByDifferentImage compares the provided image change parameters with those found in the
// previous deployment config (the one we decoded from the annotations of its latest deployment)
// and returns whether the two deployment configs have been triggered by a different image change.
func triggeredByDifferentImage(ictParams appsapi.DeploymentTriggerImageChangeParams, previous appsapi.DeploymentConfig) bool {
	for _, t := range previous.Spec.Triggers {
		if t.Type != appsapi.DeploymentTriggerOnImageChange {
			continue
		}

		if t.ImageChangeParams.From.Name != ictParams.From.Name ||
			t.ImageChangeParams.From.Namespace != ictParams.From.Namespace {
			continue
		}

		if t.ImageChangeParams.LastTriggeredImage != ictParams.LastTriggeredImage {
			klog.V(4).Infof("Deployment config %s/%s triggered by different image: %s -> %s", previous.Namespace, previous.Name, t.ImageChangeParams.LastTriggeredImage, ictParams.LastTriggeredImage)
			return true
		}
		return false
	}
	return false
}
