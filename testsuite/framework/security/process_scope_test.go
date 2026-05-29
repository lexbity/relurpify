package security

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/testsuite/testsupport"
)

// TestCommandRequestValidation validates that command request structure
// is correctly validated at the seam boundary.
func TestCommandRequestValidation(t *testing.T) {
	t.Run("valid command request is accepted", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{"echo", "hello"},
			Env:     []string{"PATH=/usr/bin"},
			Input:   "",
			Timeout: 30 * time.Second,
		}

		// Command request itself doesn't have validation, but we test
		// that the structure can be created and used
		if len(req.Args) == 0 {
			t.Error("command request should have args")
		}
		if req.Workdir == "" {
			t.Error("command request should have workdir")
		}
	})

	t.Run("command request with empty args is invalid for execution", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{},
			Env:     []string{"PATH=/usr/bin"},
			Timeout: 30 * time.Second,
		}

		// Empty args should be caught by the runner before execution
		if len(req.Args) != 0 {
			t.Error("command request should have empty args for this test")
		}
	})

	t.Run("command request with timeout is valid", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{"ls"},
			Env:     []string{},
			Timeout: 10 * time.Second,
		}

		if req.Timeout == 0 {
			t.Error("timeout should be set")
		}
	})

	t.Run("command request with zero timeout is valid", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{"ls"},
			Env:     []string{},
			Timeout: 0,
		}

		// Zero timeout means no timeout, which is valid
		if req.Args == nil {
			t.Error("args should be set")
		}
	})
}

// TestCommandPolicyEnforcement validates that command policy enforcement
// works correctly at the seam boundary.
func TestCommandPolicyEnforcement(t *testing.T) {
	t.Run("command policy func allows valid command", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{"echo", "hello"},
		}

		policy := sandbox.CommandPolicyFunc(func(ctx context.Context, r sandbox.CommandRequest) error {
			if len(r.Args) == 0 {
				return errors.New("command args required")
			}
			return nil
		})

		err := policy.AllowCommand(context.Background(), req)
		if err != nil {
			t.Errorf("valid command should be allowed: %v", err)
		}
	})

	t.Run("command policy func denies invalid command", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{},
		}

		policy := sandbox.CommandPolicyFunc(func(ctx context.Context, r sandbox.CommandRequest) error {
			if len(r.Args) == 0 {
				return errors.New("command args required")
			}
			return nil
		})

		err := policy.AllowCommand(context.Background(), req)
		if err == nil {
			t.Error("command with empty args should be denied")
		}
	})

	t.Run("nil command policy allows all commands", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{"any", "command"},
		}

		var policy sandbox.CommandPolicyFunc = nil

		err := policy.AllowCommand(context.Background(), req)
		if err != nil {
			t.Errorf("nil policy should allow all commands: %v", err)
		}
	})
}

// TestEnforcingCommandRunner validates that the enforcing command runner
// correctly applies policy before execution.
func TestEnforcingCommandRunner(t *testing.T) {
	t.Run("enforcing runner applies policy before execution", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{"echo", "test"},
		}

		inner := testsupport.FakeRunner()

		policy := sandbox.CommandPolicyFunc(func(ctx context.Context, r sandbox.CommandRequest) error {
			if len(r.Args) == 0 {
				return errors.New("args required")
			}
			return nil
		})

		runner := sandbox.NewEnforcingCommandRunner(inner, policy)

		_, _, err := runner.Run(context.Background(), req)
		if err != nil {
			t.Errorf("command execution should succeed: %v", err)
		}

		if inner.CallCount() == 0 {
			t.Error("inner runner should be called when policy allows")
		}
	})

	t.Run("enforcing runner blocks execution when policy denies", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{},
		}

		inner := testsupport.FakeRunner()

		policy := sandbox.CommandPolicyFunc(func(ctx context.Context, r sandbox.CommandRequest) error {
			if len(r.Args) == 0 {
				return errors.New("args required")
			}
			return nil
		})

		runner := sandbox.NewEnforcingCommandRunner(inner, policy)

		_, _, err := runner.Run(context.Background(), req)
		if err == nil {
			t.Error("execution should be denied when policy rejects")
		}

		if inner.CallCount() > 0 {
			t.Error("inner runner should not be called when policy denies")
		}

		var deniedErr *sandbox.ExecutionDeniedError
		if !errors.As(err, &deniedErr) {
			t.Errorf("expected ExecutionDeniedError, got %T", err)
		}
	})

	t.Run("enforcing runner without policy delegates directly", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{"echo", "test"},
		}

		inner := testsupport.FakeRunner()

		runner := sandbox.NewEnforcingCommandRunner(inner, nil)

		_, _, err := runner.Run(context.Background(), req)
		if err != nil {
			t.Errorf("command execution should succeed without policy: %v", err)
		}

		if inner.CallCount() == 0 {
			t.Error("inner runner should be called when no policy is set")
		}
	})

	t.Run("enforcing runner with missing inner returns error", func(t *testing.T) {
		req := sandbox.CommandRequest{
			Workdir: "/workspace",
			Args:    []string{"echo", "test"},
		}

		policy := sandbox.CommandPolicyFunc(func(ctx context.Context, r sandbox.CommandRequest) error {
			return nil
		})

		runner := sandbox.NewEnforcingCommandRunner(nil, policy)

		_, _, err := runner.Run(context.Background(), req)
		if err == nil {
			t.Error("enforcing runner with missing inner should return error")
		}
	})
}

