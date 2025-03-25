package importer_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	restclient "k8s.io/client-go/rest"
	kapi "k8s.io/kubernetes/pkg/apis/core"

	imageapi "github.com/openshift/openshift-apiserver/pkg/image/apis/image"
	"github.com/openshift/openshift-apiserver/pkg/image/apiserver/importer"
)

const (
	pulpRegistryName = "registry.access.redhat.com"
	quayRegistryName = "quay.io"

	maxRetryCount = 4
	retryAfter    = time.Millisecond * 500
)

var (
	// Below are lists of error patterns for use with `retryOnErrors` utility.

	// unreachableErrorPatterns will match following error examples:
	//   Get https://registry.com/v2/: dial tcp registry.com:443: i/o timeout
	//   Get https://registry.com/v2/: dial tcp: lookup registry.com: no such host
	//   Get https://registry.com/v2/: dial tcp registry.com:443: getsockopt: connection refused
	//   Get https://registry.com/v2/: read tcp 127.0.0.1:39849->registry.com:443: read: connection reset by peer
	//   Get https://registry.com/v2/: net/http: request cancelled while waiting for connection
	//   Get https://registry.com/v2/: net/http: TLS handshake timeout
	//   the registry "https://registry.com/v2/" could not be reached
	unreachableErrorPatterns = []string{
		"dial tcp",
		"read tcp",
		"net/http",
		"could not be reached",
	}

	// imageNotFoundErrorPatterns will match following error examples:
	//   the image "..." in repository "..." was not found and may have been deleted
	//   tag "..." has not been set on repository "..."
	// use only with non-internal registry
	imageNotFoundErrorPatterns = []string{
		"was not found and may have been deleted",
		"has not been set on repository",
	}
)

// retryOnErrors invokes given function several times until it succeeds,
// returns unexpected error or a maximum number of attempts is reached. It
// should be used to wrap calls to remote registry to prevent test failures
// because of short-term outages or image updates.
func retryOnErrors(t *testing.T, errorPatterns []string, f func() error) error {
	timeout := retryAfter
	attempt := 0
	for err := f(); err != nil; err = f() {
		match := false
		for _, pattern := range errorPatterns {
			if strings.Contains(err.Error(), pattern) {
				match = true
				break
			}
		}

		if !match || attempt >= maxRetryCount {
			return err
		}

		t.Logf("caught error \"%v\", retrying in %s", err, timeout.String())
		time.Sleep(timeout)
		timeout = timeout * 2
		attempt += 1
	}
	return nil
}

// retryWhenUnreachable is a convenient wrapper for retryOnErrors that makes it
// retry when the registry is not reachable. Additional error patterns may
// follow.
func retryWhenUnreachable(t *testing.T, f func() error, errorPatterns ...string) error {
	return retryOnErrors(t, append(errorPatterns, unreachableErrorPatterns...), f)
}

func TestImageStreamImportQuayIO(t *testing.T) {
	rt, _ := restclient.TransportFor(&restclient.Config{})
	importCtx := importer.NewStaticCredentialsContext(rt, nil, nil)

	repositoryName := quayRegistryName + "/coreos/etcd-operator"
	imports := &imageapi.ImageStreamImport{
		Spec: imageapi.ImageStreamImportSpec{
			Images: []imageapi.ImageImportSpec{
				{From: kapi.ObjectReference{Kind: "DockerImage", Name: repositoryName}},
			},
		},
	}

	err := retryWhenUnreachable(t, func() error {
		i := importer.NewImageStreamImporter(importCtx, nil, 3, nil, nil)
		if err := i.Import(context.Background(), imports, &imageapi.ImageStream{}); err != nil {
			return err
		}

		errs := []error{}
		for i, d := range imports.Status.Images {
			fromName := imports.Spec.Images[i].From.Name
			if d.Status.Status != metav1.StatusSuccess {
				if d.Status.Reason == "NotV2Registry" {
					t.Skipf("the server did not report as a v2 registry: %#v", d.Status)
				}
				errs = append(errs, fmt.Errorf("failed to import an image %s: %v", fromName, d.Status.Message))
			}
		}
		return kerrors.NewAggregate(errs)
	}, imageNotFoundErrorPatterns...)
	if err != nil {
		t.Fatal(err)
	}

	if imports.Status.Repository != nil {
		t.Errorf("unexpected repository: %#v", imports.Status.Repository)
	}
	if len(imports.Status.Images) != 1 {
		t.Fatalf("unexpected response: %#v", imports.Status.Images)
	}
	d := imports.Status.Images[0]
	if d.Image == nil || len(d.Image.DockerImageManifest) == 0 || !strings.HasPrefix(d.Image.DockerImageReference, repositoryName+"@") || len(d.Image.DockerImageMetadata.ID) == 0 || len(d.Image.DockerImageLayers) == 0 {
		s := spew.ConfigState{
			Indent: " ",
			// Extra deep spew.
			DisableMethods: true,
		}
		t.Logf("import: %s", s.Sdump(d))
		t.Fatalf("unexpected object: %#v", d.Image)
	}
}

