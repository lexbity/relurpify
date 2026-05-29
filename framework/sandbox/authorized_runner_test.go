// Phase 2 — AuthorizedRunner type seam (compile-edge enforcement).
//
// Asserts that NewAuthorizedRunner rejects nil policy, and that the
// AuthorizedRunner correctly delegates to the inner enforced runner.
//
// See devdocs/plans/unified-boot-contract.md for the full plan.

package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewAuthorizedRunnerRejectsNilPolicy(t *testing.T) {
	fake := &fakeRunner{}
	_, err := NewAuthorizedRunner(fake, nil)
	if err == nil {
		t.Fatal("NewAuthorizedRunner should reject nil policy")
	}
	if !strings.Contains(err.Error(), "non-nil") && !strings.Contains(err.Error(), "policy") {
		t.Errorf("error should mention policy requirement, got: %v", err)
	}
}

func TestAuthorizedRunnerDelegatesAndEnforces(t *testing.T) {
	inner := &fakeRunner{}
	policy := CommandPolicyFunc(func(_ context.Context, req CommandRequest) error {
		for _, arg := range req.Args {
			if arg == "deny" {
				return errors.New("policy denied")
			}
		}
		return nil
	})

	auth, err := NewAuthorizedRunner(inner, policy)
	if err != nil {
		t.Fatalf("NewAuthorizedRunner failed: %v", err)
	}

	// Allowed command should delegate to inner runner.
	req := CommandRequest{Args: []string{"echo", "hello"}}
	stdout, stderr, err := auth.Run(context.Background(), req)
	if err != nil {
		t.Errorf("allowed command should not error, got: %v", err)
	}
	if stdout != "ok" {
		t.Errorf("stdout = %q, want %q", stdout, "ok")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}

	// Denied command should be blocked before reaching inner runner.
	denyReq := CommandRequest{Args: []string{"deny", "something"}}
	_, _, err = auth.Run(context.Background(), denyReq)
	if err == nil {
		t.Fatal("denied command should error")
	}
	var denied *ExecutionDeniedError
	if !errors.As(err, &denied) {
		t.Errorf("expected ExecutionDeniedError, got %T: %v", err, err)
	}
}

// fakeRunner is a simple CommandRunner that returns canned output.
type fakeRunner struct{}

func (f *fakeRunner) Run(_ context.Context, req CommandRequest) (string, string, error) {
	for _, arg := range req.Args {
		if arg == "error" {
			return "", "", errors.New("runner error")
		}
	}
	return "ok", "", nil
}

var _ CommandRunner = (*fakeRunner)(nil)
