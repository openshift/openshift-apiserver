package apiserver

import (
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"

	quotaapiv1 "github.com/openshift/api/quota/v1"
	quotainformer "github.com/openshift/client-go/quota/informers/externalversions"
	"github.com/openshift/library-go/pkg/quota/clusterquotamapping"
	appliedclusterresourcequotaregistry "github.com/openshift/openshift-apiserver/pkg/quota/apiserver/registry/appliedclusterresourcequota"
	clusterresourcequotaetcd "github.com/openshift/openshift-apiserver/pkg/quota/apiserver/registry/clusterresourcequota/etcd"
)

type ExtraConfig struct {
	ClusterQuotaMappingController *clusterquotamapping.ClusterQuotaMappingController
	QuotaInformers                quotainformer.SharedInformerFactory

	// TODO these should all become local eventually
	Scheme *runtime.Scheme
	Codecs serializer.CodecFactory

	makeV1Storage sync.Once
	v1Storage     map[string]rest.Storage
	v1StorageErr  error
}

type QuotaAPIServerConfig struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

type QuotaAPIServer struct {
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
func (c *QuotaAPIServerConfig) Complete() completedConfig {
	cfg := completedConfig{
		c.GenericConfig.Complete(),
		&c.ExtraConfig,
	}

	return cfg
}

// New returns a new instance of QuotaAPIServer from the given config.
func (c completedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*QuotaAPIServer, error) {
	genericServer, err := c.GenericConfig.New("quota.openshift.io-apiserver", delegationTarget)
	if err != nil {
		return nil, err
	}

	s := &QuotaAPIServer{
		GenericAPIServer: genericServer,
	}

	v1Storage, err := c.V1RESTStorage()
	if err != nil {
		return nil, err
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(quotaapiv1.GroupName, c.ExtraConfig.Scheme, metav1.ParameterCodec, c.ExtraConfig.Codecs)
	apiGroupInfo.VersionedResourcesStorageMap[quotaapiv1.SchemeGroupVersion.Version] = v1Storage
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
	clusterResourceQuotaStorage, clusterResourceQuotaStatusStorage, err := clusterresourcequotaetcd.NewREST()
	if err != nil {
		return nil, fmt.Errorf("error building REST storage: %v", err)
	}

	v1Storage := map[string]rest.Storage{}
	v1Storage["clusterresourcequotas"] = clusterResourceQuotaStorage
	v1Storage["clusterresourcequotas/status"] = clusterResourceQuotaStatusStorage
	v1Storage["appliedclusterresourcequotas"] = appliedclusterresourcequotaregistry.NewREST(
		c.ExtraConfig.ClusterQuotaMappingController.GetClusterQuotaMapper(),
		c.ExtraConfig.QuotaInformers.Quota().V1().ClusterResourceQuotas().Lister(),
	)
	return v1Storage, nil
}
