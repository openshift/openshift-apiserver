//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Code generated by defaulter-gen. DO NOT EDIT.

package v1

import (
	buildv1 "github.com/openshift/api/build/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
)

// RegisterDefaults adds defaulters functions to the given scheme.
// Public to allow building arbitrary schemes.
// All generated defaulters are covering - they call all nested defaulters.
func RegisterDefaults(scheme *runtime.Scheme) error {
	scheme.AddTypeDefaultingFunc(&buildv1.Build{}, func(obj interface{}) { SetObjectDefaults_Build(obj.(*buildv1.Build)) })
	scheme.AddTypeDefaultingFunc(&buildv1.BuildConfig{}, func(obj interface{}) { SetObjectDefaults_BuildConfig(obj.(*buildv1.BuildConfig)) })
	scheme.AddTypeDefaultingFunc(&buildv1.BuildConfigList{}, func(obj interface{}) { SetObjectDefaults_BuildConfigList(obj.(*buildv1.BuildConfigList)) })
	scheme.AddTypeDefaultingFunc(&buildv1.BuildList{}, func(obj interface{}) { SetObjectDefaults_BuildList(obj.(*buildv1.BuildList)) })
	scheme.AddTypeDefaultingFunc(&buildv1.BuildRequest{}, func(obj interface{}) { SetObjectDefaults_BuildRequest(obj.(*buildv1.BuildRequest)) })
	return nil
}

