package openshiftapiserver

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	openshiftcontrolplanev1 "github.com/openshift/api/openshiftcontrolplane/v1"
	"github.com/openshift/apiserver-library-go/pkg/configflags"
	"github.com/openshift/library-go/pkg/apiserver/admission/admissiontimeout"
	"github.com/openshift/library-go/pkg/apiserver/apiserverconfig"
	"github.com/openshift/library-go/pkg/config/helpers"
	"github.com/openshift/library-go/pkg/config/serving"
	"github.com/openshift/library-go/pkg/features"
	routehostassignment "github.com/openshift/library-go/pkg/route/hostassignment"
	"github.com/openshift/openshift-apiserver/pkg/cmd/openshift-apiserver/openshiftadmission"
	"github.com/openshift/openshift-apiserver/pkg/cmd/openshift-apiserver/openshiftapiserver/configprocessing"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/registryhostname"
	"github.com/openshift/openshift-apiserver/pkg/version"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/admission"
	admissionmetrics "k8s.io/apiserver/pkg/admission/metrics"
	"k8s.io/apiserver/pkg/endpoints/discovery/aggregated"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericapiserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/feature"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
)

func NewOpenshiftAPIConfig(config *openshiftcontrolplanev1.OpenShiftAPIServerConfig, authenticationOptions *genericapiserveroptions.DelegatingAuthenticationOptions, authorizationOptions *genericapiserveroptions.DelegatingAuthorizationOptions, internalOAuthDisabled bool) (*OpenshiftAPIConfig, error) {
	kubeClientConfig, err := helpers.GetKubeClientConfig(config.KubeClientConfig)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(kubeClientConfig)
	if err != nil {
		return nil, err
	}
	kubeInformers := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)

	openshiftVersion := version.Get()

	genericConfig := genericapiserver.NewRecommendedConfig(legacyscheme.Codecs)
	// Current default values
	// Serializer:                   codecs,
	// ReadWritePort:                443,
	// BuildHandlerChainFunc:        DefaultBuildHandlerChain,
	// HandlerChainWaitGroup:        new(utilwaitgroup.SafeWaitGroup),
	// LegacyAPIGroupPrefixes:       sets.NewString(DefaultLegacyAPIPrefix),
	// DisabledPostStartHooks:       sets.NewString(),
	// HealthzChecks:                []healthz.HealthzChecker{healthz.PingHealthz, healthz.LogHealthz},
	// EnableIndex:                  true,
	// EnableDiscovery:              true,
	// EnableProfiling:              true,
	// EnableMetrics:                true,
	// MaxRequestsInFlight:          400,
	// MaxMutatingRequestsInFlight:  200,
	// RequestTimeout:               time.Duration(60) * time.Second,
	// MinRequestTimeout:            1800,
	// EnableAPIResponseCompression: utilfeature.DefaultFeatureGate.Enabled(features.APIResponseCompression),
	// LongRunningFunc: genericfilters.BasicLongRunningRequestCheck(sets.NewString("watch"), sets.NewString()),

	// TODO this is actually specific to the kubeapiserver
	// RuleResolver authorizer.RuleResolver
	genericConfig.SharedInformerFactory = kubeInformers
	genericConfig.ClientConfig = kubeClientConfig

	// these are set via options
	// SecureServing *SecureServingInfo
	// Authentication AuthenticationInfo
	// Authorization AuthorizationInfo
	// LoopbackClientConfig *restclient.Config
	// this is set after the options are overlayed to get the authorizer we need.
	// AdmissionControl      admission.Interface
	// ReadWritePort int
	// PublicAddress net.IP

	// these are defaulted sanely during complete
	// DiscoveryAddresses discovery.Addresses

	genericConfig.CorsAllowedOriginList = config.CORSAllowedOrigins
	genericConfig.Version = &openshiftVersion
	genericConfig.ExternalAddress = "apiserver.openshift-apiserver.svc"
	genericConfig.BuildHandlerChainFunc = OpenshiftHandlerChain
	genericConfig.RequestInfoResolver = apiserverconfig.OpenshiftRequestInfoResolver()
	genericConfig.OpenAPIConfig = configprocessing.DefaultOpenAPIConfig()
	genericConfig.OpenAPIV3Config = configprocessing.DefaultOpenAPIV3Config()
	// previously overwritten.  I don't know why
	genericConfig.RequestTimeout = time.Duration(60) * time.Second
	genericConfig.MinRequestTimeout = int(config.ServingInfo.RequestTimeoutSeconds)
	genericConfig.MaxRequestsInFlight = int(config.ServingInfo.MaxRequestsInFlight)
	genericConfig.MaxMutatingRequestsInFlight = int(config.ServingInfo.MaxRequestsInFlight / 2)
	genericConfig.LongRunningFunc = apiserverconfig.IsLongRunningRequest
	genericConfig.AggregatedDiscoveryGroupManager = aggregated.NewResourceManager("apis")
	// do not to install the default OpenAPI handler in the aggregated apiserver
	// as it will be handled by openapi aggregator (both v2 and v3)
	// non-root apiservers must set this value to false
	genericConfig.Config.SkipOpenAPIInstallation = true

	// set the global featuregates.`
	warnings, err := features.SetFeatureGates(config.APIServerArguments, feature.DefaultMutableFeatureGate)
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		klog.Warning(warning)
	}

	etcdOptions, err := ToEtcdOptions(config.APIServerArguments, config.StorageConfig)
	if err != nil {
		return nil, err
	}
	storageFactory := NewStorageFactory(etcdOptions)
	if err := etcdOptions.ApplyWithStorageFactoryTo(storageFactory, &genericConfig.Config); err != nil {
		return nil, err
	}

	// It is not worse than it was before. This code deserves refactoring.
	// Instead of having a dedicated configuration file, we should pass flags directly.
	// Until then it seems okay to have the following construct.
	if shutdownDelaySlice := config.APIServerArguments["shutdown-delay-duration"]; len(shutdownDelaySlice) == 1 {
		shutdownDelay, err := time.ParseDuration(shutdownDelaySlice[0])
		if err != nil {
			return nil, err
		}

		genericConfig.ShutdownDelayDuration = shutdownDelay
	}

	// I'm just hoping this works.  I don't think we use it.
	// MergedResourceConfig *serverstore.ResourceConfig

	servingOptions, err := serving.ToServingOptions(config.ServingInfo)
	if err != nil {
		return nil, err
	}
	if err := servingOptions.ApplyTo(&genericConfig.Config.SecureServing, &genericConfig.Config.LoopbackClientConfig); err != nil {
		return nil, err
	}
	if err := authenticationOptions.ApplyTo(&genericConfig.Authentication, genericConfig.SecureServing, genericConfig.OpenAPIConfig); err != nil {
		return nil, err
	}
	if err := authorizationOptions.ApplyTo(&genericConfig.Authorization); err != nil {
		return nil, err
	}

	informers, err := NewInformers(kubeInformers, kubeClientConfig, genericConfig.LoopbackClientConfig)
	if err != nil {
		return nil, err
	}

	auditFlags := configflags.AuditFlags(&config.AuditConfig, configflags.ArgsWithPrefix(config.APIServerArguments, "audit-"))
	auditOpt := genericapiserveroptions.NewAuditOptions()
	fs := pflag.NewFlagSet("audit", pflag.ContinueOnError)
	auditOpt.AddFlags(fs)
	if err := fs.Parse(configflags.ToFlagSlice(auditFlags)); err != nil {
		return nil, err
	}
	if errs := auditOpt.Validate(); len(errs) > 0 {
		return nil, errors.NewAggregate(errs)
	}
	if err := auditOpt.ApplyTo(
		&genericConfig.Config,
	); err != nil {
		return nil, err
	}

	projectCache, err := NewProjectCache(informers.kubernetesInformers.Core().V1().Namespaces(), kubeClientConfig, config.ProjectConfig.DefaultNodeSelector)
	if err != nil {
		return nil, err
	}
	clusterQuotaMappingController := NewClusterQuotaMappingController(informers.kubernetesInformers.Core().V1().Namespaces(), informers.quotaInformers.Quota().V1().ClusterResourceQuotas())
	discoveryClient := cacheddiscovery.NewMemCacheClient(kubeClient.Discovery())
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	admissionInitializer, err := openshiftadmission.NewPluginInitializer(config, genericConfig, dynamicClient, kubeClientConfig, informers, feature.DefaultFeatureGate, restMapper, clusterQuotaMappingController)
	if err != nil {
		return nil, err
	}

	admissionConfigFile, cleanup, err := openshiftadmission.ToAdmissionConfigFile(config.AdmissionConfig.PluginConfig)
	defer cleanup()
	if err != nil {
		return nil, err
	}
	admissionOptions := genericapiserveroptions.NewAdmissionOptions()
	admissionOptions.Decorators = admission.Decorators{
		admission.DecoratorFunc(admissionmetrics.WithControllerMetrics),
		admission.DecoratorFunc(admissiontimeout.AdmissionTimeout{Timeout: 13 * time.Second}.WithTimeout),
	}
	admissionOptions.DefaultOffPlugins = sets.String{}
	admissionOptions.RecommendedPluginOrder = openshiftadmission.OpenShiftAdmissionPlugins
	admissionOptions.Plugins = openshiftadmission.OriginAdmissionPlugins
	admissionOptions.EnablePlugins = config.AdmissionConfig.EnabledAdmissionPlugins
	if internalOAuthDisabled == true {
		config.AdmissionConfig.DisabledAdmissionPlugins = append(config.AdmissionConfig.DisabledAdmissionPlugins, "project.openshift.io/ProjectRequestLimit")
	}
	admissionOptions.DisablePlugins = config.AdmissionConfig.DisabledAdmissionPlugins
	admissionOptions.ConfigFile = admissionConfigFile
	// TODO:
	if err := admissionOptions.ApplyTo(&genericConfig.Config, kubeInformers, kubeClient, dynamicClient, nil, admissionInitializer); err != nil {
		return nil, err
	}

	var externalRegistryHostname string
	if len(config.ImagePolicyConfig.ExternalRegistryHostnames) > 0 {
		externalRegistryHostname = config.ImagePolicyConfig.ExternalRegistryHostnames[0]
	}
	registryHostnameRetriever := registryhostname.DefaultRegistryHostnameRetriever(externalRegistryHostname, config.ImagePolicyConfig.InternalRegistryHostname)

	var caData []byte
	if len(config.ImagePolicyConfig.AdditionalTrustedCA) != 0 {
		klog.V(2).Infof("Image import using additional CA path: %s", config.ImagePolicyConfig.AdditionalTrustedCA)
		var err error
		caData, err = ioutil.ReadFile(config.ImagePolicyConfig.AdditionalTrustedCA)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA bundle %s for image importing: %v", config.ImagePolicyConfig.AdditionalTrustedCA, err)
		}
	}

	subjectLocator := NewSubjectLocator(informers.GetKubernetesInformers().Rbac().V1())
	projectAuthorizationCache := NewProjectAuthorizationCache(
		subjectLocator,
		informers.GetKubernetesInformers().Core().V1().Namespaces(),
		informers.GetKubernetesInformers().Rbac().V1(),
	)

	routeAllocator, err := routehostassignment.NewSimpleAllocationPlugin(config.RoutingConfig.Subdomain)
	if err != nil {
		return nil, err
	}

	ruleResolver := NewRuleResolver(informers.kubernetesInformers.Rbac().V1())

	ret := &OpenshiftAPIConfig{
		GenericConfig: genericConfig,
		ExtraConfig: OpenshiftAPIExtraConfig{
			InformerStart:                      informers.Start,
			KubeAPIServerClientConfig:          kubeClientConfig,
			KubeInformers:                      kubeInformers, // TODO remove this and use the one from the genericconfig
			QuotaInformers:                     informers.quotaInformers,
			SecurityInformers:                  informers.securityInformers,
			OperatorInformers:                  informers.operatorInformers,
			ConfigInformers:                    informers.configInformers,
			RuleResolver:                       ruleResolver,
			SubjectLocator:                     subjectLocator,
			RegistryHostnameRetriever:          registryHostnameRetriever,
			AllowedRegistriesForImport:         config.ImagePolicyConfig.AllowedRegistriesForImport,
			MaxImagesBulkImportedPerRepository: config.ImagePolicyConfig.MaxImagesBulkImportedPerRepository,
			AdditionalTrustedCA:                caData,
			RouteAllocator:                     routeAllocator,
			AllowRouteExternalCertificates:     feature.DefaultFeatureGate.Enabled(featuregate.Feature(configv1.FeatureGateRouteExternalCertificate)),
			ProjectAuthorizationCache:          projectAuthorizationCache,
			ProjectCache:                       projectCache,
			ProjectRequestTemplate:             config.ProjectConfig.ProjectRequestTemplate,
			ProjectRequestMessage:              config.ProjectConfig.ProjectRequestMessage,
			ClusterQuotaMappingController:      clusterQuotaMappingController,
			RESTMapper:                         restMapper,
			APIServers:                         config.APIServers,
		},
	}

	return ret, ret.ExtraConfig.Validate()
}

func OpenshiftHandlerChain(apiHandler http.Handler, genericConfig *genericapiserver.Config) http.Handler {
	// this is the normal kube handler chain
	handler := genericapiserver.DefaultBuildHandlerChain(apiHandler, genericConfig)

	handler = apiserverconfig.WithCacheControl(handler, "no-store") // protected endpoints should not be cached

	return handler
}
