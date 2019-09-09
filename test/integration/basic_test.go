package integration

import (
	"testing"

	"github.com/openshift/openshift-apiserver/test/integration/fixtures"
)

func TestServerUp(t *testing.T) {
	s, err := fixtures.StartTestServerWithInProcessEtcd(t)
	if err != nil {
		t.Fatal(err)
	}
	defer s.TearDownFn()
}
