package validation

import (
	"context"
	"fmt"
	"strings"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/authentication/user"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/library-go/pkg/authorization/authorizationutil"
	routecommon "github.com/openshift/library-go/pkg/route"
)

var validateRouteName = apimachineryvalidation.NameIsDNSSubdomain

func ValidateRoute(ctx context.Context, route *routev1.Route, sarc routecommon.SubjectAccessReviewCreator, secrets corev1client.SecretsGetter, opts routecommon.RouteValidationOptions) field.ErrorList {
	return validateRoute(ctx, route, true, sarc, secrets, opts)
}

// validLabels - used in the ValidateRouteUpdate function to check if "older" routes conform to DNS1123Labels or not
func validLabels(host string) bool {
	if len(host) == 0 {
		return true
	}
	return checkLabelSegments(host)
}

// checkLabelSegments - function that checks if hostname labels conform to DNS1123Labels
func checkLabelSegments(host string) bool {
	segments := strings.Split(host, ".")
	for _, s := range segments {
		errs := kvalidation.IsDNS1123Label(s)
		if len(errs) > 0 {
			return false
		}
	}
	return true
}

// validateRoute - private function to validate route
func validateRoute(ctx context.Context, route *routev1.Route, checkHostname bool, sarc routecommon.SubjectAccessReviewCreator, secrets corev1client.SecretsGetter, opts routecommon.RouteValidationOptions) field.ErrorList {
	//ensure meta is set properly
	result := validateObjectMeta(&route.ObjectMeta, true, validateRouteName, field.NewPath("metadata"))

	specPath := field.NewPath("spec")

	//host is not required but if it is set ensure it meets DNS requirements
	if len(route.Spec.Host) > 0 {
		if len(kvalidation.IsDNS1123Subdomain(route.Spec.Host)) != 0 {
			result = append(result, field.Invalid(specPath.Child("host"), route.Spec.Host, "host must conform to DNS 952 subdomain conventions"))
		}

		// Check the hostname only if the old route did not have an invalid DNS1123Label
		// and the new route cares about DNS compliant labels.
		if checkHostname && route.Annotations[routev1.AllowNonDNSCompliantHostAnnotation] != "true" {
			segments := strings.Split(route.Spec.Host, ".")
			for _, s := range segments {
				errs := kvalidation.IsDNS1123Label(s)
				for _, e := range errs {
					result = append(result, field.Invalid(specPath.Child("host"), route.Spec.Host, e))
				}
			}
		}
	}

	if len(route.Spec.Subdomain) > 0 {
		// Subdomain is not lenient because it was never used outside of
		// routes.
		//
		// TODO: Use ValidateSubdomain from library-go.
		if len(route.Spec.Subdomain) > kvalidation.DNS1123SubdomainMaxLength {
			result = append(result, field.Invalid(field.NewPath("spec.subdomain"), route.Spec.Subdomain, kvalidation.MaxLenError(kvalidation.DNS1123SubdomainMaxLength)))
		}
		for _, label := range strings.Split(route.Spec.Subdomain, ".") {
			if errs := kvalidation.IsDNS1123Label(label); len(errs) > 0 {
				result = append(result, field.Invalid(field.NewPath("spec.subdomain"), label, strings.Join(errs, ", ")))
			}
		}
	}

	if err := validateWildcardPolicy(route.Spec.Host, route.Spec.WildcardPolicy, specPath.Child("wildcardPolicy")); err != nil {
		result = append(result, err)
	}

	if len(route.Spec.Path) > 0 && !strings.HasPrefix(route.Spec.Path, "/") {
		result = append(result, field.Invalid(specPath.Child("path"), route.Spec.Path, "path must begin with /"))
	}

	if len(route.Spec.Path) > 0 && route.Spec.TLS != nil &&
		route.Spec.TLS.Termination == routev1.TLSTerminationPassthrough {
		result = append(result, field.Invalid(specPath.Child("path"), route.Spec.Path, "passthrough termination does not support paths"))
	}

	if len(route.Spec.To.Name) == 0 {
		result = append(result, field.Required(specPath.Child("to", "name"), ""))
	}
	if route.Spec.To.Kind != "Service" {
		result = append(result, field.Invalid(specPath.Child("to", "kind"), route.Spec.To.Kind, "must reference a Service"))
	}
	if route.Spec.To.Weight != nil && (*route.Spec.To.Weight < 0 || *route.Spec.To.Weight > 256) {
		result = append(result, field.Invalid(specPath.Child("to", "weight"), route.Spec.To.Weight, "weight must be an integer between 0 and 256"))
	}

	backendPath := specPath.Child("alternateBackends")
	if len(route.Spec.AlternateBackends) > 3 {
		result = append(result, field.Required(backendPath, "cannot specify more than 3 alternate backends"))
	}
	for i, svc := range route.Spec.AlternateBackends {
		if len(svc.Name) == 0 {
			result = append(result, field.Required(backendPath.Index(i).Child("name"), ""))
		}
		if svc.Kind != "Service" {
			result = append(result, field.Invalid(backendPath.Index(i).Child("kind"), svc.Kind, "must reference a Service"))
		}
		if svc.Weight != nil && (*svc.Weight < 0 || *svc.Weight > 256) {
			result = append(result, field.Invalid(backendPath.Index(i).Child("weight"), svc.Weight, "weight must be an integer between 0 and 256"))
		}
	}

	if route.Spec.Port != nil {
		switch target := route.Spec.Port.TargetPort; {
		case target.Type == intstr.Int && target.IntVal == 0,
			target.Type == intstr.String && len(target.StrVal) == 0:
			result = append(result, field.Required(specPath.Child("port", "targetPort"), ""))
		}
	}

	if errs := validateTLS(ctx, route, specPath.Child("tls"), sarc, secrets, opts); len(errs) != 0 {
		result = append(result, errs...)
	}

	return result
}

