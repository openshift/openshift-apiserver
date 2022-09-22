package routehostassignment

import (
	"fmt"
	"strings"

	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/klog/v2"

	routev1 "github.com/openshift/api/route/v1"
)

const defaultDNSSuffix = "router.default.svc.cluster.local"

type HostDefaulter struct {
	dnsSuffix string
}

func NewHostDefaulter(suffix string) (*HostDefaulter, error) {
	if len(suffix) == 0 {
		suffix = defaultDNSSuffix
	}

	klog.V(4).Infof("Route host defaulter initialized with suffix=%s", suffix)

	// Check that the DNS suffix is valid.
	if len(kvalidation.IsDNS1123Subdomain(suffix)) != 0 {
		return nil, fmt.Errorf("invalid DNS suffix: %s", suffix)
	}

	return &HostDefaulter{dnsSuffix: suffix}, nil
}

func (p *HostDefaulter) DefaultHost(route *routev1.Route) string {
	if len(route.Name) == 0 || len(route.Namespace) == 0 {
		return ""
	}
	return fmt.Sprintf("%s-%s.%s", strings.Replace(route.Name, ".", "-", -1), route.Namespace, p.dnsSuffix)
}

type WantsRouteHostDefaulter interface {
	SetRouteHostDefaulter(*HostDefaulter)
	admission.InitializationValidator
}

var _ = admission.PluginInitializer(&HostDefaulter{})

func (hd *HostDefaulter) Initialize(plugin admission.Interface) {
	if wants, ok := plugin.(WantsRouteHostDefaulter); ok {
		wants.SetRouteHostDefaulter(hd)
	}
}