func SetObjectDefaults_Build(in *buildv1.Build) {
	SetDefaults_BuildSource(&in.Spec.CommonSpec.Source)
	SetDefaults_BuildStrategy(&in.Spec.CommonSpec.Strategy)
	if in.Spec.CommonSpec.Strategy.DockerStrategy != nil {
		SetDefaults_DockerBuildStrategy(in.Spec.CommonSpec.Strategy.DockerStrategy)
		for i := range in.Spec.CommonSpec.Strategy.DockerStrategy.Env {
			a := &in.Spec.CommonSpec.Strategy.DockerStrategy.Env[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
		for i := range in.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs {
			a := &in.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
		for i := range in.Spec.CommonSpec.Strategy.DockerStrategy.Volumes {
			a := &in.Spec.CommonSpec.Strategy.DockerStrategy.Volumes[i]
			if a.Source.Secret != nil {
				corev1.SetDefaults_SecretVolumeSource(a.Source.Secret)
			}
			if a.Source.ConfigMap != nil {
				corev1.SetDefaults_ConfigMapVolumeSource(a.Source.ConfigMap)
			}
		}
	}
	if in.Spec.CommonSpec.Strategy.SourceStrategy != nil {
		SetDefaults_SourceBuildStrategy(in.Spec.CommonSpec.Strategy.SourceStrategy)
		for i := range in.Spec.CommonSpec.Strategy.SourceStrategy.Env {
			a := &in.Spec.CommonSpec.Strategy.SourceStrategy.Env[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
		for i := range in.Spec.CommonSpec.Strategy.SourceStrategy.Volumes {
			a := &in.Spec.CommonSpec.Strategy.SourceStrategy.Volumes[i]
			if a.Source.Secret != nil {
				corev1.SetDefaults_SecretVolumeSource(a.Source.Secret)
			}
			if a.Source.ConfigMap != nil {
				corev1.SetDefaults_ConfigMapVolumeSource(a.Source.ConfigMap)
			}
		}
	}
	if in.Spec.CommonSpec.Strategy.CustomStrategy != nil {
		SetDefaults_CustomBuildStrategy(in.Spec.CommonSpec.Strategy.CustomStrategy)
		for i := range in.Spec.CommonSpec.Strategy.CustomStrategy.Env {
			a := &in.Spec.CommonSpec.Strategy.CustomStrategy.Env[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
	}
	if in.Spec.CommonSpec.Strategy.JenkinsPipelineStrategy != nil {
		for i := range in.Spec.CommonSpec.Strategy.JenkinsPipelineStrategy.Env {
			a := &in.Spec.CommonSpec.Strategy.JenkinsPipelineStrategy.Env[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
	}
	corev1.SetDefaults_ResourceList(&in.Spec.CommonSpec.Resources.Limits)
	corev1.SetDefaults_ResourceList(&in.Spec.CommonSpec.Resources.Requests)
}

func SetObjectDefaults_BuildConfig(in *buildv1.BuildConfig) {
	SetDefaults_BuildConfigSpec(&in.Spec)
	for i := range in.Spec.Triggers {
		a := &in.Spec.Triggers[i]
		SetDefaults_BuildTriggerPolicy(a)
	}
	SetDefaults_BuildSource(&in.Spec.CommonSpec.Source)
	SetDefaults_BuildStrategy(&in.Spec.CommonSpec.Strategy)
	if in.Spec.CommonSpec.Strategy.DockerStrategy != nil {
		SetDefaults_DockerBuildStrategy(in.Spec.CommonSpec.Strategy.DockerStrategy)
		for i := range in.Spec.CommonSpec.Strategy.DockerStrategy.Env {
			a := &in.Spec.CommonSpec.Strategy.DockerStrategy.Env[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
		for i := range in.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs {
			a := &in.Spec.CommonSpec.Strategy.DockerStrategy.BuildArgs[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
		for i := range in.Spec.CommonSpec.Strategy.DockerStrategy.Volumes {
			a := &in.Spec.CommonSpec.Strategy.DockerStrategy.Volumes[i]
			if a.Source.Secret != nil {
				corev1.SetDefaults_SecretVolumeSource(a.Source.Secret)
			}
			if a.Source.ConfigMap != nil {
				corev1.SetDefaults_ConfigMapVolumeSource(a.Source.ConfigMap)
			}
		}
	}
	if in.Spec.CommonSpec.Strategy.SourceStrategy != nil {
		SetDefaults_SourceBuildStrategy(in.Spec.CommonSpec.Strategy.SourceStrategy)
		for i := range in.Spec.CommonSpec.Strategy.SourceStrategy.Env {
			a := &in.Spec.CommonSpec.Strategy.SourceStrategy.Env[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
		for i := range in.Spec.CommonSpec.Strategy.SourceStrategy.Volumes {
			a := &in.Spec.CommonSpec.Strategy.SourceStrategy.Volumes[i]
			if a.Source.Secret != nil {
				corev1.SetDefaults_SecretVolumeSource(a.Source.Secret)
			}
			if a.Source.ConfigMap != nil {
				corev1.SetDefaults_ConfigMapVolumeSource(a.Source.ConfigMap)
			}
		}
	}
	if in.Spec.CommonSpec.Strategy.CustomStrategy != nil {
		SetDefaults_CustomBuildStrategy(in.Spec.CommonSpec.Strategy.CustomStrategy)
		for i := range in.Spec.CommonSpec.Strategy.CustomStrategy.Env {
			a := &in.Spec.CommonSpec.Strategy.CustomStrategy.Env[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
	}
	if in.Spec.CommonSpec.Strategy.JenkinsPipelineStrategy != nil {
		for i := range in.Spec.CommonSpec.Strategy.JenkinsPipelineStrategy.Env {
			a := &in.Spec.CommonSpec.Strategy.JenkinsPipelineStrategy.Env[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
	}
	corev1.SetDefaults_ResourceList(&in.Spec.CommonSpec.Resources.Limits)
	corev1.SetDefaults_ResourceList(&in.Spec.CommonSpec.Resources.Requests)
}

func SetObjectDefaults_BuildConfigList(in *buildv1.BuildConfigList) {
	for i := range in.Items {
		a := &in.Items[i]
		SetObjectDefaults_BuildConfig(a)
	}
}

func SetObjectDefaults_BuildList(in *buildv1.BuildList) {
	for i := range in.Items {
		a := &in.Items[i]
		SetObjectDefaults_Build(a)
	}
}

func SetObjectDefaults_BuildRequest(in *buildv1.BuildRequest) {
	for i := range in.Env {
		a := &in.Env[i]
		if a.ValueFrom != nil {
			if a.ValueFrom.FieldRef != nil {
				corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
			}
		}
	}
	if in.DockerStrategyOptions != nil {
		for i := range in.DockerStrategyOptions.BuildArgs {
			a := &in.DockerStrategyOptions.BuildArgs[i]
			if a.ValueFrom != nil {
				if a.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(a.ValueFrom.FieldRef)
				}
			}
		}
	}
}