func ValidateRouteUpdate(ctx context.Context, route *routev1.Route, older *routev1.Route, sarc routecommon.SubjectAccessReviewCreator, secrets corev1client.SecretsGetter, opts routecommon.RouteValidationOptions) field.ErrorList {
	allErrs := validateObjectMetaUpdate(&route.ObjectMeta, &older.ObjectMeta, field.NewPath("metadata"))
	allErrs = append(allErrs, apimachineryvalidation.ValidateImmutableField(route.Spec.WildcardPolicy, older.Spec.WildcardPolicy, field.NewPath("spec", "wildcardPolicy"))...)
	hostnameUpdated := route.Spec.Host != older.Spec.Host
	allErrs = append(allErrs, validateRoute(ctx, route, hostnameUpdated && validLabels(older.Spec.Host), sarc, secrets, opts)...)
	return allErrs
}

// ValidateRouteStatusUpdate validates status updates for routes.
//
// Note that this function shouldn't call ValidateRouteUpdate, otherwise
// we are risking to break existing routes.
func ValidateRouteStatusUpdate(route *routev1.Route, older *routev1.Route) field.ErrorList {
	allErrs := validateObjectMetaUpdate(&route.ObjectMeta, &older.ObjectMeta, field.NewPath("metadata"))

	// TODO: validate route status
	return allErrs
}

// validateTLS tests fields for different types of TLS combinations are set.  Called
// by ValidateRoute.
func validateTLS(ctx context.Context, route *routev1.Route, fldPath *field.Path, sarc routecommon.SubjectAccessReviewCreator, secrets corev1client.SecretsGetter, opts routecommon.RouteValidationOptions) field.ErrorList {
	result := field.ErrorList{}
	tls := route.Spec.TLS

	// no tls config present, no need for validation
	if tls == nil {
		return nil
	}

	switch tls.Termination {
	// reencrypt may specify destination ca cert
	// externalCert, cert, key, cacert may not be specified because the route may be a wildcard
	case routev1.TLSTerminationReencrypt:
		if opts.AllowExternalCertificates && tls.ExternalCertificate != nil {
			if len(tls.Certificate) > 0 && len(tls.ExternalCertificate.Name) > 0 {
				result = append(result, field.Invalid(fldPath.Child("externalCertificate"), tls.ExternalCertificate.Name, "cannot specify both tls.certificate and tls.externalCertificate"))
			}
		}
	//passthrough term should not specify any cert
	case routev1.TLSTerminationPassthrough:
		if len(tls.Certificate) > 0 {
			result = append(result, field.Invalid(fldPath.Child("certificate"), "redacted certificate data", "passthrough termination does not support certificates"))
		}

		if len(tls.Key) > 0 {
			result = append(result, field.Invalid(fldPath.Child("key"), "redacted key data", "passthrough termination does not support certificates"))
		}

		if opts.AllowExternalCertificates && tls.ExternalCertificate != nil {
			if len(tls.ExternalCertificate.Name) > 0 {
				result = append(result, field.Invalid(fldPath.Child("externalCertificate"), tls.ExternalCertificate.Name, "passthrough termination does not support certificates"))
			}
		}

		if len(tls.CACertificate) > 0 {
			result = append(result, field.Invalid(fldPath.Child("caCertificate"), "redacted ca certificate data", "passthrough termination does not support certificates"))
		}

		if len(tls.DestinationCACertificate) > 0 {
			result = append(result, field.Invalid(fldPath.Child("destinationCACertificate"), "redacted destination ca certificate data", "passthrough termination does not support certificates"))
		}
	// edge cert should only specify cert, key, and cacert but those certs
	// may not be specified if the route is a wildcard route
	case routev1.TLSTerminationEdge:
		if len(tls.DestinationCACertificate) > 0 {
			result = append(result, field.Invalid(fldPath.Child("destinationCACertificate"), "redacted destination ca certificate data", "edge termination does not support destination certificates"))
		}

		if opts.AllowExternalCertificates && tls.ExternalCertificate != nil {
			if len(tls.Certificate) > 0 && len(tls.ExternalCertificate.Name) > 0 {
				result = append(result, field.Invalid(fldPath.Child("externalCertificate"), tls.ExternalCertificate.Name, "cannot specify both tls.certificate and tls.externalCertificate"))
				break
			} else if len(tls.ExternalCertificate.Name) > 0 {
				errs := validateTLSExternalCertificate(ctx, route, fldPath.Child("externalCertificate"), sarc, secrets)
				result = append(result, errs...)
			}
		}

	default:
		validValues := []string{string(routev1.TLSTerminationEdge), string(routev1.TLSTerminationPassthrough), string(routev1.TLSTerminationReencrypt)}
		result = append(result, field.NotSupported(fldPath.Child("termination"), tls.Termination, validValues))
	}

	if err := validateInsecureEdgeTerminationPolicy(tls, fldPath.Child("insecureEdgeTerminationPolicy")); err != nil {
		result = append(result, err)
	}

	return result
}

