package validation

import "k8s.io/apimachinery/pkg/util/validation/field"

// ValidateCertificateAuthority is an exported wrapper around validateCertificateAuthority
func ValidateCertificateAuthority(certificateAuthority string, fldPath *field.Path) field.ErrorList {
	return validateCertificateAuthority(certificateAuthority, fldPath)
}
