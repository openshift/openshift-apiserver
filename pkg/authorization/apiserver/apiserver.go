package apiserver

import (
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	kubeinformers "k8s.io/client-go/informers"
	restclient "k8s.io/client-go/rest"

	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	rbacregistryvalidation "k8s.io/kubernetes/pkg/registry/rbac/validation"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"

	authorizationapiv1 "github.com/openshift/api/authorization/v1"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/clusterrole"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/clusterrolebinding"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/localresourceaccessreview"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/localsubjectaccessreview"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/resourceaccessreview"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/role"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/rolebinding"
	rolebindingrestrictionetcd "github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/rolebindingrestriction/etcd"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/selfsubjectrulesreview"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/subjectaccessreview"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/subjectrulesreview"
)

type ExtraConfig struct {
	KubeAPIServerClientConfig *restclient.Config
	KubeInformers             kubeinformers.SharedInformerFactory
	RuleResolver              rbacregistryvalidation.AuthorizationRuleResolver
	SubjectLocator            rbac.SubjectLocator

	// TODO these should all become local eventually
	Scheme *runtime.Scheme
	Codecs serializer.CodecFactory

	makeV1Storage sync.Once
	v1Storage     map[string]rest.Storage
	v1StorageErr  error
}

type AuthorizationAPIServerConfig struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

type AuthorizationAPIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

type CompletedConfig struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (c *AuthorizationAPIServerConfig) Complete() completedConfig {
	cfg := completedConfig{
		c.GenericConfig.Complete(),
		&c.ExtraConfig,
	}

	return cfg
}

// New returns a new instance of AuthorizationAPIServer from the given config.
func (c completedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*AuthorizationAPIServer, error) {
	genericServer, err := c.GenericConfig.New("authorization.openshift.io-apiserver", delegationTarget)
	if err != nil {
		return nil, err
	}

	s := &AuthorizationAPIServer{
		GenericAPIServer: genericServer,
	}

	v1Storage, err := c.V1RESTStorage()
	if err != nil {
		return nil, err
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(authorizationapiv1.GroupName, c.ExtraConfig.Scheme, metav1.ParameterCodec, c.ExtraConfig.Codecs)
	apiGroupInfo.VersionedResourcesStorageMap[authorizationapiv1.SchemeGroupVersion.Version] = v1Storage
	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	return s, nil
}

func (c *completedConfig) V1RESTStorage() (map[string]rest.Storage, error) {
	c.ExtraConfig.makeV1Storage.Do(func() {
		c.ExtraConfig.v1Storage, c.ExtraConfig.v1StorageErr = c.newV1RESTStorage()
	})

	return c.ExtraConfig.v1Storage, c.ExtraConfig.v1StorageErr
}

func (c *completedConfig) newV1RESTStorage() (map[string]rest.Storage, error) {
	rbacClient, err := rbacv1client.NewForConfig(c.ExtraConfig.KubeAPIServerClientConfig)
	if err != nil {
		return nil, err
	}

	selfSubjectRulesReviewStorage := selfsubjectrulesreview.NewREST(c.ExtraConfig.RuleResolver, c.ExtraConfig.KubeInformers.Rbac().V1().ClusterRoles().Lister())
	subjectRulesReviewStorage := subjectrulesreview.NewREST(c.ExtraConfig.RuleResolver, c.ExtraConfig.KubeInformers.Rbac().V1().ClusterRoles().Lister())
	subjectAccessReviewStorage := subjectaccessreview.NewREST(c.GenericConfig.Authorization.Authorizer)
	subjectAccessReviewRegistry := subjectaccessreview.NewRegistry(subjectAccessReviewStorage)
	localSubjectAccessReviewStorage := localsubjectaccessreview.NewREST(subjectAccessReviewRegistry)
	resourceAccessReviewStorage := resourceaccessreview.NewREST(c.GenericConfig.Authorization.Authorizer, c.ExtraConfig.SubjectLocator)
	resourceAccessReviewRegistry := resourceaccessreview.NewRegistry(resourceAccessReviewStorage)
	localResourceAccessReviewStorage := localresourceaccessreview.NewREST(resourceAccessReviewRegistry)
	roleBindingRestrictionStorage, err := rolebindingrestrictionetcd.NewREST()
	if err != nil {
		return nil, fmt.Errorf("error building REST storage: %v", err)
	}

	v1Storage := map[string]rest.Storage{}
	v1Storage["resourceaccessreviews"] = resourceAccessReviewStorage
	v1Storage["subjectaccessreviews"] = subjectAccessReviewStorage
	v1Storage["localsubjectaccessreviews"] = localSubjectAccessReviewStorage
	v1Storage["localresourceaccessreviews"] = localResourceAccessReviewStorage
	v1Storage["selfsubjectrulesreviews"] = selfSubjectRulesReviewStorage
	v1Storage["subjectrulesreviews"] = subjectRulesReviewStorage
	v1Storage["roles"] = role.NewREST(rbacClient.RESTClient())
	v1Storage["rolebindings"] = rolebinding.NewREST(rbacClient.RESTClient())
	v1Storage["clusterroles"] = clusterrole.NewREST(rbacClient.RESTClient())
	v1Storage["clusterrolebindings"] = clusterrolebinding.NewREST(rbacClient.RESTClient())
	v1Storage["rolebindingrestrictions"] = roleBindingRestrictionStorage
	return v1Storage, nil
}
