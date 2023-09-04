package registryhostname

import (
	"context"
	"os"
)

// RegistryHostnameRetriever represents an interface for retrieving the hostname
// of internal and external registry.
type RegistryHostnameRetriever interface {
	InternalRegistryHostname(context.Context) (string, bool)
	ExternalRegistryHostname() (string, bool)
}

// DefaultRegistryHostnameRetriever is a default implementation of
// RegistryHostnameRetriever.
func DefaultRegistryHostnameRetriever(external, internal string) RegistryHostnameRetriever {
	return &defaultRegistryHostnameRetriever{
		externalHostname: external,
		internalHostname: internal,
	}
}

// env returns an environment variable, or the defaultValue if it is not set.
func env(key string, defaultValue string) string {
	val := os.Getenv(key)
	if len(val) == 0 {
		return defaultValue
	}
	return val
}

type defaultRegistryHostnameRetriever struct {
	internalHostname string
	externalHostname string
}

// InternalRegistryHostname returns the internal registry hostname as seen in
// InternalRegistryHostname
func (r *defaultRegistryHostnameRetriever) InternalRegistryHostname(ctx context.Context) (string, bool) {
	if len(r.internalHostname) > 0 {
		return r.internalHostname, true
	}
	return "", false
}

// ExternalRegistryHostnameFn returns a function that can be used to retrieve an
// external/public hostname of Docker Registry. External location can be
// configured in master config using 'ExternalRegistryHostname' property.
func (r *defaultRegistryHostnameRetriever) ExternalRegistryHostname() (string, bool) {
	return r.externalHostname, len(r.externalHostname) > 0
}
