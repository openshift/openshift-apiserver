package internalversion

import (
	"fmt"
	"runtime"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kprinters "k8s.io/kubernetes/pkg/printers"

	"github.com/openshift/openshift-apiserver/pkg/route/apis/route"
)

func TestPrintersWithDeepCopy(t *testing.T) {
	tests := []struct {
		name    string
		printer func() ([]metav1.TableRow, error)
	}{
		{
			name: "Route",
			printer: func() ([]metav1.TableRow, error) {
				return printRoute(&route.Route{}, kprinters.GenerateOptions{})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows, err := test.printer()
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

		})
	}
}
