package execute

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

type recordingRunner struct {
	requests []sandbox.CommandRequest
	stdout   string
	stderr   string
	err      error
}

func (r *recordingRunner) Run(_ context.Context, req sandbox.CommandRequest) (string, string, error) {
	r.requests = append(r.requests, req)
	return r.stdout, r.stderr, r.err
}

func TestExecutorPreservesStdoutStderrAndMetadata(t *testing.T) {
	runner := &recordingRunner{stdout: "out", stderr: "err"}
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:       "cli_echo",
		Command:    "echo",
		Timeout:    5 * time.Second,
		AllowStdin: true,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{"hello"}, "stdin")
	require.NoError(t, err)
	require.True(t, envelope.Success)
	require.Equal(t, "out", envelope.Stdout)
	require.Equal(t, "err", envelope.Stderr)
	require.Equal(t, []string{"echo", "hello"}, envelope.Command)
	require.Equal(t, "cli_echo", envelope.Preset)
	require.Equal(t, "stdin", runner.requests[0].Input)
	require.Equal(t, 5*time.Second, runner.requests[0].Timeout)
	require.Equal(t, exec.BasePath, runner.requests[0].Workdir)
	require.Equal(t, "echo", envelope.Metadata["command"])
	require.Equal(t, []string{"hello"}, envelope.Metadata["args"])
}

func TestExecutorIgnoresStdinWhenDisabled(t *testing.T) {
	runner := &recordingRunner{}
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:       "cli_echo",
		Command:    "echo",
		AllowStdin: false,
	}, runner)

	_, err := exec.Execute(context.Background(), "", []interface{}{"hello"}, "stdin")
	require.NoError(t, err)
	require.Len(t, runner.requests, 1)
	require.Empty(t, runner.requests[0].Input)
}

func TestExecutorAppliesCargoHelpers(t *testing.T) {
	base := t.TempDir()
	crateDir := filepath.Join(base, "nested")
	require.NoError(t, os.MkdirAll(crateDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(crateDir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0o644))

	runner := &recordingRunner{}
	exec := NewExecutor(base, CommandPreset{
		Name:    "cli_cargo",
		Command: "cargo",
	}, runner)

	envelope, err := exec.Execute(context.Background(), "nested", []interface{}{"test"}, "")
	require.NoError(t, err)
	require.True(t, envelope.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{"cargo", "test", "--manifest-path", filepath.Join(crateDir, "Cargo.toml")}, runner.requests[0].Args)
	require.Equal(t, crateDir, runner.requests[0].Workdir)
}

func TestExecutorIsolatesNestedCargoRuns(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(base, "Cargo.toml"), []byte("[package]\nname = \"root\"\nversion = \"0.1.0\"\n"), 0o644))
	crateDir := filepath.Join(base, "nested")
	require.NoError(t, os.MkdirAll(filepath.Join(crateDir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(crateDir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(crateDir, "src", "lib.rs"), []byte("pub fn add(a:i32,b:i32)->i32{a+b}\n"), 0o644))

	runner := &recordingRunner{}
	exec := NewExecutor(base, CommandPreset{
		Name:    "cli_cargo",
		Command: "cargo",
	}, runner)

	envelope, err := exec.Execute(context.Background(), "nested", []interface{}{"test"}, "")
	require.NoError(t, err)
	require.True(t, envelope.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, base, runner.requests[0].Workdir)
	require.Equal(t, "test", runner.requests[0].Args[1])
	require.Equal(t, "--manifest-path", runner.requests[0].Args[2])
	require.NotContains(t, runner.requests[0].Args[3], base)
}
