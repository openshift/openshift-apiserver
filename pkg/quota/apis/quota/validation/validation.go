package validation

import (
	quotaapi "github.com/openshift/openshift-apiserver/pkg/quota/apis/quota"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/validation"
)

func ValidateClusterResourceQuota(clusterquota *quotaapi.ClusterResourceQuota) field.ErrorList {
	allErrs := validation.ValidateObjectMeta(&clusterquota.ObjectMeta, false, validation.ValidateResourceQuotaName, field.NewPath("metadata"))

	hasSelectionCriteria := (clusterquota.Spec.Selector.LabelSelector != nil && len(clusterquota.Spec.Selector.LabelSelector.MatchLabels)+len(clusterquota.Spec.Selector.LabelSelector.MatchExpressions) > 0) ||
		(len(clusterquota.Spec.Selector.AnnotationSelector) > 0)

	if !hasSelectionCriteria {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "selector"), "must restrict the selected projects"))
	}
	if clusterquota.Spec.Selector.LabelSelector != nil {
		allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(clusterquota.Spec.Selector.LabelSelector, field.NewPath("spec", "selector", "labels"))...)
		if len(clusterquota.Spec.Selector.LabelSelector.MatchLabels)+len(clusterquota.Spec.Selector.LabelSelector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "selector", "labels"), clusterquota.Spec.Selector.LabelSelector, "must restrict the selected projects"))
		}
	}
	if clusterquota.Spec.Selector.AnnotationSelector != nil {
		allErrs = append(allErrs, validation.ValidateAnnotations(clusterquota.Spec.Selector.AnnotationSelector, field.NewPath("spec", "selector", "annotations"))...)
	}

	allErrs = append(allErrs, validation.ValidateResourceQuotaSpec(&clusterquota.Spec.Quota, field.NewPath("spec", "quota"))...)
	allErrs = append(allErrs, validation.ValidateResourceQuotaStatus(&clusterquota.Status.Total, field.NewPath("status", "overall"))...)

	for e := clusterquota.Status.Namespaces.OrderedKeys().Front(); e != nil; e = e.Next() {
		namespace := e.Value.(string)
		used, _ := clusterquota.Status.Namespaces.Get(namespace)

		fldPath := field.NewPath("status", "namespaces").Key(namespace)
		for k, v := range used.Used {
			resPath := fldPath.Key(string(k))
			allErrs = append(allErrs, validation.ValidateResourceQuotaResourceName(string(k), resPath)...)
			allErrs = append(allErrs, validation.ValidateResourceQuantityValue(string(k), v, resPath)...)
		}
	}

	return allErrs
}

func ValidateClusterResourceQuotaUpdate(clusterquota, oldClusterResourceQuota *quotaapi.ClusterResourceQuota) field.ErrorList {
	allErrs := validation.ValidateObjectMetaUpdate(&clusterquota.ObjectMeta, &oldClusterResourceQuota.ObjectMeta, field.NewPath("metadata"))
	allErrs = append(allErrs, ValidateClusterResourceQuota(clusterquota)...)

	return allErrs
}

func ValidateAppliedClusterResourceQuota(clusterquota *quotaapi.AppliedClusterResourceQuota) field.ErrorList {
	return ValidateClusterResourceQuota(quotaapi.ConvertAppliedClusterResourceQuotaToClusterResourceQuota(clusterquota))
}

func ValidateAppliedClusterResourceQuotaUpdate(clusterquota, oldClusterResourceQuota *quotaapi.AppliedClusterResourceQuota) field.ErrorList {
	return ValidateClusterResourceQuotaUpdate(
		quotaapi.ConvertAppliedClusterResourceQuotaToClusterResourceQuota(clusterquota),
		quotaapi.ConvertAppliedClusterResourceQuotaToClusterResourceQuota(oldClusterResourceQuota))
}

func hasCrossNamespacePodAffinityScope(spec *core.ResourceQuotaSpec) bool {
	if spec == nil {
		return false
	}
	for _, scope := range spec.Scopes {
		if scope == core.ResourceQuotaScopeCrossNamespacePodAffinity {
			return true
		}
	}

	if spec.ScopeSelector == nil {
		return false
	}
	for _, req := range spec.ScopeSelector.MatchExpressions {
		if req.ScopeName == core.ResourceQuotaScopeCrossNamespacePodAffinity {
			return true
		}
	}
	return false
}
