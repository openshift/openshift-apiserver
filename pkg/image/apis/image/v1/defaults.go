package v1

import (
	v1 "k8s.io/api/core/v1"

	imagev1 "github.com/openshift/api/image/v1"
	apisimage "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/library-go/pkg/image/reference"
)

func SetDefaults_ImageImportSpec(obj *imagev1.ImageImportSpec) {
	if obj.To == nil {
		if ref, err := reference.Parse(obj.From.Name); err == nil {
			if len(ref.Tag) > 0 {
				obj.To = &v1.LocalObjectReference{Name: ref.Tag}
			}
		}
	}
}

func SetDefaults_TagReferencePolicy(obj *imagev1.TagReferencePolicy) {
	if len(obj.Type) == 0 {
		obj.Type = imagev1.SourceTagReferencePolicy
	}
}

func SetDefaults_TagImportPolicy(obj *imagev1.TagImportPolicy) {
	if len(obj.ImportMode) == 0 {
		obj.ImportMode = imagev1.ImportModeType(apisimage.DefaultImportMode)
	}
}
