package validation

import (
	"context"

	routecommon "github.com/openshift/library-go/pkg/route"
	appsvalidation "github.com/openshift/openshift-apiserver/pkg/apps/apis/apps/validation"
	authorizationvalidation "github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization/validation"
	buildvalidation "github.com/openshift/openshift-apiserver/pkg/build/apis/build/validation"
	imagevalidation "github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation"
	projectvalidation "github.com/openshift/openshift-apiserver/pkg/project/apis/project/validation"
	quotavalidation "github.com/openshift/openshift-apiserver/pkg/quota/apis/quota/validation"
	routevalidation "github.com/openshift/openshift-apiserver/pkg/route/apis/route/validation"
	securityvalidation "github.com/openshift/openshift-apiserver/pkg/security/apis/security/validation"
	templatevalidation "github.com/openshift/openshift-apiserver/pkg/template/apis/template/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	appsapi "github.com/openshift/openshift-apiserver/pkg/apps/apis/apps"
	authorizationapi "github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization"
	buildapi "github.com/openshift/openshift-apiserver/pkg/build/apis/build"
	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	projectapi "github.com/openshift/openshift-apiserver/pkg/project/apis/project"
	quotaapi "github.com/openshift/openshift-apiserver/pkg/quota/apis/quota"
	routeapi "github.com/openshift/openshift-apiserver/pkg/route/apis/route"
	securityapi "github.com/openshift/openshift-apiserver/pkg/security/apis/security"
	templateapi "github.com/openshift/openshift-apiserver/pkg/template/apis/template"
)

func init() {
	registerAll()
}

