package sandbox

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// stubRuntime implements SandboxRuntime for testing.
type stubRuntime struct {
	name          string
	verifyError   error
	validateError error
	applyError    error
	capabilities  Capabilities
	runConfig     SandboxConfig
}

func (s *stubRuntime) Name() string                                         { return s.name }
func (s *stubRuntime) Verify(_ context.Context) error                       { return s.verifyError }
func (s *stubRuntime) Capabilities() Capabilities                           { return s.capabilities }
func (s *stubRuntime) ValidatePolicy(_ SandboxPolicy) error                 { return s.validateError }
func (s *stubRuntime) ApplyPolicy(_ context.Context, _ SandboxPolicy) error { return s.applyError }
func (s *stubRuntime) Policy() SandboxPolicy                                { return SandboxPolicy{} }
func (s *stubRuntime) RunConfig() SandboxConfig                             { return s.runConfig }

// stubProviderRuntime implements both SandboxRuntime and CommandRunnerProvider.
type stubProviderRuntime struct {
	stubRuntime
	runner CommandRunner
}

func (s *stubProviderRuntime) NewCommandRunner(_ *sandbox.CommandRunnerConfig) (CommandRunner, error) {
	return s.runner, nil
}

// stubRunner implements CommandRunner for testing.
type stubRunner struct{}

func (s *stubRunner) Run(_ context.Context, _ CommandRequest) (*ports.CommandResult, error) {
	return &ports.CommandResult{}, nil
}

var (
	_ SandboxRuntime        = (*stubRuntime)(nil)
	_ CommandRunnerProvider = (*stubProviderRuntime)(nil)
	_ CommandRunner         = (*stubRunner)(nil)
)

func TestNewVerifiedCommandRunner_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ws := t.TempDir()

	rt := &stubRuntime{
		name: "test",
		capabilities: Capabilities{
			NetworkIsolation: true,
			ReadOnlyRoot:     true,
		},
	}
	policy := SandboxPolicy{}
	config := &sandbox.CommandRunnerConfig{
		Workspace: ws,
	}

	runner, err := NewVerifiedCommandRunner(ctx, rt, policy, config)
	if err != nil {
		t.Fatalf("NewVerifiedCommandRunner failed: %v", err)
	}
	if runner == nil {
		t.Fatal("NewVerifiedCommandRunner returned nil runner")
	}
}

func TestNewVerifiedCommandRunner_VerifyFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ws := t.TempDir()

	expectedErr := errors.New("verify failed")
	rt := &stubRuntime{
		name:        "test",
		verifyError: expectedErr,
	}
	policy := SandboxPolicy{}
	config := &sandbox.CommandRunnerConfig{
		Workspace: ws,
	}

	runner, err := NewVerifiedCommandRunner(ctx, rt, policy, config)
	if err == nil {
		t.Fatal("NewVerifiedCommandRunner should fail when Verify fails")
	}
	if runner != nil {
		t.Errorf("expected nil runner, got %T", runner)
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("error %v does not wrap %v", err, expectedErr)
	}
}

func TestNewVerifiedCommandRunner_NilRuntime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	policy := SandboxPolicy{}
	config := &sandbox.CommandRunnerConfig{
		Workspace: "/tmp/test",
	}

	runner, err := NewVerifiedCommandRunner(ctx, nil, policy, config)
	if err == nil {
		t.Fatal("NewVerifiedCommandRunner should fail with nil runtime")
	}
	if runner != nil {
		t.Errorf("expected nil runner, got %T", runner)
	}
}

func TestNewVerifiedCommandRunner_CommandRunnerProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fakeRunner := &stubRunner{}
	rt := &stubProviderRuntime{
		stubRuntime: stubRuntime{
			name: "provider",
		},
		runner: fakeRunner,
	}
	policy := SandboxPolicy{}
	config := &sandbox.CommandRunnerConfig{}

	runner, err := NewVerifiedCommandRunner(ctx, rt, policy, config)
	if err != nil {
		t.Fatalf("NewVerifiedCommandRunner failed: %v", err)
	}
	if runner != fakeRunner {
		t.Errorf("expected provider's runner (%T), got %T", fakeRunner, runner)
	}
}

func TestNewVerifiedCommandRunner_ValidatePolicyFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ws := t.TempDir()

	expectedErr := errors.New("validate failed")
	rt := &stubRuntime{
		name:          "test",
		validateError: expectedErr,
	}
	policy := SandboxPolicy{}
	config := &sandbox.CommandRunnerConfig{
		Workspace: ws,
	}

	runner, err := NewVerifiedCommandRunner(ctx, rt, policy, config)
	if err == nil {
		t.Fatal("NewVerifiedCommandRunner should fail when ValidatePolicy fails")
	}
	if runner != nil {
		t.Errorf("expected nil runner, got %T", runner)
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("error %v does not wrap %v", err, expectedErr)
	}
}

func TestNewVerifiedCommandRunner_ApplyPolicyFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ws := t.TempDir()

	expectedErr := errors.New("apply failed")
	rt := &stubRuntime{
		name:       "test",
		applyError: expectedErr,
	}
	policy := SandboxPolicy{}
	config := &sandbox.CommandRunnerConfig{
		Workspace: ws,
	}

	runner, err := NewVerifiedCommandRunner(ctx, rt, policy, config)
	if err == nil {
		t.Fatal("NewVerifiedCommandRunner should fail when ApplyPolicy fails")
	}
	if runner != nil {
		t.Errorf("expected nil runner, got %T", runner)
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("error %v does not wrap %v", err, expectedErr)
	}
}
