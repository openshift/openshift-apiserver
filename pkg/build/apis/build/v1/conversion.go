package v1

import (
	"net/url"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	corev1conversions "k8s.io/kubernetes/pkg/apis/core/v1"

	buildv1 "github.com/openshift/api/build/v1"
	"github.com/openshift/library-go/pkg/image/imageutil"
	newer "github.com/openshift/openshift-apiserver/pkg/build/apis/build"
	buildinternalhelpers "github.com/openshift/openshift-apiserver/pkg/build/apis/build/internal_helpers"
)

func Convert_v1_BuildConfig_To_build_BuildConfig(in *buildv1.BuildConfig, out *newer.BuildConfig, s conversion.Scope) error {
	if err := autoConvert_v1_BuildConfig_To_build_BuildConfig(in, out, s); err != nil {
		return err
	}

	newTriggers := []newer.BuildTriggerPolicy{}
	// Strip off any default imagechange triggers where the buildconfig's
	// "from" is not an ImageStreamTag, because those triggers
	// will never be invoked.
	imageRef := buildinternalhelpers.GetInputReference(out.Spec.Strategy)
	hasIST := imageRef != nil && imageRef.Kind == "ImageStreamTag"
	for _, trigger := range out.Spec.Triggers {
		if trigger.Type != newer.ImageChangeBuildTriggerType {
			newTriggers = append(newTriggers, trigger)
			continue
		}
		if (trigger.ImageChange == nil || trigger.ImageChange.From == nil) && !hasIST {
			continue
		}
		newTriggers = append(newTriggers, trigger)
	}
	out.Spec.Triggers = newTriggers
	return nil
}

func Convert_v1_SourceBuildStrategy_To_build_SourceBuildStrategy(in *buildv1.SourceBuildStrategy, out *newer.SourceBuildStrategy, s conversion.Scope) error {
	if err := autoConvert_v1_SourceBuildStrategy_To_build_SourceBuildStrategy(in, out, s); err != nil {
		return err
	}
	switch in.From.Kind {
	case "ImageStream":
		out.From.Kind = "ImageStreamTag"
		out.From.Name = imageutil.JoinImageStreamTag(in.From.Name, "")
	}
	return nil
}

func Convert_v1_DockerBuildStrategy_To_build_DockerBuildStrategy(in *buildv1.DockerBuildStrategy, out *newer.DockerBuildStrategy, s conversion.Scope) error {
	if err := autoConvert_v1_DockerBuildStrategy_To_build_DockerBuildStrategy(in, out, s); err != nil {
		return err
	}
	if in.From != nil {
		switch in.From.Kind {
		case "ImageStream":
			out.From.Kind = "ImageStreamTag"
			out.From.Name = imageutil.JoinImageStreamTag(in.From.Name, "")
		}
	}
	return nil
}

func Convert_v1_CustomBuildStrategy_To_build_CustomBuildStrategy(in *buildv1.CustomBuildStrategy, out *newer.CustomBuildStrategy, s conversion.Scope) error {
	if err := autoConvert_v1_CustomBuildStrategy_To_build_CustomBuildStrategy(in, out, s); err != nil {
		return err
	}
	switch in.From.Kind {
	case "ImageStream":
		out.From.Kind = "ImageStreamTag"
		out.From.Name = imageutil.JoinImageStreamTag(in.From.Name, "")
	}
	return nil
}

func Convert_v1_BuildOutput_To_build_BuildOutput(in *buildv1.BuildOutput, out *newer.BuildOutput, s conversion.Scope) error {
	if err := autoConvert_v1_BuildOutput_To_build_BuildOutput(in, out, s); err != nil {
		return err
	}
	if in.To != nil && (in.To.Kind == "ImageStream" || len(in.To.Kind) == 0) {
		out.To.Kind = "ImageStreamTag"
		out.To.Name = imageutil.JoinImageStreamTag(in.To.Name, "")
	}
	return nil
}

func Convert_v1_BuildTriggerPolicy_To_build_BuildTriggerPolicy(in *buildv1.BuildTriggerPolicy, out *newer.BuildTriggerPolicy, s conversion.Scope) error {
	if err := autoConvert_v1_BuildTriggerPolicy_To_build_BuildTriggerPolicy(in, out, s); err != nil {
		return err
	}

	switch in.Type {
	case buildv1.ImageChangeBuildTriggerTypeDeprecated:
		out.Type = newer.ImageChangeBuildTriggerType
	case buildv1.GenericWebHookBuildTriggerTypeDeprecated:
		out.Type = newer.GenericWebHookBuildTriggerType
	case buildv1.GitHubWebHookBuildTriggerTypeDeprecated:
		out.Type = newer.GitHubWebHookBuildTriggerType
	}
	return nil
}

func Convert_build_SourceRevision_To_v1_SourceRevision(in *newer.SourceRevision, out *buildv1.SourceRevision, s conversion.Scope) error {
	if err := autoConvert_build_SourceRevision_To_v1_SourceRevision(in, out, s); err != nil {
		return err
	}
	out.Type = buildv1.BuildSourceGit
	return nil
}

