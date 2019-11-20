package apps

import (
	"reflect"
	"testing"

	kapi "k8s.io/kubernetes/pkg/apis/core"
)

func TestLogOptionsDrift(t *testing.T) {
	popts := reflect.TypeOf(kapi.PodLogOptions{})
	dopts := reflect.TypeOf(DeploymentLogOptions{})

	for i := 0; i < popts.NumField(); i++ {
		// Verify name
		name := popts.Field(i).Name
		doptsField, found := dopts.FieldByName(name)
		// TODO: Add this option to deployment config log options
		if name == "InsecureSkipTLSVerifyBackend" {
			continue
		}
		if !found {
			t.Errorf("deploymentLogOptions drifting from podLogOptions! Field %q wasn't found!", name)
		}
		// Verify type
		if should, is := popts.Field(i).Type, doptsField.Type; is != should {
			t.Errorf("deploymentLogOptions drifting from podLogOptions! Field %q should be a %s but is %s!", name, should.String(), is.String())
		}
	}
}
