package util

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	apistorage "k8s.io/apiserver/pkg/storage"
	kapi "k8s.io/kubernetes/pkg/apis/core"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"

	projectv1 "github.com/openshift/api/project/v1"
	oapi "github.com/openshift/openshift-apiserver/pkg/api"
	projectapi "github.com/openshift/openshift-apiserver/pkg/project/apis/project"
	projectinternalv1 "github.com/openshift/openshift-apiserver/pkg/project/apis/project/v1"
)

// ConvertNamespaceFromExternal transforms a versioned Namespace into a Project
func ConvertNamespaceFromExternal(namespace *corev1.Namespace) *projectapi.Project {
	internalFinalizers := []kapi.FinalizerName{}
	for _, externalFinalizer := range namespace.Spec.Finalizers {
		internalFinalizers = append(internalFinalizers, kapi.FinalizerName(externalFinalizer))
	}

	p := &projectapi.Project{
		ObjectMeta: namespace.ObjectMeta,
		Spec: projectapi.ProjectSpec{
			Finalizers: internalFinalizers,
		},
		Status: projectapi.ProjectStatus{
			Phase: kapi.NamespacePhase(namespace.Status.Phase),
		},
	}
	if namespace.Status.Conditions != nil {
		p.Status.Conditions = []kapi.NamespaceCondition{}
		status := &kapi.NamespaceStatus{}
		if err := v1.Convert_v1_NamespaceStatus_To_core_NamespaceStatus(&namespace.Status, status, nil); err != nil {
			panic(err)
		}
		for _, c := range status.Conditions {
			p.Status.Conditions = append(p.Status.Conditions, c)
		}
	}

	return p
}

func ConvertProjectToExternal(project *projectapi.Project) *corev1.Namespace {
	externalFinalizers := []corev1.FinalizerName{}
	for _, internalFinalizer := range project.Spec.Finalizers {
		externalFinalizers = append(externalFinalizers, corev1.FinalizerName(internalFinalizer))
	}

	namespace := &corev1.Namespace{
		ObjectMeta: project.ObjectMeta,
		Spec: corev1.NamespaceSpec{
			Finalizers: externalFinalizers,
		},
		Status: corev1.NamespaceStatus{
			Phase: corev1.NamespacePhase(project.Status.Phase),
		},
	}
	if project.Status.Conditions != nil {
		namespace.Status.Conditions = []corev1.NamespaceCondition{}
		status := &projectv1.ProjectStatus{}
		if err := projectinternalv1.Convert_project_ProjectStatus_To_v1_ProjectStatus(&project.Status, status, nil); err != nil {
			panic(err)
		}
		for _, c := range status.Conditions {
			namespace.Status.Conditions = append(namespace.Status.Conditions, c)
		}
	}
	if namespace.Annotations == nil {
		namespace.Annotations = map[string]string{}
	}
	namespace.Annotations[oapi.OpenShiftDisplayName] = project.Annotations[oapi.OpenShiftDisplayName]
	return namespace
}

// ConvertNamespaceList transforms a NamespaceList into a ProjectList
func ConvertNamespaceList(namespaceList *corev1.NamespaceList) *projectapi.ProjectList {
	projects := &projectapi.ProjectList{}
	for _, n := range namespaceList.Items {
		projects.Items = append(projects.Items, *ConvertNamespaceFromExternal(&n))
	}
	return projects
}

// getAttrs returns labels and fields of a given object for filtering purposes.
func getAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	projectObj, ok := obj.(*projectapi.Project)
	if !ok {
		return nil, nil, fmt.Errorf("not a project")
	}
	return labels.Set(projectObj.Labels), projectToSelectableFields(projectObj), nil
}

// MatchProject returns a generic matcher for a given label and field selector.
func MatchProject(label labels.Selector, field fields.Selector) apistorage.SelectionPredicate {
	return apistorage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: getAttrs,
	}
}

// projectToSelectableFields returns a field set that represents the object
func projectToSelectableFields(projectObj *projectapi.Project) fields.Set {
	objectMetaFieldsSet := generic.ObjectMetaFieldsSet(&projectObj.ObjectMeta, false)
	specificFieldsSet := fields.Set{
		"status.phase": string(projectObj.Status.Phase),
	}
	return generic.MergeFieldsSets(objectMetaFieldsSet, specificFieldsSet)
}