func Convert_build_BuildSource_To_v1_BuildSource(in *newer.BuildSource, out *buildv1.BuildSource, s conversion.Scope) error {
	if err := autoConvert_build_BuildSource_To_v1_BuildSource(in, out, s); err != nil {
		return err
	}
	switch {
	// It is legal for a buildsource to have both a git+dockerfile source, but in buildv1 that was represented
	// as type git.
	case in.Git != nil:
		out.Type = buildv1.BuildSourceGit
	// It is legal for a buildsource to have both a binary+dockerfile source, but in buildv1 that was represented
	// as type binary.
	case in.Binary != nil:
		out.Type = buildv1.BuildSourceBinary
	case in.Dockerfile != nil:
		out.Type = buildv1.BuildSourceDockerfile
	case len(in.Images) > 0:
		out.Type = buildv1.BuildSourceImage
	default:
		out.Type = buildv1.BuildSourceNone
	}
	return nil
}

func Convert_build_BuildStrategy_To_v1_BuildStrategy(in *newer.BuildStrategy, out *buildv1.BuildStrategy, s conversion.Scope) error {
	if err := autoConvert_build_BuildStrategy_To_v1_BuildStrategy(in, out, s); err != nil {
		return err
	}
	switch {
	case in.SourceStrategy != nil:
		out.Type = buildv1.SourceBuildStrategyType
	case in.DockerStrategy != nil:
		out.Type = buildv1.DockerBuildStrategyType
	case in.CustomStrategy != nil:
		out.Type = buildv1.CustomBuildStrategyType
	case in.JenkinsPipelineStrategy != nil:
		out.Type = buildv1.JenkinsPipelineBuildStrategyType
	default:
		out.Type = ""
	}
	return nil
}

func Convert_url_Values_To_v1_BuildLogOptions(in *url.Values, out *buildv1.BuildLogOptions, s conversion.Scope) error {
	if in == nil || out == nil {
		return nil
	}
	var podLogOptions *corev1.PodLogOptions
	if err := corev1conversions.Convert_url_Values_To_v1_PodLogOptions(in, podLogOptions, nil); err != nil {
		return err
	}
	*out = buildinternalhelpers.PodLogOptionsToBuildLogOptions(podLogOptions)
	return nil
}

// TODO: Is this needed?
func Convert_v1_BuildLogOptions_To_url_Values(in *buildv1.BuildLogOptions, out *url.Values, s conversion.Scope) error {
	if in == nil || out == nil {
		return nil
	}
	return nil
}

func Convert_url_Values_To_v1_BinaryBuildRequestOptions(in *url.Values, out *buildv1.BinaryBuildRequestOptions, s conversion.Scope) error {
	if in == nil || out == nil {
		return nil
	}
	out.AsFile = in.Get("asFile")
	out.Commit = in.Get("revision.commit")
	out.Message = in.Get("revision.message")
	out.AuthorName = in.Get("revision.authorName")
	out.AuthorEmail = in.Get("revision.authorEmail")
	out.CommitterName = in.Get("revision.committerName")
	out.CommitterEmail = in.Get("revision.committerEmail")
	return nil
}

func Convert_v1_BinaryBuildRequestOptions_To_url_Values(in *buildv1.BinaryBuildRequestOptions, out *url.Values, s conversion.Scope) error {
	if in == nil || out == nil {
		return nil
	}
	out.Set("asFile", in.AsFile)
	out.Set("revision.commit", in.Commit)
	out.Set("revision.message", in.Message)
	out.Set("revision.authorName", in.AuthorName)
	out.Set("revision.authorEmail", in.AuthorEmail)
	out.Set("revision.committerName", in.CommitterName)
	out.Set("revision.committerEmail", in.CommitterEmail)
	return nil
}

// AddCustomConversionFuncs adds conversion functions which cannot be automatically generated.
// This is typically due to the objects not having 1:1 field mappings.
func AddCustomConversionFuncs(scheme *runtime.Scheme) error {
	if err := scheme.AddConversionFunc((*url.Values)(nil), (*buildv1.BinaryBuildRequestOptions)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1_BinaryBuildRequestOptions(a.(*url.Values), b.(*buildv1.BinaryBuildRequestOptions), scope)
	}); err != nil {
		return err
	}
	if err := scheme.AddConversionFunc((*buildv1.BinaryBuildRequestOptions)(nil), (*url.Values)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_BinaryBuildRequestOptions_To_url_Values(a.(*buildv1.BinaryBuildRequestOptions), b.(*url.Values), scope)
	}); err != nil {
		return err
	}
	if err := scheme.AddConversionFunc((*buildv1.BuildLogOptions)(nil), (*url.Values)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1_BuildLogOptions(a.(*url.Values), b.(*buildv1.BuildLogOptions), scope)
	}); err != nil {
		return err
	}
	return nil
}

func AddFieldSelectorKeyConversions(scheme *runtime.Scheme) error {
	return scheme.AddFieldLabelConversionFunc(buildv1.GroupVersion.WithKind("Build"), buildFieldSelectorKeyConversionFunc)
}

func buildFieldSelectorKeyConversionFunc(label, value string) (internalLabel, internalValue string, err error) {
	switch label {
	case "status",
		"podName":
		return label, value, nil
	default:
		return runtime.DefaultMetaV1FieldSelectorConversion(label, value)
	}
}
