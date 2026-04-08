//go:build !integration
// +build !integration

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- NewLocalCommandRunner ----

func TestNewLocalCommandRunner_EmptyWorkspaceDefaultsToDot(t *testing.T) {
	r := NewLocalCommandRunner("", nil)
	require.NotNil(t, r)
	// Empty workspace resolves to cwd via filepath.Abs(".")
	assert.NotEmpty(t, r.workspace)
	assert.True(t, filepath.IsAbs(r.workspace))
}

func TestNewLocalCommandRunner_RelativePathResolved(t *testing.T) {
	r := NewLocalCommandRunner(".", nil)
	require.NotNil(t, r)
	assert.True(t, filepath.IsAbs(r.workspace))
}

func TestNewLocalCommandRunner_AbsolutePathPreserved(t *testing.T) {
	r := NewLocalCommandRunner("/tmp", nil)
	require.NotNil(t, r)
	assert.Equal(t, "/tmp", r.workspace)
}

func TestNewLocalCommandRunner_ExtraEnvCopied(t *testing.T) {
	env := []string{"FOO=bar", "BAZ=qux"}
	r := NewLocalCommandRunner("/tmp", env)
	require.NotNil(t, r)
	assert.Equal(t, env, r.extraEnv)
	// Verify it's a copy, not a reference.
	env[0] = "MUTATED=yes"
	assert.Equal(t, "FOO=bar", r.extraEnv[0])
}

// ---- LocalCommandRunner.Run — early-exit guards ----

