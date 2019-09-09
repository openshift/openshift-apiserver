/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testing

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	openshiftapiserver "github.com/openshift/openshift-apiserver/pkg/cmd/openshift-apiserver"
	openshiftapiserverconfig "github.com/openshift/openshift-apiserver/pkg/cmd/openshift-apiserver/openshiftapiserver"
)

// TearDownFunc is to be called to tear down a test server.
type TearDownFunc func()

// TestServerInstanceOptions Instance options the TestServer
type TestServerInstanceOptions struct {
	// DisableStorageCleanup Disable the automatic storage cleanup
	DisableStorageCleanup bool
}

// TestServer is the result of test server startup.
type TestServer struct {
	ClientConfig *restclient.Config // Rest client config
	TearDownFn   TearDownFunc       // TearDown function
	TmpDir       string             // Temp Dir used, by the apiserver
}

// Logger allows t.Testing and b.Testing to be passed to StartTestServer and StartTestServerOrDie
type Logger interface {
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
}

// NewDefaultTestServerOptions Default options for TestServer instances
func NewDefaultTestServerOptions() *TestServerInstanceOptions {
	return &TestServerInstanceOptions{
		DisableStorageCleanup: false,
	}
}

// StartTestServer starts a openshift-apiserver. A rest client config and a tear-down func,
// and location of the tmpdir are returned.
//
// Note: we return a tear-down func instead of a stop channel because the later will leak temporary
// 		 files that because Golang testing's call to os.Exit will not give a stop channel go routine
// 		 enough time to remove temporary files.
func StartTestServer(t Logger, instanceOptions *TestServerInstanceOptions, storageConfig *storagebackend.Config) (result TestServer, err error) {
	if instanceOptions == nil {
		instanceOptions = NewDefaultTestServerOptions()
	}

	// TODO : Remove TrackStorageCleanup below when PR
	// https://github.com/kubernetes/kubernetes/pull/50690
	// merges as that shuts down storage properly
	if !instanceOptions.DisableStorageCleanup {
		registry.TrackStorageCleanup()
	}

	stopCh := make(chan struct{})
	tearDown := func() {
		if !instanceOptions.DisableStorageCleanup {
			registry.CleanupStorage()
		}
		close(stopCh)
		if len(result.TmpDir) != 0 {
			os.RemoveAll(result.TmpDir)
		}
	}
	defer func() {
		if result.TearDownFn == nil {
			tearDown()
		}
	}()

	result.TmpDir, err = ioutil.TempDir("", "openshift-apiserver")
	if err != nil {
		return result, fmt.Errorf("failed to create temp dir: %v", err)
	}

	configFile := filepath.Join(result.TmpDir, "config.yaml")
	if err := ioutil.WriteFile(configFile, []byte(`
apiVersion: openshiftcontrolplane.config.openshift.io/v1
kind: OpenShiftAPIServerConfig
`), 0755); err != nil {
		return result, err
	}

	var port int
	options := openshiftapiserver.OpenShiftAPIServer{
		Output:     os.Stdout,
		ConfigFile: configFile,
		PatchServing: func(o *options.SecureServingOptionsWithLoopback) error {
			o.Listener, o.BindPort, err = createLocalhostListenerOnFreePort()
			if err != nil {
				return fmt.Errorf("failed to create listener: %v", err)
			}
			port = o.BindPort
			o.ServerCert.CertDirectory = result.TmpDir
			o.ExternalAddress = o.Listener.Addr().(*net.TCPAddr).IP // use listener addr although it is a loopback device

			_, thisFile, _, ok := runtime.Caller(0)
			if !ok {
				return fmt.Errorf("failed to get current file")
			}
			o.ServerCert.FixtureDirectory = path.Join(path.Dir(thisFile), "testdata")

			return nil
		},
		PatchEtcd: func(o *options.EtcdOptions) error {
			o.StorageConfig = *storageConfig
			return nil
		},
	}

	if err := options.Validate(); err != nil {
		return result, fmt.Errorf("failed to validate options: %v", err)
	}

	t.Logf("Starting openshift-apiserver on port %d...", port)

	errCh := make(chan error)
	srvCh := make(chan *openshiftapiserverconfig.OpenshiftAPIServer)
	go func(stopCh <-chan struct{}) {
		if err := options.RunAPIServer(srvCh, stopCh); err != nil {
			errCh <- err
		}
	}(stopCh)

	var server *openshiftapiserverconfig.OpenshiftAPIServer
	select {
	case err := <-errCh:
		return result, err
	case s := <-srvCh:
		server = s
	}

	t.Logf("Waiting for /healthz to be ok...")

	client, err := kubernetes.NewForConfig(server.GenericAPIServer.LoopbackClientConfig)
	if err != nil {
		return result, fmt.Errorf("failed to create a client: %v", err)
	}
	err = wait.Poll(100*time.Millisecond, 30*time.Second, func() (bool, error) {
		select {
		case err := <-errCh:
			return false, err
		default:
		}

		result := client.CoreV1().RESTClient().Get().AbsPath("/healthz").Do()
		status := 0
		result.StatusCode(&status)
		if status == 200 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return result, fmt.Errorf("failed to wait for /healthz to return ok: %v", err)
	}

	// from here the caller must call tearDown
	result.ClientConfig = server.GenericAPIServer.LoopbackClientConfig
	result.TearDownFn = tearDown

	return result, nil
}

// StartTestServerOrDie calls StartTestServer t.Fatal if it does not succeed.
func StartTestServerOrDie(t Logger, instanceOptions *TestServerInstanceOptions, flags []string, storageConfig *storagebackend.Config) *TestServer {
	result, err := StartTestServer(t, instanceOptions, storageConfig)
	if err == nil {
		return &result
	}

	t.Fatalf("failed to launch server: %v", err)
	return nil
}

func createLocalhostListenerOnFreePort() (net.Listener, int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}

	// get port
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		ln.Close()
		return nil, 0, fmt.Errorf("invalid listen address: %q", ln.Addr().String())
	}

	return ln, tcpAddr.Port, nil
}
