package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	sandbox2 "codeburg.org/lexbit/relurpify/framework/sandbox"
)

type fakeToolRunner struct{}

func (f *fakeToolRunner) Run(_ context.Context, _ sandbox2.CommandRequest) (string, string, error) {
	return "", "", nil
}

var _ sandbox2.CommandRunner = (*fakeToolRunner)(nil)

func TestNewToolExecCmd_VerifyFailure(t *testing.T) {
	old := buildToolExecRunner
	buildToolExecRunner = func(_ context.Context, _, _ string) (sandbox2.CommandRunner, error) {
		return nil, errors.New("sandbox verify failed")
	}
	defer func() { buildToolExecRunner = old }()

	workspace = t.TempDir()
	sandboxBackend = "gvisor"

	cmd := newToolExecCmd()
	cmd.SetArgs([]string{"cli_echo", "--args", `{"args":["hello"]}`})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from sandbox verification failure")
	}
	if !strings.Contains(err.Error(), "sandbox") {
		t.Errorf("expected error mentioning sandbox, got: %v", err)
	}
}

func TestNewToolExecCmd_SandboxRunnerUsed(t *testing.T) {
	runnerCalled := false
	old := buildToolExecRunner
	buildToolExecRunner = func(_ context.Context, _, _ string) (sandbox2.CommandRunner, error) {
		runnerCalled = true
		return &fakeToolRunner{}, nil
	}
	defer func() { buildToolExecRunner = old }()

	workspace = t.TempDir()
	sandboxBackend = "gvisor"

	// We test that the runner construction hook is called. The actual tool
	// invocation may succeed or fail depending on args and tool schema, but
	// the critical security invariant is that buildToolExecRunner was used.
	cmd := newToolExecCmd()
	cmd.SetArgs([]string{"cli_echo", "--args", `{"args":["hello"]}`})
	_ = cmd.Execute()

	if !runnerCalled {
		t.Error("buildToolExecRunner was not called; command bypassed the sandbox runner")
	}
}

func TestBuildToolExecRunner_Success(t *testing.T) {
	if _, err := exec.LookPath("runsc"); err != nil {
		t.Skip("skipping: runsc not found on PATH")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("skipping: docker not found on PATH")
	}
	// Direct unit test of the success path: verify the seam returns a non-nil
	// runner when no error occurs.
	sboxRunner, err := buildToolExecRunner(context.Background(), t.TempDir(), "")
	if err != nil {
		t.Fatalf("buildToolExecRunner failed: %v", err)
	}
	if sboxRunner == nil {
		t.Fatal("buildToolExecRunner returned nil runner")
	}
}
