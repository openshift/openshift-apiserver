package internalversion

import (
	"testing"

	projectapi "github.com/openshift/openshift-apiserver/pkg/project/apis/project"
	kprinters "k8s.io/kubernetes/pkg/printers"
)

func TestPrintProjectWithDeepCopy(t *testing.T) {
	p := projectapi.Project{}
	rows, err := printProject(&p, kprinters.GenerateOptions{})

	if err != nil {
		t.Fatalf("expected no error, but got: %#v", err)
	}
	if len(rows) <= 0 {
		t.Fatalf("expected to have at least one TableRow, but got: %d", len(rows))
	}

	// should not panic
	rows[0].DeepCopy()
}
