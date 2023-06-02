package route

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/apis/core"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Route encapsulates the inputs needed to connect an alias to endpoints.
type Route struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// Spec is the desired behavior of the route
	Spec RouteSpec
	// Status describes the current observed state of the route
	Status RouteStatus
}

// RouteSpec describes the desired behavior of a route.
type RouteSpec struct {
	// Host is an alias/DNS that points to the service. Optional
	// Must follow DNS952 subdomain conventions.
	Host string
	// Subdomain is a DNS subdomain that is requested within the ingress controller's
	// domain (as a subdomain). If host is set this field is ignored. An ingress
	// controller may choose to ignore this suggested name, in which case the controller
	// will report the assigned name in the status.ingress array or refuse to admit the
	// route. If this value is set and the server does not support this field host will
	// be populated automatically. Otherwise host is left empty. The field may have
	// multiple parts separated by a dot, but not all ingress controllers may honor
	// the request. This field may not be changed after creation except by a user with
	// the update routes/custom-host permission.
	//
	// Example: subdomain `frontend` automatically receives the router subdomain
	// `apps.mycluster.com` to have a full hostname `frontend.apps.mycluster.com`.
	Subdomain string

	// Path that the router watches for, to route traffic for to the service. Optional
	Path string

	// Objects that the route points to. Only the Service kind is allowed, and it will
	// be defaulted to Service.
	To RouteTargetReference

	// Alternate objects that the route may want to point to. Use the 'weight' field to
	// determine which ones of the several get more emphasis
	AlternateBackends []RouteTargetReference

	// If specified, the port to be used by the router. Most routers will use all
	// endpoints exposed by the service by default - set this value to instruct routers
	// which port to use.
	Port *RoutePort

	//TLS provides the ability to configure certificates and termination for the route
	TLS *TLSConfig

	// Wildcard policy if any for the route.
	// Currently only 'Subdomain' or 'None' is allowed.
	WildcardPolicy WildcardPolicyType

	// httpHeaders defines policy for HTTP headers.
	//
	// If this field is empty, the default values are used.
	//
	// +optional
	HTTPHeaders RouteHTTPHeaders `json:"httpHeaders,omitempty" protobuf:"bytes,9,opt,name=httpHeaders"`
}

// RouteHTTPHeaders defines policy for HTTP headers.
type RouteHTTPHeaders struct {
	// actions specifies actions to take on headers.
	// Note that this option only applies to cleartext HTTP connections
	// and to secure HTTP connections for which the ingress controller
	// terminates encryption (that is, edge-terminated or reencrypt
	// connections).  Headers cannot be set or deleted for TLS passthrough
	// connections.
	// Setting HSTS header is supported via another mechanism in Ingress.Spec.RequiredHSTSPolicies.
	// Setting httpCaptureHeaders for the headers set via this API will not work.
	// If spec.clientTLS, spec.httpHeaders.forwardedHeaderPolicy, spec.httpHeaders.uniqueId, spec.httpHeaders.headerNameCaseAdjustments is set
	// and if you set the same header via this API then that header will get replaced with the value specified via this API.
	// If cache-control is set using this API then the existing value of cache-control will get replaced.
	// The header set using route will over-ride the header set in ingress controller if the headers defined both were same.
	// +optional
	Actions RouteHTTPHeaderActions `json:"actions,omitempty" protobuf:"bytes,1,opt,name=actions"`
}

