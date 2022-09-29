package apiserver

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	appsv1 "github.com/openshift/api/apps/v1"
	appsclient "github.com/openshift/client-go/apps/clientset/versioned"
	imagev1client "github.com/openshift/client-go/image/clientset/versioned"
	deployconfigetcd "github.com/openshift/openshift-apiserver/pkg/apps/apiserver/registry/deployconfig/etcd"
	deploylogregistry "github.com/openshift/openshift-apiserver/pkg/apps/apiserver/registry/deploylog"
	deployconfiginstantiate "github.com/openshift/openshift-apiserver/pkg/apps/apiserver/registry/instantiate"
	deployrollback "github.com/openshift/openshift-apiserver/pkg/apps/apiserver/registry/rollback"
)

type ExtraConfig struct {
	KubeAPIServerClientConfig *restclient.Config

	// TODO these should all become local eventually
	Scheme *runtime.Scheme
	Codecs serializer.CodecFactory

	makeV1Storage sync.Once
	v1Storage     map[string]rest.Storage
	v1StorageErr  error
}

type AppsServerConfig struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

type AppsServer struct {
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
func (c *AppsServerConfig) Complete() completedConfig {
	cfg := completedConfig{
		c.GenericConfig.Complete(),
		&c.ExtraConfig,
	}

	return cfg
}

// New returns a new instance of AppsServer from the given config.
func (c completedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*AppsServer, error) {
	genericServer, err := c.GenericConfig.New("apps.openshift.io-apiserver", delegationTarget)
	if err != nil {
		return nil, err
	}

	s := &AppsServer{
		GenericAPIServer: genericServer,
	}

	v1Storage, err := c.V1RESTStorage()
	if err != nil {
		return nil, err
	}

	parameterCodec := runtime.NewParameterCodec(c.ExtraConfig.Scheme)
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(appsv1.GroupName, c.ExtraConfig.Scheme, parameterCodec, c.ExtraConfig.Codecs)
	apiGroupInfo.VersionedResourcesStorageMap[appsv1.SchemeGroupVersion.Version] = v1Storage

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
	openshiftAppsClient, err := appsclient.NewForConfig(c.GenericConfig.LoopbackClientConfig)
	if err != nil {
		return nil, err
	}
	// This client is using the core api server client config, since the apps server doesn't host images
	openshiftImageClient, err := imagev1client.NewForConfig(c.ExtraConfig.KubeAPIServerClientConfig)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(c.ExtraConfig.KubeAPIServerClientConfig)
	if err != nil {
		return nil, err
	}

	deployConfigStorage, deployConfigStatusStorage, deployConfigScaleStorage, err := deployconfigetcd.NewREST(c.GenericConfig.RESTOptionsGetter)
	if err != nil {
		return nil, err
	}
	dcInstantiateStorage := deployconfiginstantiate.NewREST(
		*deployConfigStorage.Store,
		openshiftImageClient,
		kubeClient,
		c.GenericConfig.AdmissionControl,
	)
	deployConfigRollbackStorage := deployrollback.NewREST(openshiftAppsClient, kubeClient)

	v1Storage := map[string]rest.Storage{}
	v1Storage["deploymentconfigs"] = deployConfigStorage
	v1Storage["deploymentconfigs/scale"] = deployConfigScaleStorage
	v1Storage["deploymentconfigs/status"] = deployConfigStatusStorage
	v1Storage["deploymentconfigs/rollback"] = deployConfigRollbackStorage
	v1Storage["deploymentconfigs/log"] = deploylogregistry.NewREST(openshiftAppsClient.AppsV1(), kubeClient)
	v1Storage["deploymentconfigs/instantiate"] = dcInstantiateStorage
	return v1Storage, nil
}
