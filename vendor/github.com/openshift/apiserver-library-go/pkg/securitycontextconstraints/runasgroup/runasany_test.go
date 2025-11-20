package runasgroup

import (
	"testing"

	securityv1 "github.com/openshift/api/security/v1"
)

func TestRunAsAnyOptions(t *testing.T) {
	tests := map[string]struct {
		opts *securityv1.RunAsGroupStrategyOptions
		pass bool
	}{
		"nil opts": {
			opts: nil,
			pass: true,
		},
		"empty opts": {
			opts: &securityv1.RunAsGroupStrategyOptions{},
			pass: true,
		},
		"opts with ranges": {
			opts: &securityv1.RunAsGroupStrategyOptions{
				Ranges: []securityv1.RunAsGroupIDRange{
					{Min: int64Ptr(1), Max: int64Ptr(10)},
				},
			},
			pass: true,
		},
	}
	for name, tc := range tests {
		_, err := NewRunAsAny(tc.opts)
		if err != nil && tc.pass {
			t.Errorf("%s expected to pass but received error %v", name, err)
		}
		if err == nil && !tc.pass {
			t.Errorf("%s expected to fail but did not receive an error", name)
		}
	}
}

func TestRunAsAnyGenerate(t *testing.T) {
	opts := &securityv1.RunAsGroupStrategyOptions{}
	runAsAny, err := NewRunAsAny(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewRunAsAny %v", err)
	}
	generated, err := runAsAny.Generate(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error generating gid %v", err)
	}
	if generated != nil {
		t.Errorf("expected nil gid from RunAsAny but got %v", *generated)
	}
}

func TestRunAsAnyValidate(t *testing.T) {
	opts := &securityv1.RunAsGroupStrategyOptions{}
	runAsAny, err := NewRunAsAny(opts)
	if err != nil {
		t.Fatalf("unexpected error initializing NewRunAsAny %v", err)
	}

	// RunAsAny should accept nil
	errs := runAsAny.Validate(nil, nil, nil, nil)
	if len(errs) != 0 {
		t.Errorf("expected no errors from nil runAsGroup but got %v", errs)
	}

	// RunAsAny should accept any GID
	var gid int64 = 0
	errs = runAsAny.Validate(nil, nil, nil, &gid)
	if len(errs) != 0 {
		t.Errorf("expected no errors from gid 0 but got %v", errs)
	}

	gid = 999
	errs = runAsAny.Validate(nil, nil, nil, &gid)
	if len(errs) != 0 {
		t.Errorf("expected no errors from gid 999 but got %v", errs)
	}

	gid = -1
	errs = runAsAny.Validate(nil, nil, nil, &gid)
	if len(errs) != 0 {
		t.Errorf("expected no errors from negative gid but got %v", errs)
	}
}
