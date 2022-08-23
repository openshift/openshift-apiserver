package internalversion

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/openshift/openshift-apiserver/pkg/apps/apis/apps"
	kprinters "k8s.io/kubernetes/pkg/printers"
)

func TestPrinterWithDeepCopy(t *testing.T) {
	dc := apps.DeploymentConfig{}
	rows, err := printDeploymentConfig(&dc, kprinters.GenerateOptions{})

	if err != nil {
		t.Fatalf("expected no error, but got: %#v", err)
	}
	if len(rows) <= 0 {
		t.Fatalf("expected to have at least one TableRow, but got: %d", len(rows))
	}

	func() {
		defer func() {
			if err := recover(); err != nil {
				// Same as stdlib http server code. Manually allocate stack
				// trace buffer size to prevent excessively large logs
				const size = 64 << 10
				buf := make([]byte, size)
				buf = buf[:runtime.Stack(buf, false)]
				err = fmt.Errorf("%q stack:\n%s", err, buf)

				t.Errorf("Expected no panic, but got: %v", err)
			}
		}()

		// should not panic
		rows[0].DeepCopy()
	}()
}