// RouteHTTPHeaderActions defines configuration for actions on HTTP request and response headers.
type RouteHTTPHeaderActions struct {
	// response is a list of HTTP response headers to set or delete.
	// The actions either Set or Delete will be performed on the headers in sequence as defined in the list of response headers.
	// A maximum of 64 response header actions may be configured.
	// You can use this field to specify HTTP response headers that should be set or deleted
	// when forwarding responses from your application to the client.
	// Sample fetchers allowed are "req.hdr" and "ssl_c_der".
	// Converters allowed are "lower" and "base64".
	// Examples of header value - "%[res.hdr(X-target),lower]", "%[res.hdr(X-client),base64]", "{+Q}[ssl_c_der,base64]", "{+Q}[ssl_c_der,lower]"
	// Note: This field cannot be used if your route uses TLS passthrough.
	// + ---
	// + Note: Any change to regex mentioned below must be reflected in the crd validation of route in https://github.com/openshift/library-go/blob/master/pkg/route/validation/validation.go) and vice-versa.
	// +listType=map
	// +listMapKey=name
	// +optional
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:XValidation:rule=`self.all(key, key.action.type == "Delete" || (has(key.action.set) && key.action.set.value.matches('^(?:%(?:%|(?:\\{[-+]?[QXE](?:,[-+]?[QXE])*\\})?\\[(?:res\\.hdr\\([0-9A-Za-z-]+\\)|ssl_c_der)(?:,(?:lower|base64))*\\])|[^%[:cntrl:]])+$')))`,message="Either the header value provided is not in correct format or the sample fetcher/converter specified is not allowed. The dynamic header value will be interpreted as an HAProxy format string as defined in https://cbonte.github.io/haproxy-dconv/2.4/configuration.html#8.2.4 and may use HAProxy's %[] syntax and otherwise must be a valid HTTP header value as defined in https://datatracker.ietf.org/doc/html/rfc7230#section-3.2 Sample fetchers allowed are res.hdr, ssl_c_der. Converters allowed are lower, base64."
	Response []RouteHTTPHeader `json:"response,omitempty" protobuf:"bytes,1,rep,name=response"`
	// request is a list of HTTP request headers to set or delete.
	// The actions either Set or Delete will be performed on the headers in sequence as defined in the list of request headers.
	// A maximum of 64 request header actions may be configured.
	// You can use this field to specify HTTP request headers that should be set or deleted
	// when forwarding connections from the client to your application.
	// Sample fetchers allowed are "req.hdr" and "ssl_c_der".
	// Converters allowed are "lower" and "base64".
	// Examples of header value - "%[req.hdr(host),lower]", "%[req.hdr(host),base64]", "{+Q}[ssl_c_der,base64]", "{+Q}[ssl_c_der,lower]"
	// Note: This field cannot be used if your route uses TLS passthrough.
	// + ---
	// + Note: Any change to regex mentioned below must be reflected in the crd validation of route in https://github.com/openshift/library-go/blob/master/pkg/route/validation/validation.go) and vice-versa.
	// +listType=map
	// +listMapKey=name
	// +optional
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:XValidation:rule=`self.all(key, key.action.type == "Delete" || (has(key.action.set) && key.action.set.value.matches('^(?:%(?:%|(?:\\{[-+]?[QXE](?:,[-+]?[QXE])*\\})?\\[(?:req\\.hdr\\([0-9A-Za-z-]+\\)|ssl_c_der)(?:,(?:lower|base64))*\\])|[^%[:cntrl:]])+$')))`,message="Either the header value provided is not in correct format or the sample fetcher/converter specified is not allowed. The dynamic header value will be interpreted as an HAProxy format string as defined in https://cbonte.github.io/haproxy-dconv/2.4/configuration.html#8.2.4 and may use HAProxy's %[] syntax and otherwise must be a valid HTTP header value as defined in https://datatracker.ietf.org/doc/html/rfc7230#section-3.2 Sample fetchers allowed are req.hdr, ssl_c_der. Converters allowed are lower, base64."
	Request []RouteHTTPHeader `json:"request,omitempty" protobuf:"bytes,2,rep,name=request"`
}

