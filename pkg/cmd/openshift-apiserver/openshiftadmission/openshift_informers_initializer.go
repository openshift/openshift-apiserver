package openshiftadmission

import "k8s.io/apiserver/pkg/admission"

type openshiftInformersInitializer struct {
	informerAccess InformerAccess
}

func NewOpenShiftInformersInitializer(access InformerAccess) *openshiftInformersInitializer {
	return &openshiftInformersInitializer{informerAccess: access}
}

type WantsOpenShiftInformerAccess interface {
	SetOpenShiftInformerAccess(access InformerAccess)
}

func (i *openshiftInformersInitializer) Initialize(plugin admission.Interface) {
	if wants, ok := plugin.(WantsOpenShiftInformerAccess); ok {
		wants.SetOpenShiftInformerAccess(i.informerAccess)
	}
}