func validateTLSExternalCertificate(ctx context.Context, route *routev1.Route, fldPath *field.Path, sarc routecommon.SubjectAccessReviewCreator, secrets corev1client.SecretsGetter) field.ErrorList {
	tls := route.Spec.TLS
	var errs field.ErrorList

	errs = append(errs, routecommon.CheckRouteCustomHostSAR(ctx, fldPath, sarc)...)

	if err := authorizationutil.Authorize(sarc, &user.DefaultInfo{Name: "system:serviceaccount:openshift-ingress:router"},
		&authorizationv1.ResourceAttributes{
			Namespace: route.Namespace,
			Verb:      "get",
			Resource:  "secrets",
			Name:      tls.ExternalCertificate.Name,
		}); err != nil {
		errs = append(errs, field.Forbidden(fldPath, "router serviceaccount does not have permission to get this secret"))
	}

	if err := authorizationutil.Authorize(sarc, &user.DefaultInfo{Name: "system:serviceaccount:openshift-ingress:router"},
		&authorizationv1.ResourceAttributes{
			Namespace: route.Namespace,
			Verb:      "watch",
			Resource:  "secrets",
			Name:      tls.ExternalCertificate.Name,
		}); err != nil {
		errs = append(errs, field.Forbidden(fldPath, "router serviceaccount does not have permission to watch this secret"))
	}

	if err := authorizationutil.Authorize(sarc, &user.DefaultInfo{Name: "system:serviceaccount:openshift-ingress:router"},
		&authorizationv1.ResourceAttributes{
			Namespace: route.Namespace,
			Verb:      "list",
			Resource:  "secrets",
			Name:      tls.ExternalCertificate.Name,
		}); err != nil {
		errs = append(errs, field.Forbidden(fldPath, "router serviceaccount does not have permission to list this secret"))
	}

	secret, err := secrets.Secrets(route.Namespace).Get(ctx, tls.ExternalCertificate.Name, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		errs = append(errs, field.NotFound(fldPath, err))
		return errs
	} else if err != nil {
		errs = append(errs, field.InternalError(fldPath, err))
		return errs
	}

	if secret.Type != corev1.SecretTypeTLS {
		errs = append(errs, field.Invalid(fldPath, tls.ExternalCertificate.Name, "secret of type 'kubernetes/tls' required"))
	}

	return errs
}

