package internalversion

import (
	"fmt"
	"runtime"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kprinters "k8s.io/kubernetes/pkg/printers"

	"github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization"
)

func TestPrintersWithDeepCopy(t *testing.T) {
	tests := []struct {
		name    string
		printer func() ([]metav1.TableRow, error)
	}{
		{
			name: "RoleBindingRestriction",
			printer: func() ([]metav1.TableRow, error) {
				return printRoleBindingRestriction(&authorization.RoleBindingRestriction{}, kprinters.GenerateOptions{})
			},
		},
		{
			name: "IsPersonalSubjectAccessReview",
			printer: func() ([]metav1.TableRow, error) {
				return printIsPersonalSubjectAccessReview(&authorization.IsPersonalSubjectAccessReview{}, kprinters.GenerateOptions{})
			},
		},
		{
			name: "SubjectRulesReview",
			printer: func() ([]metav1.TableRow, error) {
				review := &authorization.SubjectRulesReview{}
				review.Status.Rules = []authorization.PolicyRule{
					{},
				}
				return printSubjectRulesReview(review, kprinters.GenerateOptions{})
			},
		},
		{
			name: "SelfSubjectRulesReview",
			printer: func() ([]metav1.TableRow, error) {
				review := &authorization.SelfSubjectRulesReview{}
				review.Status.Rules = []authorization.PolicyRule{
					{},
				}
				return printSelfSubjectRulesReview(review, kprinters.GenerateOptions{})
			},
		},
		{
			name: "ClusterRole",
			printer: func() ([]metav1.TableRow, error) {
				return printClusterRole(&authorization.ClusterRole{}, kprinters.GenerateOptions{})
			},
		},
		{
			name: "Role",
			printer: func() ([]metav1.TableRow, error) {
				return printRole(&authorization.Role{}, kprinters.GenerateOptions{})
			},
		},
		{
			name: "RoleBinding",
			printer: func() ([]metav1.TableRow, error) {
				return printRoleBinding(&authorization.RoleBinding{}, kprinters.GenerateOptions{})
			},
		},
		{
			name: "ClusterRoleBinding",
			printer: func() ([]metav1.TableRow, error) {
				return printClusterRoleBinding(&authorization.ClusterRoleBinding{}, kprinters.GenerateOptions{})
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
