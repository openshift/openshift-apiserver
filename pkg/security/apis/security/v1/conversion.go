package v1

import (
	"k8s.io/apimachinery/pkg/conversion"

	v1 "github.com/openshift/api/security/v1"
	securityapi "github.com/openshift/openshift-apiserver/pkg/security/apis/security"
)

func Convert_v1_SecurityContextConstraints_To_security_SecurityContextConstraints(in *v1.SecurityContextConstraints, out *securityapi.SecurityContextConstraints, s conversion.Scope) error {
	return autoConvert_v1_SecurityContextConstraints_To_security_SecurityContextConstraints(in, out, s)
}

func Convert_v1_RunAsGroupStrategyOptions_To_security_RunAsGroupStrategyOptions(in *v1.RunAsGroupStrategyOptions, out *securityapi.RunAsGroupStrategyOptions, s conversion.Scope) error {
	return autoConvert_v1_RunAsGroupStrategyOptions_To_security_RunAsGroupStrategyOptions(in, out, s)
}

func Convert_security_RunAsGroupStrategyOptions_To_v1_RunAsGroupStrategyOptions(in *securityapi.RunAsGroupStrategyOptions, out *v1.RunAsGroupStrategyOptions, s conversion.Scope) error {
	return autoConvert_security_RunAsGroupStrategyOptions_To_v1_RunAsGroupStrategyOptions(in, out, s)
}

func Convert_security_SecurityContextConstraints_To_v1_SecurityContextConstraints(in *securityapi.SecurityContextConstraints, out *v1.SecurityContextConstraints, s conversion.Scope) error {
	if err := autoConvert_security_SecurityContextConstraints_To_v1_SecurityContextConstraints(in, out, s); err != nil {
		return err
	}

	if in.Volumes != nil {
		for _, v := range in.Volumes {
			// set the Allow* fields based on the existence in the volume slice
			switch v {
			case securityapi.FSTypeHostPath, securityapi.FSTypeAll:
				out.AllowHostDirVolumePlugin = true
			}
		}
	}
	return nil
}

// Convert_v1_IDRange_To_security_RunAsGroupIDRange converts v1.IDRange to internal RunAsGroupIDRange
func Convert_v1_IDRange_To_security_RunAsGroupIDRange(in *v1.IDRange, out *securityapi.RunAsGroupIDRange, s conversion.Scope) error {
	out.Min = &in.Min
	out.Max = &in.Max
	return nil
}

// Convert_security_RunAsGroupIDRange_To_v1_IDRange converts internal RunAsGroupIDRange to v1.IDRange
func Convert_security_RunAsGroupIDRange_To_v1_IDRange(in *securityapi.RunAsGroupIDRange, out *v1.IDRange, s conversion.Scope) error {
	if in.Min != nil {
		out.Min = *in.Min
	}
	if in.Max != nil {
		out.Max = *in.Max
	}
	return nil
}
