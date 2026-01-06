package imagestreamimport

import (
	"context"
	"errors"
	"net/http"
	"net/netip"

	"github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation"
)

// hostContactCheck is a function that checks whether it's safe to contact a
// given host based on blocked and allowed IP prefix lists.
type hostContactCheck func(context.Context, string, []netip.Prefix, []netip.Prefix) error

// RestrictedTransport is an http.RoundTripper.
var _ http.RoundTripper = &RestrictedTransport{}

// RestrictedTransport restricts requests to certain IP ranges. It is meant to
// avoid doing requests to addresses the registry should not be allowed to
// reach.
type RestrictedTransport struct {
	wrapped           http.RoundTripper
	blocked           []netip.Prefix
	allowed           []netip.Prefix
	shouldContactHost hostContactCheck
}

// RoundTrip implements the http.RoundTripper interface. If the IP is blocked
// an error is returned, if not then it delegates to the wrapped RoundTripper.
func (r *RestrictedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.wrapped == nil {
		return nil, errors.New("no wrapped round tripper")
	}

	if r.shouldContactHost == nil {
		return nil, errors.New("nil validation function")
	}

	if req == nil || req.URL == nil {
		return nil, errors.New("nil request or url")
	}

	host := req.URL.Hostname()
	if err := r.shouldContactHost(req.Context(), host, r.blocked, r.allowed); err != nil {
		return nil, err
	}

	return r.wrapped.RoundTrip(req)
}

// NewRestrictedTransport creates a new RestrictedTransport that wraps the
// given http.RoundTripper and blocks requests to the given list of CIDR ranges.
// IPs in the allowed list bypass all checks; IPs not in allowed are checked
// against loopback, link-local, and the blocked list.
func NewRestrictedTransport(wrapped http.RoundTripper, blocked, allowed []netip.Prefix) *RestrictedTransport {
	return &RestrictedTransport{
		wrapped:           wrapped,
		blocked:           blocked,
		allowed:           allowed,
		shouldContactHost: validation.ShouldContactHost,
	}
}
