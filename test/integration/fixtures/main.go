package fixtures

import (
	"testing"

	etcdtesting "k8s.io/apiserver/pkg/storage/etcd3/testing"

	servertesting "github.com/openshift/openshift-apiserver/pkg/cmd/openshift-apiserver/testing"
)

func StartTestServerWithInProcessEtcd(t *testing.T) (result servertesting.TestServer, err error) {
	etcd, storageConfig := etcdtesting.NewUnsecuredEtcd3TestClientServer(t)
	s, err := servertesting.StartTestServer(t, servertesting.NewDefaultTestServerOptions(), storageConfig)
	if err != nil {
		return result, err
	}
	origTearDown := s.TearDownFn
	s.TearDownFn = func() {
		defer etcd.Terminate(t)
		origTearDown()
	}
	return s, nil
}
