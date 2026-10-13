package fake

import (
	"context"

	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apis/image/validation/whitelist"
)

type RegistryWhitelister struct{}

func (rw *RegistryWhitelister) AdmitHostname(_ context.Context, host string, transport whitelist.WhitelistTransport) error {
	return nil
}
func (rw *RegistryWhitelister) AdmitPullSpec(_ context.Context, pullSpec string, transport whitelist.WhitelistTransport) error {
	return nil
}
func (rw *RegistryWhitelister) AdmitDockerImageReference(_ context.Context, ref imageapi.DockerImageReference, transport whitelist.WhitelistTransport) error {
	return nil
}
func (rw *RegistryWhitelister) WhitelistRegistry(hostPortGlob string, transport whitelist.WhitelistTransport) error {
	return nil
}
func (rw *RegistryWhitelister) WhitelistRepository(pullSpec string) error {
	return nil
}
func (rw *RegistryWhitelister) Copy() whitelist.RegistryWhitelister {
	return &RegistryWhitelister{}
}