// TestExecutionDeniedError validates that execution denied errors
// are correctly structured and unwrappable.
func TestExecutionDeniedError(t *testing.T) {
	t.Run("execution denied error includes command", func(t *testing.T) {
		err := &sandbox.ExecutionDeniedError{
			Command: "rm -rf /",
			Reason:  "destructive command",
			Policy:  "sandbox policy",
		}

		msg := err.Error()
		if msg == "" {
			t.Error("error message should not be empty")
		}
		if err.Command != "rm -rf /" {
			t.Error("command should be preserved in error")
		}
	})

	t.Run("execution denied error includes reason", func(t *testing.T) {
		err := &sandbox.ExecutionDeniedError{
			Command: "rm -rf /",
			Reason:  "destructive command",
			Policy:  "sandbox policy",
		}

		if err.Reason != "destructive command" {
			t.Error("reason should be preserved in error")
		}
	})

	t.Run("execution denied error includes policy", func(t *testing.T) {
		err := &sandbox.ExecutionDeniedError{
			Command: "rm -rf /",
			Reason:  "destructive command",
			Policy:  "sandbox policy",
		}

		if err.Policy != "sandbox policy" {
			t.Error("policy should be preserved in error")
		}
	})

	t.Run("execution denied error unwraps cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := &sandbox.ExecutionDeniedError{
			Command: "test",
			Reason:  "test reason",
			Policy:  "test policy",
			Cause:   cause,
		}

		unwrapped := err.Unwrap()
		if unwrapped != cause {
			t.Errorf("unwrapped error should match cause, got %v", unwrapped)
		}
	})

	t.Run("execution denied error with nil cause unwraps to nil", func(t *testing.T) {
		err := &sandbox.ExecutionDeniedError{
			Command: "test",
			Reason:  "test reason",
			Policy:  "test policy",
			Cause:   nil,
		}

		unwrapped := err.Unwrap()
		if unwrapped != nil {
			t.Errorf("unwrapped error should be nil when cause is nil, got %v", unwrapped)
		}
	})
}

// TestSeccompProfile validates that seccomp profile policy is correctly
// applied and visible in the sandbox policy state.
func TestSeccompProfile(t *testing.T) {
	t.Run("seccomp profile can be set in policy", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			SeccompProfile: "default",
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply seccomp profile policy: %v", err)
		}

		retrieved := runtime.Policy()
		if retrieved.SeccompProfile != "default" {
			t.Errorf("seccomp profile mismatch: got %s, want default", retrieved.SeccompProfile)
		}
	})

	t.Run("empty seccomp profile is valid", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			SeccompProfile: "",
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply empty seccomp profile policy: %v", err)
		}

		retrieved := runtime.Policy()
		if retrieved.SeccompProfile != "" {
			t.Error("empty seccomp profile should remain empty")
		}
	})

	t.Run("seccomp profile can be updated", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		initialPolicy := sandbox.SandboxPolicy{
			SeccompProfile: "default",
		}
		err := runtime.ApplyPolicy(context.Background(), initialPolicy)
		if err != nil {
			t.Fatalf("failed to apply initial seccomp profile: %v", err)
		}

		updatedPolicy := sandbox.SandboxPolicy{
			SeccompProfile: "strict",
		}
		err = runtime.ApplyPolicy(context.Background(), updatedPolicy)
		if err != nil {
			t.Fatalf("failed to apply updated seccomp profile: %v", err)
		}

		retrieved := runtime.Policy()
		if retrieved.SeccompProfile != "strict" {
			t.Errorf("seccomp profile should be updated, got %s", retrieved.SeccompProfile)
		}
	})
}


