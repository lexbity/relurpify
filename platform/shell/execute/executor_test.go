package execute

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/assert"
)

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

func TestToolResultLargeStdoutTruncation(t *testing.T) {
	runner := &recordingRunner{stdout: strings.Repeat("x", 1024*1024)} // 1MB
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:           "cli_cat",
		Command:        "cat",
		MaxOutputBytes: 256 * 1024,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{}, "")
	require.NoError(t, err)
	require.True(t, envelope.Truncated, "output must be marked truncated")
	require.Equal(t, int64(256*1024), envelope.TruncatedAt)
	require.Equal(t, int64(256*1024), envelope.StdoutBytes, "StdoutBytes reflects the received (capped) byte count")
	require.LessOrEqual(t, len(envelope.Stdout), 256*1024, "truncated stdout must not exceed limit")
	require.True(t, envelope.Success, "truncation alone must not set Success=false")
}

func TestToolResultStderrTruncation(t *testing.T) {
	runner := &recordingRunner{stderr: strings.Repeat("e", 512*1024)} // 512KB
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:           "cli_fail",
		Command:        "sh",
		MaxOutputBytes: 128 * 1024,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{}, "")
	require.NoError(t, err)
	require.True(t, envelope.Truncated)
	require.Equal(t, int64(128*1024), envelope.TruncatedAt)
	require.Equal(t, int64(128*1024), envelope.StderrBytes, "StderrBytes reflects received (capped) byte count")
	require.LessOrEqual(t, len(envelope.Stderr), 128*1024)
}

func TestToolResultNoTruncationUnderLimit(t *testing.T) {
	runner := &recordingRunner{stdout: "hello world", stderr: "short err"}
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:          "cli_echo",
		Command:       "echo",
		MaxOutputBytes: 256 * 1024,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{}, "")
	require.NoError(t, err)
	require.False(t, envelope.Truncated, "output under limit must not be truncated")
	require.Equal(t, "hello world", envelope.Stdout)
	require.Equal(t, "short err", envelope.Stderr)
	require.Equal(t, int64(len("hello world")), envelope.StdoutBytes)
	require.Equal(t, int64(len("short err")), envelope.StderrBytes)
}

func TestToolResultMaxOutputBytesZeroUsesDefault(t *testing.T) {
	runner := &recordingRunner{stdout: strings.Repeat("x", 300*1024)} // 300KB — exceeds 256KB default
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:    "cli_cat",
		Command: "cat",
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{}, "")
	require.NoError(t, err)
	assert.True(t, envelope.Truncated, "300KB output must be truncated with default 256KB limit")
	assert.Equal(t, defaultMaxOutputBytes, envelope.TruncatedAt)
}

func TestToolResultMaxOutputBytesNegativeIsUnlimited(t *testing.T) {
	runner := &recordingRunner{stdout: strings.Repeat("x", 1024*1024)} // 1MB
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:           "cli_cat",
		Command:        "cat",
		MaxOutputBytes: -1,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{}, "")
	require.NoError(t, err)
	assert.False(t, envelope.Truncated, "negative limit must disable truncation")
	assert.Equal(t, int64(1024*1024), envelope.StdoutBytes)
	assert.Equal(t, 1024*1024, len(envelope.Stdout))
	assert.Equal(t, int64(-1), envelope.TruncatedAt, "TruncatedAt reflects the preset limit")
}

func TestToolResultTruncationPropagatedToToolResult(t *testing.T) {
	runner := &recordingRunner{stdout: "ab"}
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:           "cli_echo",
		Command:        "echo",
		MaxOutputBytes: 1,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{}, "")
	require.NoError(t, err)
	require.True(t, envelope.Truncated)
	require.Equal(t, int64(1), envelope.TruncatedAt)
	require.Equal(t, int64(1), envelope.StdoutBytes)
}

func TestToolResultMixedTruncation(t *testing.T) {
	runner := &recordingRunner{stdout: "small out", stderr: strings.Repeat("e", 1024*1024)} // 1MB stderr
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:           "cli_mixed",
		Command:        "sh",
		MaxOutputBytes: 256 * 1024,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{}, "")
	require.NoError(t, err)
	require.True(t, envelope.Truncated, "truncated must be true when stderr over limit")
	require.Equal(t, "small out", envelope.Stdout, "stdout under limit must be preserved")
	require.Equal(t, int64(len("small out")), envelope.StdoutBytes)
	require.LessOrEqual(t, len(envelope.Stderr), 256*1024, "truncated stderr must not exceed limit")
}

func TestFlagInjectionBlockedByDefault(t *testing.T) {
	runner := &recordingRunner{}
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:    "cli_tool",
		Command: "somebinary",
	}, runner)

	_, err := exec.Execute(context.Background(), "", []interface{}{"--config=/etc/passwd"}, "")
	if err == nil {
		t.Fatal("expected flag injection error, got nil")
	}
	if !strings.Contains(err.Error(), "flag injection") {
		t.Fatalf("expected error to contain 'flag injection', got: %v", err)
	}
	if len(runner.requests) > 0 {
		t.Fatal("runner should not be called when flag injection is detected")
	}
}

func TestSingleDashArgBlockedByDefault(t *testing.T) {
	runner := &recordingRunner{}
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:    "cli_tool",
		Command: "somebinary",
	}, runner)

	_, err := exec.Execute(context.Background(), "", []interface{}{"-n", "10"}, "")
	if err == nil {
		t.Fatal("expected flag injection error, got nil")
	}
}

func TestFlagInjectionAllowedWhenOptedIn(t *testing.T) {
	runner := &recordingRunner{stdout: "ok"}
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:       "cli_tool",
		Command:    "somebinary",
		AllowFlags: true,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{"--verbose"}, "")
	if err != nil {
		t.Fatalf("expected no error when AllowFlags=true, got: %v", err)
	}
	if !envelope.Success {
		t.Fatal("expected success")
	}
	require.Len(t, runner.requests, 1)
	require.Equal(t, "somebinary", runner.requests[0].Args[0])
	require.Equal(t, "--verbose", runner.requests[0].Args[1])
}

func TestNonFlagArgsAlwaysAllowed(t *testing.T) {
	runner := &recordingRunner{stdout: "ok"}
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:    "cli_tool",
		Command: "cp",
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{"src/main.go", "/tmp/dest"}, "")
	if err != nil {
		t.Fatalf("expected no error for non-flag args, got: %v", err)
	}
	if !envelope.Success {
		t.Fatal("expected success")
	}
	require.Len(t, runner.requests, 1)
	require.Equal(t, "cp", runner.requests[0].Args[0])
	require.Equal(t, "src/main.go", runner.requests[0].Args[1])
	require.Equal(t, "/tmp/dest", runner.requests[0].Args[2])
}

func TestDoubleDashTerminatorAllowedWhenOptedIn(t *testing.T) {
	runner := &recordingRunner{stdout: "ok"}
	exec := NewExecutor(t.TempDir(), CommandPreset{
		Name:       "cli_tool",
		Command:    "grep",
		AllowFlags: true,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "", []interface{}{"--", "-pattern"}, "")
	if err != nil {
		t.Fatalf("expected no error when AllowFlags=true with double-dash, got: %v", err)
	}
	if !envelope.Success {
		t.Fatal("expected success")
	}
	require.Len(t, runner.requests, 1)
	require.Equal(t, "grep", runner.requests[0].Args[0])
	require.Equal(t, "--", runner.requests[0].Args[1])
	require.Equal(t, "-pattern", runner.requests[0].Args[2])
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