// validateInsecureEdgeTerminationPolicy tests fields for different types of
// insecure options. Called by validateTLS.
func validateInsecureEdgeTerminationPolicy(tls *routev1.TLSConfig, fldPath *field.Path) *field.Error {
	// Check insecure option value if specified (empty is ok).
	if len(tls.InsecureEdgeTerminationPolicy) == 0 {
		return nil
	}

	// It is an edge-terminated or reencrypt route, check insecure option value is
	// one of None(for disable), Allow or Redirect.
	allowedValues := map[routev1.InsecureEdgeTerminationPolicyType]struct{}{
		routev1.InsecureEdgeTerminationPolicyNone:     {},
		routev1.InsecureEdgeTerminationPolicyAllow:    {},
		routev1.InsecureEdgeTerminationPolicyRedirect: {},
	}

	switch tls.Termination {
	case routev1.TLSTerminationReencrypt:
		fallthrough
	case routev1.TLSTerminationEdge:
		if _, ok := allowedValues[tls.InsecureEdgeTerminationPolicy]; !ok {
			msg := fmt.Sprintf("invalid value for InsecureEdgeTerminationPolicy option, acceptable values are %s, %s, %s, or empty", routev1.InsecureEdgeTerminationPolicyNone, routev1.InsecureEdgeTerminationPolicyAllow, routev1.InsecureEdgeTerminationPolicyRedirect)
			return field.Invalid(fldPath, tls.InsecureEdgeTerminationPolicy, msg)
		}
	case routev1.TLSTerminationPassthrough:
		if routev1.InsecureEdgeTerminationPolicyNone != tls.InsecureEdgeTerminationPolicy && routev1.InsecureEdgeTerminationPolicyRedirect != tls.InsecureEdgeTerminationPolicy {
			msg := fmt.Sprintf("invalid value for InsecureEdgeTerminationPolicy option, acceptable values are %s, %s, or empty", routev1.InsecureEdgeTerminationPolicyNone, routev1.InsecureEdgeTerminationPolicyRedirect)
			return field.Invalid(fldPath, tls.InsecureEdgeTerminationPolicy, msg)
		}
	}

	return nil
}

var (
	allowedWildcardPolicies    = []string{string(routev1.WildcardPolicyNone), string(routev1.WildcardPolicySubdomain)}
	allowedWildcardPoliciesSet = sets.NewString(allowedWildcardPolicies...)
)

// validateWildcardPolicy tests that the wildcard policy is either empty or one of the supported types.
func validateWildcardPolicy(host string, policy routev1.WildcardPolicyType, fldPath *field.Path) *field.Error {
	if len(policy) == 0 {
		return nil
	}

	// Check if policy is one of None or Subdomain.
	if !allowedWildcardPoliciesSet.Has(string(policy)) {
		return field.NotSupported(fldPath, policy, allowedWildcardPolicies)
	}

	if policy == routev1.WildcardPolicySubdomain && len(host) == 0 {
		return field.Invalid(fldPath, policy, "host name not specified for wildcard policy")
	}

	return nil
}

// The special finalizer name validations were copied from k8s.io/kubernetes to eliminate that
// dependency and preserve the existing behavior.

// k8s.io/kubernetes/pkg/apis/core/validation.ValidateObjectMeta
func validateObjectMeta(meta *metav1.ObjectMeta, requiresNamespace bool, nameFn apimachineryvalidation.ValidateNameFunc, fldPath *field.Path) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMeta(meta, requiresNamespace, apimachineryvalidation.ValidateNameFunc(nameFn), fldPath)
	// run additional checks for the finalizer name
	for i := range meta.Finalizers {
		allErrs = append(allErrs, validateKubeFinalizerName(string(meta.Finalizers[i]), fldPath.Child("finalizers").Index(i))...)
	}
	return allErrs
}

// k8s.io/kubernetes/pkg/apis/core/validation.ValidateObjectMetaUpdate
func validateObjectMetaUpdate(newMeta, oldMeta *metav1.ObjectMeta, fldPath *field.Path) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMetaUpdate(newMeta, oldMeta, fldPath)
	// run additional checks for the finalizer name
	for i := range newMeta.Finalizers {
		allErrs = append(allErrs, validateKubeFinalizerName(string(newMeta.Finalizers[i]), fldPath.Child("finalizers").Index(i))...)
	}

	return allErrs
}

var standardFinalizers = sets.NewString(
	string(corev1.FinalizerKubernetes),
	metav1.FinalizerOrphanDependents,
	metav1.FinalizerDeleteDependents,
)

func validateKubeFinalizerName(stringValue string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(strings.Split(stringValue, "/")) == 1 {
		if !standardFinalizers.Has(stringValue) {
			return append(allErrs, field.Invalid(fldPath, stringValue, "name is neither a standard finalizer name nor is it fully qualified"))
		}
	}

	return allErrs
}

func Warnings(route *routev1.Route) []string {
	if len(route.Spec.Host) != 0 && len(route.Spec.Subdomain) != 0 {
		var warnings []string
		warnings = append(warnings, "spec.host is set; spec.subdomain may be ignored")
		return warnings
	}
	return nil
}
