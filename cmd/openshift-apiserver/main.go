package main

import (
	"os"
	"runtime"

	"github.com/spf13/cobra"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"

	"github.com/openshift/library-go/pkg/serviceability"
	openshiftapiserver "github.com/openshift/openshift-apiserver/pkg/cmd/openshift-apiserver"
	"github.com/openshift/openshift-apiserver/pkg/version"
)

func main() {
	stopCh := genericapiserver.SetupSignalHandler()
	defer serviceability.BehaviorOnPanic(os.Getenv("OPENSHIFT_ON_PANIC"), version.Get())()
	defer serviceability.Profile(os.Getenv("OPENSHIFT_PROFILE")).Stop()
	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	os.Exit(cli.Run(NewOpenshiftApiServerCommand(stopCh)))
}

func NewOpenshiftApiServerCommand(stopCh <-chan struct{}) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "openshift-apiserver",
		Short: "Command for the OpenShift API Server",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
			os.Exit(1)
		},
	}
	start := openshiftapiserver.NewOpenShiftAPIServerCommand("start", os.Stdout, os.Stderr, stopCh)
	cmd.AddCommand(start)

	return cmd
}