func TestLocalCommandRunner_Run_NilReceiver(t *testing.T) {
	var r *LocalCommandRunner
	_, _, err := r.Run(context.Background(), CommandRequest{Args: []string{"echo", "hi"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestLocalCommandRunner_Run_EmptyArgs(t *testing.T) {
	r := NewLocalCommandRunner("/tmp", nil)
	_, _, err := r.Run(context.Background(), CommandRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "arguments required")
}

// ---- LocalCommandRunner.Run — actual execution ----

func TestLocalCommandRunner_Run_BasicExecution(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalCommandRunner(dir, nil)
	stdout, stderr, err := r.Run(context.Background(), CommandRequest{
		Args: []string{"echo", "hello"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hello\n", stdout)
	assert.Empty(t, stderr)
}

func TestLocalCommandRunner_Run_CapturesStderr(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalCommandRunner(dir, nil)
	// sh -c writes to both stdout and stderr.
	stdout, stderr, err := r.Run(context.Background(), CommandRequest{
		Args: []string{"sh", "-c", "echo out; echo err >&2"},
	})
	require.NoError(t, err)
	assert.Equal(t, "out\n", stdout)
	assert.Equal(t, "err\n", stderr)
}

func TestLocalCommandRunner_Run_StdinInput(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalCommandRunner(dir, nil)
	stdout, _, err := r.Run(context.Background(), CommandRequest{
		Args:  []string{"cat"},
		Input: "stdin-content",
	})
	require.NoError(t, err)
	assert.Equal(t, "stdin-content", stdout)
}

func TestLocalCommandRunner_Run_CommandNotFound_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalCommandRunner(dir, nil)
	_, _, err := r.Run(context.Background(), CommandRequest{
		Args: []string{"__no_such_binary_xyzzy__"},
	})
	require.Error(t, err)
}

func TestLocalCommandRunner_Run_NonZeroExitReturnsError(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalCommandRunner(dir, nil)
	_, _, err := r.Run(context.Background(), CommandRequest{
		Args: []string{"sh", "-c", "exit 1"},
	})
	require.Error(t, err)
}

func TestLocalCommandRunner_Run_ExtraEnvPropagated(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalCommandRunner(dir, []string{"MY_EXTRA=injected"})
	stdout, _, err := r.Run(context.Background(), CommandRequest{
		Args: []string{"sh", "-c", "echo $MY_EXTRA"},
	})
	require.NoError(t, err)
	assert.Equal(t, "injected\n", stdout)
}

func TestLocalCommandRunner_Run_RequestEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalCommandRunner(dir, []string{"MY_VAR=base"})
	stdout, _, err := r.Run(context.Background(), CommandRequest{
		Args: []string{"sh", "-c", "echo $MY_VAR"},
		Env:  []string{"MY_VAR=override"},
	})
	require.NoError(t, err)
	// Request env is appended last, shell uses last defined value.
	assert.Equal(t, "override\n", stdout)
}

func TestLocalCommandRunner_Run_TimeoutEnforced(t *testing.T) {
	dir := t.TempDir()
	r := NewLocalCommandRunner(dir, nil)
	start := time.Now()
	_, _, err := r.Run(context.Background(), CommandRequest{
		Args:    []string{"sleep", "10"},
		Timeout: 50 * time.Millisecond,
	})
	elapsed := time.Since(start)
	require.Error(t, err)
	// Should have been killed well before 10 seconds.
	assert.Less(t, elapsed, 3*time.Second)
}

func TestLocalCommandRunner_Run_WithWorkdir(t *testing.T) {
	workspace := t.TempDir()
	subdir := filepath.Join(workspace, "sub")
	require.NoError(t, mkdirAll(subdir))

	r := NewLocalCommandRunner(workspace, nil)
	stdout, _, err := r.Run(context.Background(), CommandRequest{
		Args:    []string{"sh", "-c", "pwd"},
		Workdir: subdir,
	})
	require.NoError(t, err)
	assert.Equal(t, subdir+"\n", stdout)
}

func TestLocalCommandRunner_Run_RelativeWorkdir(t *testing.T) {
	workspace := t.TempDir()
	subdir := filepath.Join(workspace, "rel")
	require.NoError(t, mkdirAll(subdir))

	r := NewLocalCommandRunner(workspace, nil)
	stdout, _, err := r.Run(context.Background(), CommandRequest{
		Args:    []string{"sh", "-c", "pwd"},
		Workdir: "rel",
	})
	require.NoError(t, err)
	assert.Equal(t, subdir+"\n", stdout)
}

// ---- resolveWorkdir boundary enforcement ----

func TestLocalCommandRunner_ResolveWorkdir_Empty_ReturnsWorkspace(t *testing.T) {
	r := NewLocalCommandRunner("/tmp/ws", nil)
	got, err := r.resolveWorkdir("")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/ws", got)
}

func TestLocalCommandRunner_ResolveWorkdir_DotDotEscape_Rejected(t *testing.T) {
	r := NewLocalCommandRunner("/tmp/ws", nil)
	_, err := r.resolveWorkdir("/tmp/ws/../../etc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside workspace")
}

func TestLocalCommandRunner_ResolveWorkdir_AbsoluteOutside_Rejected(t *testing.T) {
	r := NewLocalCommandRunner("/tmp/ws", nil)
	_, err := r.resolveWorkdir("/etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside workspace")
}

func TestLocalCommandRunner_ResolveWorkdir_AbsoluteInside_Accepted(t *testing.T) {
	r := NewLocalCommandRunner("/tmp/ws", nil)
	got, err := r.resolveWorkdir("/tmp/ws/subdir")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/ws/subdir", got)
}

func TestLocalCommandRunner_ResolveWorkdir_RelativeInside_Accepted(t *testing.T) {
	r := NewLocalCommandRunner("/tmp/ws", nil)
	got, err := r.resolveWorkdir("subdir/deep")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/ws/subdir/deep", got)
}

// ---- SandboxCommandRunner constructor ----

func TestNewSandboxCommandRunner_NilManifest(t *testing.T) {
	rt := &stubSandboxRuntime{}
	_, err := NewSandboxCommandRunner(nil, rt, "/tmp/ws")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest required")
}

func TestNewSandboxCommandRunner_NilRuntime(t *testing.T) {
	m := minimalAgentManifest()
	_, err := NewSandboxCommandRunner(m, nil, "/tmp/ws")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime required")
}

func TestNewSandboxCommandRunner_EmptyWorkspace(t *testing.T) {
	m := minimalAgentManifest()
	rt := &stubSandboxRuntime{}
	_, err := NewSandboxCommandRunner(m, rt, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace required")
}

func TestNewSandboxCommandRunner_Valid(t *testing.T) {
	m := minimalAgentManifest()
	rt := &stubSandboxRuntime{
		config: SandboxConfig{ContainerRuntime: "docker", RunscPath: "/usr/bin/runsc"},
	}
	runner, err := NewSandboxCommandRunner(m, rt, "/tmp/ws")
	require.NoError(t, err)
	require.NotNil(t, runner)
	assert.Equal(t, "/tmp/ws", runner.workspace)
	assert.Equal(t, "test-image:latest", runner.image)
}

func TestNewSandboxCommandRunner_SetsSecurityFieldsFromManifest(t *testing.T) {
	m := minimalAgentManifest()
	m.Spec.Security.RunAsUser = 1000
	m.Spec.Security.ReadOnlyRoot = true
	m.Spec.Security.NoNewPrivileges = true

	rt := &stubSandboxRuntime{}
	runner, err := NewSandboxCommandRunner(m, rt, "/tmp/ws")
	require.NoError(t, err)
	assert.Equal(t, 1000, runner.user)
	assert.True(t, runner.readOnlyRoot)
	assert.True(t, runner.noNewPrivileges)
}

// ---- SandboxCommandRunner.containerWorkdir ----

func TestSandboxCommandRunner_ContainerWorkdir_Empty(t *testing.T) {
	runner := sandboxRunnerForWorkspaceTests(t, "/tmp/ws")
	got, err := runner.containerWorkdir("")
	require.NoError(t, err)
	assert.Equal(t, "/workspace", got)
}

func TestSandboxCommandRunner_ContainerWorkdir_RelativePath(t *testing.T) {
	runner := sandboxRunnerForWorkspaceTests(t, "/tmp/ws")
	got, err := runner.containerWorkdir("sub/dir")
	require.NoError(t, err)
	assert.Equal(t, "/workspace/sub/dir", got)
}

func TestSandboxCommandRunner_ContainerWorkdir_AbsoluteInWorkspace(t *testing.T) {
	runner := sandboxRunnerForWorkspaceTests(t, "/tmp/ws")
	got, err := runner.containerWorkdir("/tmp/ws/project")
	require.NoError(t, err)
	assert.Equal(t, "/workspace/project", got)
}

func TestSandboxCommandRunner_ContainerWorkdir_ExactWorkspace(t *testing.T) {
	runner := sandboxRunnerForWorkspaceTests(t, "/tmp/ws")
	got, err := runner.containerWorkdir("/tmp/ws")
	require.NoError(t, err)
	assert.Equal(t, "/workspace", got)
}

func TestSandboxCommandRunner_ContainerWorkdir_EscapeRejected(t *testing.T) {
	runner := sandboxRunnerForWorkspaceTests(t, "/tmp/ws")
	_, err := runner.containerWorkdir("/tmp/ws/../../etc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside workspace")
}

func TestSandboxCommandRunner_ContainerWorkdir_AbsoluteOutside_Rejected(t *testing.T) {
	runner := sandboxRunnerForWorkspaceTests(t, "/tmp/ws")
	_, err := runner.containerWorkdir("/etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside workspace")
}

// ---- SandboxCommandRunner.Run — early-exit guards (no docker needed) ----

func TestSandboxCommandRunner_Run_NilReceiver(t *testing.T) {
	var r *SandboxCommandRunner
	_, _, err := r.Run(context.Background(), CommandRequest{Args: []string{"echo"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestSandboxCommandRunner_Run_EmptyArgs(t *testing.T) {
	runner := sandboxRunnerForWorkspaceTests(t, "/tmp/ws")
	_, _, err := runner.Run(context.Background(), CommandRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "arguments required")
}

// ---- helpers ----

// stubSandboxRuntime satisfies SandboxRuntime for constructor tests.
type stubSandboxRuntime struct {
	config SandboxConfig
	policy SandboxPolicy
}

func (s *stubSandboxRuntime) Name() string                   { return "stub" }
func (s *stubSandboxRuntime) Verify(_ context.Context) error { return nil }
func (s *stubSandboxRuntime) RunConfig() SandboxConfig       { return s.config }
func (s *stubSandboxRuntime) EnforcePolicy(p SandboxPolicy) error {
	s.policy = p
	return nil
}
func (s *stubSandboxRuntime) Policy() SandboxPolicy { return s.policy }

// minimalAgentManifest returns a manifest with the fields used by SandboxCommandRunner.
func minimalAgentManifest() *manifest.AgentManifest {
	return &manifest.AgentManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata:   manifest.ManifestMetadata{Name: "test-agent"},
		Spec: manifest.ManifestSpec{
			Image:    "test-image:latest",
			Runtime:  "gvisor",
			Security: manifest.SecuritySpec{},
			Defaults: &manifest.ManifestDefaults{
				Permissions: &core.PermissionSet{},
			},
		},
	}
}

// sandboxRunnerForWorkspaceTests builds a SandboxCommandRunner against a fixed workspace.
func sandboxRunnerForWorkspaceTests(t *testing.T, workspace string) *SandboxCommandRunner {
	t.Helper()
	m := minimalAgentManifest()
	rt := &stubSandboxRuntime{}
	runner, err := NewSandboxCommandRunner(m, rt, workspace)
	require.NoError(t, err)
	return runner
}

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
