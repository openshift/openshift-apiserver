package internalversion

import (
	"testing"

	securityapi "github.com/openshift/openshift-apiserver/pkg/security/apis/security"
	kprinters "k8s.io/kubernetes/pkg/printers"
)

func TestPrintSecurityContextConstraintWithDeepCopy(t *testing.T) {
	scc := securityapi.SecurityContextConstraints{}
	rows, err := printSecurityContextConstraint(&scc, kprinters.GenerateOptions{})

	if err != nil {
		t.Fatalf("expected no error, but got: %#v", err)
	}
	if len(rows) <= 0 {
		t.Fatalf("expected to have at least one TableRow, but got: %d", len(rows))
	}

	// should not panic
	rows[0].DeepCopy()
}
