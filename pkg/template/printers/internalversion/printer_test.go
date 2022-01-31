package internalversion

import (
	"testing"

	templateapi "github.com/openshift/openshift-apiserver/pkg/template/apis/template"
	kprinters "k8s.io/kubernetes/pkg/printers"
)

func TestPrintTemplateWithDeepCopy(t *testing.T) {
	template := templateapi.Template{}
	rows, err := printTemplate(&template, kprinters.GenerateOptions{})

	if err != nil {
		t.Fatalf("expected no error, but got: %#v", err)
	}
	if len(rows) <= 0 {
		t.Fatalf("expected to have at least one TableRow, but got: %d", len(rows))
	}

	// should not panic
	rows[0].DeepCopy()
}