func TestImageStreamImportRedHatRegistry(t *testing.T) {
	rt, _ := restclient.TransportFor(&restclient.Config{})
	importCtx := importer.NewStaticCredentialsContext(rt, nil, nil)

	repositoryName := pulpRegistryName + "/rhel7"
	// test without the client on the context
	imports := &imageapi.ImageStreamImport{
		Spec: imageapi.ImageStreamImportSpec{
			Images: []imageapi.ImageImportSpec{
				{From: kapi.ObjectReference{Kind: "DockerImage", Name: repositoryName}},
			},
		},
	}

	i := importer.NewImageStreamImporter(importCtx, nil, 3, nil, nil)
	if err := i.Import(context.Background(), imports, &imageapi.ImageStream{}); err != nil {
		t.Fatal(err)
	}

	if imports.Status.Repository != nil {
		t.Errorf("unexpected repository: %#v", imports.Status.Repository)
	}
	if len(imports.Status.Images) != 1 {
		t.Fatalf("unexpected response: %#v", imports.Status.Images)
	}
	d := imports.Status.Images[0]
	if d.Image == nil || d.Status.Status == metav1.StatusFailure {
		t.Errorf("unexpected object: %#v", d.Status)
	}

	// test with the client on the context
	imports = &imageapi.ImageStreamImport{
		Spec: imageapi.ImageStreamImportSpec{
			Images: []imageapi.ImageImportSpec{
				{From: kapi.ObjectReference{Kind: "DockerImage", Name: repositoryName}},
			},
		},
	}
	context := context.Background()
	importCtx = importer.NewStaticCredentialsContext(rt, nil, nil)
	err := retryWhenUnreachable(t, func() error {
		i = importer.NewImageStreamImporter(importCtx, nil, 3, nil, nil)
		if err := i.Import(context, imports, &imageapi.ImageStream{}); err != nil {
			return err
		}

		errs := []error{}
		for i, d := range imports.Status.Images {
			fromName := imports.Spec.Images[i].From.Name
			if d.Status.Status != metav1.StatusSuccess {
				errs = append(errs, fmt.Errorf("failed to import an image %s: %v", fromName, d.Status.Message))
			}
		}
		return kerrors.NewAggregate(errs)
	}, imageNotFoundErrorPatterns...)
	if err != nil {
		if strings.Contains(err.Error(), "x509: certificate has expired or is not yet valid") {
			t.Skipf("SKIPPING: due to expired certificate of %s: %v", pulpRegistryName, err)
		}
		t.Fatal(err.Error())
	}

	if imports.Status.Repository != nil {
		t.Errorf("unexpected repository: %#v", imports.Status.Repository)
	}
	if len(imports.Status.Images) != 1 {
		t.Fatalf("unexpected response: %#v", imports.Status.Images)
	}
	d = imports.Status.Images[0]
	if d.Image == nil || len(d.Image.DockerImageManifest) == 0 || !strings.HasPrefix(d.Image.DockerImageReference, repositoryName) || len(d.Image.DockerImageMetadata.ID) == 0 || len(d.Image.DockerImageLayers) == 0 {
		t.Logf("imports: %#v", imports.Status.Images[0].Image)
		t.Fatalf("unexpected object: %#v", d.Image)
	}
}