func registerAll() {
	Validator.MustRegister(&authorizationapi.SelfSubjectRulesReview{}, true, authorizationvalidation.ValidateSelfSubjectRulesReview, nil)
	Validator.MustRegister(&authorizationapi.SubjectAccessReview{}, false, authorizationvalidation.ValidateSubjectAccessReview, nil)
	Validator.MustRegister(&authorizationapi.SubjectRulesReview{}, true, authorizationvalidation.ValidateSubjectRulesReview, nil)
	Validator.MustRegister(&authorizationapi.ResourceAccessReview{}, false, authorizationvalidation.ValidateResourceAccessReview, nil)
	Validator.MustRegister(&authorizationapi.LocalSubjectAccessReview{}, true, authorizationvalidation.ValidateLocalSubjectAccessReview, nil)
	Validator.MustRegister(&authorizationapi.LocalResourceAccessReview{}, true, authorizationvalidation.ValidateLocalResourceAccessReview, nil)

	Validator.MustRegister(&authorizationapi.Role{}, true, authorizationvalidation.ValidateLocalRole, authorizationvalidation.ValidateLocalRoleUpdate)
	Validator.MustRegister(&authorizationapi.RoleBinding{}, true, authorizationvalidation.ValidateLocalRoleBinding, authorizationvalidation.ValidateLocalRoleBindingUpdate)
	Validator.MustRegister(&authorizationapi.RoleBindingRestriction{}, true, authorizationvalidation.ValidateRoleBindingRestriction, authorizationvalidation.ValidateRoleBindingRestrictionUpdate)
	Validator.MustRegister(&authorizationapi.ClusterRole{}, false, authorizationvalidation.ValidateClusterRole, authorizationvalidation.ValidateClusterRoleUpdate)
	Validator.MustRegister(&authorizationapi.ClusterRoleBinding{}, false, authorizationvalidation.ValidateClusterRoleBinding, authorizationvalidation.ValidateClusterRoleBindingUpdate)

	Validator.MustRegister(&buildapi.Build{}, true, buildvalidation.ValidateBuild, buildvalidation.ValidateBuildUpdate)
	Validator.MustRegister(&buildapi.BuildConfig{}, true, buildvalidation.ValidateBuildConfig, buildvalidation.ValidateBuildConfigUpdate)
	Validator.MustRegister(&buildapi.BuildRequest{}, true, buildvalidation.ValidateBuildRequest, nil)
	Validator.MustRegister(&buildapi.BuildLogOptions{}, true, buildvalidation.ValidateBuildLogOptions, nil)

	Validator.MustRegister(&appsapi.DeploymentConfig{}, true, appsvalidation.ValidateDeploymentConfig, appsvalidation.ValidateDeploymentConfigUpdate)
	Validator.MustRegister(&appsapi.DeploymentConfigRollback{}, true, appsvalidation.ValidateDeploymentConfigRollback, nil)
	Validator.MustRegister(&appsapi.DeploymentLogOptions{}, true, appsvalidation.ValidateDeploymentLogOptions, nil)
	Validator.MustRegister(&appsapi.DeploymentRequest{}, true, appsvalidation.ValidateDeploymentRequest, nil)

	Validator.MustRegister(&imageapi.Image{}, false, imagevalidation.ValidateImage, imagevalidation.ValidateImageUpdate)
	Validator.MustRegister(&imageapi.ImageSignature{}, false, imagevalidation.ValidateImageSignature, imagevalidation.ValidateImageSignatureUpdate)
	Validator.MustRegister(&imageapi.ImageStream{}, true, imagevalidation.ValidateImageStream, imagevalidation.ValidateImageStreamUpdate)
	Validator.MustRegister(&imageapi.ImageStreamImport{}, true, imagevalidation.ValidateImageStreamImport, nil)
	Validator.MustRegister(&imageapi.ImageStreamMapping{}, true, imagevalidation.ValidateImageStreamMapping, nil)
	Validator.MustRegister(&imageapi.ImageStreamTag{}, true, imagevalidation.ValidateImageStreamTag, imagevalidation.ValidateImageStreamTagUpdate)

	Validator.MustRegister(&projectapi.Project{}, false, projectvalidation.ValidateProject, projectvalidation.ValidateProjectUpdate)
	Validator.MustRegister(&projectapi.ProjectRequest{}, false, projectvalidation.ValidateProjectRequest, nil)

	Validator.MustRegister(&routeapi.Route{}, true, routeOptionsAdapter, routeUpdateOptionsAdapter)

	Validator.MustRegister(&templateapi.Template{}, true, templatevalidation.ValidateTemplate, templatevalidation.ValidateTemplateUpdate)
	Validator.MustRegister(&templateapi.TemplateInstance{}, true, templatevalidation.ValidateTemplateInstance, templatevalidation.ValidateTemplateInstanceUpdate)
	Validator.MustRegister(&templateapi.BrokerTemplateInstance{}, false, templatevalidation.ValidateBrokerTemplateInstance, templatevalidation.ValidateBrokerTemplateInstanceUpdate)

	Validator.MustRegister(&securityapi.SecurityContextConstraints{}, false, securityvalidation.ValidateSecurityContextConstraints, securityvalidation.ValidateSecurityContextConstraintsUpdate)
	Validator.MustRegister(&securityapi.PodSecurityPolicySubjectReview{}, true, securityvalidation.ValidatePodSecurityPolicySubjectReview, nil)
	Validator.MustRegister(&securityapi.PodSecurityPolicySelfSubjectReview{}, true, securityvalidation.ValidatePodSecurityPolicySelfSubjectReview, nil)
	Validator.MustRegister(&securityapi.PodSecurityPolicyReview{}, true, securityvalidation.ValidatePodSecurityPolicyReview, nil)

	Validator.MustRegister(&quotaapi.ClusterResourceQuota{}, false, quotavalidation.ValidateClusterResourceQuota, quotavalidation.ValidateClusterResourceQuotaUpdate)
}

func routeOptionsAdapter(route *routeapi.Route) field.ErrorList {
	a := &authorizationclient.AuthorizationV1Client{}
	return routevalidation.ValidateRoute(context.Background(), route, a.SubjectAccessReviews(), &v1.CoreV1Client{}, routecommon.RouteValidationOptions{})
}

func routeUpdateOptionsAdapter(route *routeapi.Route, oldRoute *routeapi.Route) field.ErrorList {
	a := &authorizationclient.AuthorizationV1Client{}
	return routevalidation.ValidateRouteUpdate(context.Background(), route, oldRoute, a.SubjectAccessReviews(), &v1.CoreV1Client{}, routecommon.RouteValidationOptions{})
}