// RouteHTTPHeader specifies configuration for setting or deleting an HTTP header.
type RouteHTTPHeader struct {
	// name specifies a header name to be set or deleted.  Its value must be a valid HTTP header.
	// name as defined in RFC 2616 section 4.2.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:Pattern="^[-!#$%&'*+.0-9A-Z^_`a-z|~]+$"
	// +kubebuilder:validation:XValidation:rule="self.lowerAscii() != 'strict-transport-security'",message="strict-transport-security may not be set/delete via header actions"
	// +kubebuilder:validation:XValidation:rule="self.lowerAscii() != 'proxy'",message="proxy may not be set/delete via header actions"
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`

	// action specifies actions to perform on headers, such as setting or deleting headers.
	// +kubebuilder:validation:Required
	Action RouteHTTPHeaderActionUnion `json:"action" protobuf:"bytes,2,opt,name=action"`
}

// RouteHTTPHeaderActionUnion specifies an action to take on an HTTP header.
// +union
type RouteHTTPHeaderActionUnion struct {
	// type defines the type of the action to be applied on the header.
	// Possible values are Set and Delete.
	// Set allows you to set http request and response headers.
	// Delete allows you to delete http request and response headers
	// +unionDiscriminator
	// +kubebuilder:validation:Enum:=Set;Delete
	// +kubebuilder:validation:Required
	Type RouteHTTPHeaderActionType `json:"type" protobuf:"bytes,11,opt,name=type,casttype=RouteHTTPHeaderActionType"`

	// set defines the HTTP header that should be set: added if doesn't exist or replaced if does.
	// If this field is empty, no header is added or replaced.
	// +optional
	// +unionMember
	Set *RouteSetHTTPHeader `json:"set,omitempty" protobuf:"bytes,9,opt,name=set"`
}

// RouteSetHTTPHeader specifies what value needs to be set on an HTTP header.
type RouteSetHTTPHeader struct {
	// value specifies a header value.
	// Dynamic values can be added. The value will be interpreted as an HAProxy format string as defined in
	// https://cbonte.github.io/haproxy-dconv/2.4/configuration.html#8.2.4 and may use HAProxy's %[] syntax and
	// otherwise must be a valid HTTP header value as defined in https://datatracker.ietf.org/doc/html/rfc7230#section-3.2
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	Value string `json:"value" protobuf:"bytes,2,opt,name=value"`
}

// RouteHTTPHeaderActionType defines actions that can be performed on HTTP headers.
type RouteHTTPHeaderActionType string

const (
	// Set specifies that an HTTP header should be set.
	Set RouteHTTPHeaderActionType = "Set"
	// Delete specifies that an HTTP header should be deleted.
	Delete RouteHTTPHeaderActionType = "Delete"
)

// RouteTargetReference specifies the target that resolve into endpoints. Only the 'Service'
// kind is allowed. Use 'weight' field to emphasize one over others.
type RouteTargetReference struct {
	Kind   string
	Name   string
	Weight *int32
}

// RoutePort defines a port mapping from a router to an endpoint in the service endpoints.
type RoutePort struct {
	// The target port on pods selected by the service this route points to.
	// If this is a string, it will be looked up as a named port in the target
	// endpoints port list. Required
	TargetPort intstr.IntOrString
}

// RouteStatus provides relevant info about the status of a route, including which routers
// acknowledge it.
type RouteStatus struct {
	// Ingress describes the places where the route may be exposed. The list of
	// ingress points may contain duplicate Host or RouterName values. Routes
	// are considered live once they are `Ready`
	Ingress []RouteIngress
}

// RouteIngress holds information about the places where a route is exposed
type RouteIngress struct {
	// Host is the host string under which the route is exposed; this value is required
	Host string
	// Name is a name chosen by the router to identify itself; this value is required
	RouterName string
	// Conditions is the state of the route, may be empty.
	Conditions []RouteIngressCondition
	// Wildcard policy is the wildcard policy that was allowed where this route is exposed.
	WildcardPolicy WildcardPolicyType
	// CanonicalHostname is an external host name for the router; this value is optional
	RouterCanonicalHostname string
}

// RouteIngressConditionType is a valid value for RouteCondition
type RouteIngressConditionType string

// These are valid conditions of pod.
const (
	// RouteAdmitted means the route is able to service requests for the provided Host
	RouteAdmitted RouteIngressConditionType = "Admitted"
	// RouteExtendedValidationFailed means the route configuration failed an extended validation check.
	RouteExtendedValidationFailed RouteIngressConditionType = "ExtendedValidationFailed"
	// TODO: add other route condition types
)

// RouteIngressCondition contains details for the current condition of this pod.
// TODO: add LastTransitionTime, Reason, Message to match NodeCondition api.
type RouteIngressCondition struct {
	// Type is the type of the condition.
	// Currently only Ready.
	Type RouteIngressConditionType
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status core.ConditionStatus
	// (brief) reason for the condition's last transition, and is usually a machine and human
	// readable constant
	Reason string
	// Human readable message indicating details about last transition.
	Message string
	// RFC 3339 date and time at which the object was acknowledged by the router.
	// This may be before the router exposes the route
	LastTransitionTime *metav1.Time
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RouteList is a collection of Routes.
type RouteList struct {
	metav1.TypeMeta
	metav1.ListMeta

	// Items is a list of routes
	Items []Route
}

// RouterShard has information of a routing shard and is used to
// generate host names and routing table entries when a routing shard is
// allocated for a specific route.
type RouterShard struct {
	// ShardName uniquely identifies a router shard in the "set" of
	// routers used for routing traffic to the services.
	ShardName string

	// DNSSuffix for the shard ala: shard-1.v3.openshift.com
	DNSSuffix string
}

// TLSConfig defines config used to secure a route and provide termination
type TLSConfig struct {
	// Termination indicates termination type.
	Termination TLSTerminationType

	// Certificate provides certificate contents
	Certificate string

	// Key provides key file contents
	Key string

	// CACertificate provides the cert authority certificate contents
	CACertificate string

	// DestinationCACertificate provides the contents of the ca certificate of the final destination.  When using reencrypt
	// termination this file should be provided in order to have routers use it for health checks on the secure connection
	DestinationCACertificate string

	// InsecureEdgeTerminationPolicy indicates the desired behavior for
	// insecure connections to an edge-terminated route:
	//   disable, allow or redirect
	InsecureEdgeTerminationPolicy InsecureEdgeTerminationPolicyType
}

// TLSTerminationType dictates where the secure communication will stop
// TODO: Reconsider this type in v2
type TLSTerminationType string

// InsecureEdgeTerminationPolicyType dictates the behavior of insecure
// connections to an edge-terminated route.
type InsecureEdgeTerminationPolicyType string

const (
	// TLSTerminationEdge terminate encryption at the edge router.
	TLSTerminationEdge TLSTerminationType = "edge"
	// TLSTerminationPassthrough terminate encryption at the destination, the destination is responsible for decrypting traffic
	TLSTerminationPassthrough TLSTerminationType = "passthrough"
	// TLSTerminationReencrypt terminate encryption at the edge router and re-encrypt it with a new certificate supplied by the destination
	TLSTerminationReencrypt TLSTerminationType = "reencrypt"

	// InsecureEdgeTerminationPolicyNone disables insecure connections for an edge-terminated route.
	InsecureEdgeTerminationPolicyNone InsecureEdgeTerminationPolicyType = "None"
	// InsecureEdgeTerminationPolicyAllow allows insecure connections for an edge-terminated route.
	InsecureEdgeTerminationPolicyAllow InsecureEdgeTerminationPolicyType = "Allow"
	// InsecureEdgeTerminationPolicyRedirect redirects insecure connections for an edge-terminated route.
	// As an example, for routers that support HTTP and HTTPS, the
	// insecure HTTP connections will be redirected to use HTTPS.
	InsecureEdgeTerminationPolicyRedirect InsecureEdgeTerminationPolicyType = "Redirect"
)

// WildcardPolicyType indicates the type of wildcard support needed by routes.
type WildcardPolicyType string

const (
	// WildcardPolicyNone indicates no wildcard support is needed.
	WildcardPolicyNone WildcardPolicyType = "None"

	// WildcardPolicySubdomain indicates the host needs wildcard support for the subdomain.
	// Example: With host = "www.acme.test", indicates that the router
	//          should support requests for *.acme.test
	//          Note that this will not match acme.test only *.acme.test
	WildcardPolicySubdomain WildcardPolicyType = "Subdomain"
)
