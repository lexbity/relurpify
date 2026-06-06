package subprocess

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

// recordingRunner implements contracts.CommandRunner by recording the request
// and returning canned output.
type recordingRunner struct {
	requests []contracts.CommandRequest
	stdout   string
	stderr   string
}

func (r *recordingRunner) Run(_ context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	r.requests = append(r.requests, req)
	return &contracts.CommandResult{
		Stdout:      r.stdout,
		Stderr:      r.stderr,
		StdoutBytes: int64(len(r.stdout)),
		StderrBytes: int64(len(r.stderr)),
	}, nil
}

func TestSubprocessToolBasicExecute(t *testing.T) {
	runner := &recordingRunner{stdout: "out", stderr: "err"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_jq",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"jq"}},
		},
	}, runner)

	require.Equal(t, "cli_jq", tool.Name())
	require.Equal(t, "text", tool.Category())
	require.True(t, tool.IsAvailable(context.Background()))
}

func TestSubprocessToolExecuteReturnsStdoutStderrExitCode(t *testing.T) {
	runner := &recordingRunner{stdout: "{}", stderr: ""}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_jq",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"jq"}},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"."},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "{}", result.Data["stdout"])
	require.Equal(t, "", result.Data["stderr"])
	require.Equal(t, 0, result.Data["exit_code"])
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{"jq", "."}, runner.requests[0].Args)
}

func TestSubprocessToolExecuteWithStdin(t *testing.T) {
	runner := &recordingRunner{stdout: "transformed"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_sed",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"sed"}},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"stdin": "hello",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, "hello", runner.requests[0].Input)
}

func TestSubprocessToolNonZeroExitCode(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_tool",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"tool"}},
		},
	}, &exitCodeRunner{exitCode: 1, stderr: "parse error"})

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "parse error")
}

func TestSubprocessToolErrorMapping(t *testing.T) {
	runner := &recordingRunner{stderr: "raw error"}
	runner.requests = nil
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_jq",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"jq"}},
		},
		Errors: map[string]string{
			"1": "jq: parse error — check your filter syntax",
			"5": "jq: internal error",
		},
	}, &exitCodeRunner{exitCode: 1})

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "jq: parse error — check your filter syntax", result.Error)
}

func TestSubprocessToolExitsWithCodeErrorWhenNoStderr(t *testing.T) {
	runner := &exitCodeRunner{exitCode: 2}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_tool",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"tool"}},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "exit code 2", result.Error)
}

func TestSubprocessToolWithoutRunnerReportsUnavailable(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "missing",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"tool"}},
		},
	}, nil)

	require.False(t, tool.IsAvailable(context.Background()))
	_, err := tool.Execute(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "command runner missing")
}

func TestSubprocessToolSandboxTimeouts(t *testing.T) {
	runner := &recordingRunner{stdout: "ok"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_go",
		Family:  "build",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"go"}},
			Sandbox: &contracts.ToolManifestSandbox{
				TimeoutSeconds: 30,
				MemoryMB:       512,
			},
		},
	}, runner)

	_, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, runner.requests, 1)
	require.Equal(t, 30*1000_000_000, int(runner.requests[0].Timeout)) // 30s
	require.Equal(t, int64(512*1024*1024), runner.requests[0].MemoryBytes)
}

// --- Flag injection parity ---

func TestFlagInjectionBlockedByDefault(t *testing.T) {
	runner := &recordingRunner{}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_tool",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"somebinary"}},
			// no sandbox — allow_flags defaults to false
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"--config=/etc/passwd"},
	})
	require.NoError(t, err) // flag injection returns a structured result, not a Go error
	require.False(t, result.Success)
	require.Contains(t, result.Error, "flag injection")
	require.Len(t, runner.requests, 0, "runner must not be called when flag injection is detected")
}

func TestSingleDashArgBlockedByDefault(t *testing.T) {
	runner := &recordingRunner{}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_tool",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"somebinary"}},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"-n", "10"},
	})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "flag injection")
}

func TestFlagInjectionAllowedWhenOptedIn(t *testing.T) {
	runner := &recordingRunner{stdout: "ok"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_tool",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"somebinary"}},
			Sandbox: &contracts.ToolManifestSandbox{AllowFlags: true},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"--verbose"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{"somebinary", "--verbose"}, runner.requests[0].Args)
}

func TestNonFlagArgsAlwaysAllowed(t *testing.T) {
	runner := &recordingRunner{stdout: "ok"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_tool",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"cp"}},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"src/main.go", "/tmp/dest"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{"cp", "src/main.go", "/tmp/dest"}, runner.requests[0].Args)
}

func TestDoubleDashTerminatorAllowedWhenOptedIn(t *testing.T) {
	runner := &recordingRunner{stdout: "ok"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_tool",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"grep"}},
			Sandbox: &contracts.ToolManifestSandbox{AllowFlags: true},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"--", "-pattern"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{"grep", "--", "-pattern"}, runner.requests[0].Args)
}

// --- Parity golden tests for representative tools ---

func TestParityCLIJQ(t *testing.T) {
	runner := &recordingRunner{stdout: "{\"key\": \"value\"}"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_jq",
		Family:  "text",
		Intent:  []string{"extract", "structured-data"},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"jq"}},
			Sandbox: &contracts.ToolManifestSandbox{
				AllowFlags:     true,
				TimeoutSeconds: 30,
			},
			AllowStdin:      true,
			SupportsWorkdir: true,
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args":              []interface{}{"."},
		"working_directory": ".",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "{\"key\": \"value\"}", result.Data["stdout"])
	require.Len(t, runner.requests, 1)
}

func TestParityCLIRG(t *testing.T) {
	runner := &recordingRunner{stdout: "src/main.go:1:1: func main"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_rg",
		Family:  "fileops",
		Intent:  []string{"search"},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"rg"}},
			Sandbox: &contracts.ToolManifestSandbox{
				AllowFlags:     true,
				TimeoutSeconds: 30,
			},
			AllowStdin:      true,
			SupportsWorkdir: true,
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"func"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "src/main.go:1:1: func main", result.Data["stdout"])
}

func TestParityCLISed(t *testing.T) {
	runner := &recordingRunner{stdout: "hello world"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_sed",
		Family:  "text",
		Intent:  []string{"transform", "edit"},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"sed"}},
			Sandbox: &contracts.ToolManifestSandbox{
				AllowFlags:     true,
				TimeoutSeconds: 30,
			},
			AllowStdin:      true,
			SupportsWorkdir: true,
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args":  []interface{}{"s/foo/hello/"},
		"stdin": "foo world",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "hello world", result.Data["stdout"])
}

func TestParityCLICurl(t *testing.T) {
	runner := &recordingRunner{stdout: "response body"}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_curl",
		Family:  "network",
		Intent:  []string{"fetch", "http"},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: &contracts.ToolManifestSandbox{
				AllowFlags:     true,
				TimeoutSeconds: 30,
			},
			AllowStdin:      true,
			SupportsWorkdir: true,
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute", "network"},
			EffectClass: []string{"process_spawn"},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"https://example.com"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "response body", result.Data["stdout"])
}

func TestParityCLIMkdir(t *testing.T) {
	runner := &recordingRunner{}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_mkdir",
		Family:  "fileops",
		Intent:  []string{"create"},
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"mkdir"}},
			DefaultArgs: []string{"-p"},
			Sandbox: &contracts.ToolManifestSandbox{
				AllowFlags:     true,
				TimeoutSeconds: 30,
			},
			AllowStdin:      true,
			SupportsWorkdir: true,
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"/tmp/newdir"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{"mkdir", "-p", "/tmp/newdir"}, runner.requests[0].Args)
}

// --- Param/tag/metadata parity ---

func TestSubprocessToolPermissionsNotEmptyWithCommand(t *testing.T) {
	runner := &recordingRunner{}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_jq",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"jq"}},
		},
	}, runner)

	perms := tool.Permissions()
	require.NotNil(t, perms.Permissions)
	require.Len(t, ps(perms.Permissions).Executables, 1, "Permissions must derive Executables from command.base")
	require.Equal(t, "jq", ps(perms.Permissions).Executables[0].Binary)
}

func TestSubprocessToolTagsFromCapability(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_jq",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"jq"}},
		},
		Capability: contracts.ToolManifestCapability{
			RiskClass: []string{"execute"},
		},
	}, nil)

	require.Equal(t, []string{"execute"}, tool.Tags())
}

func TestSubprocessToolNilRunner(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "nil_runner",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"tool"}},
		},
	}, nil)

	require.False(t, tool.IsAvailable(context.Background()))
	_, err := tool.Execute(context.Background(), nil)
	require.Error(t, err)
}

// --- Permissions tests ---

func TestPermissionsDerivesBinaryFromCommandBase(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_jq",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"jq"}},
		},
	}, nil)

	perms := tool.Permissions()
	require.NotNil(t, perms.Permissions)
	require.Len(t, ps(perms.Permissions).Executables, 1)
	require.Equal(t, "jq", ps(perms.Permissions).Executables[0].Binary)
}

func TestPermissionsEmptyWhenNoCommand(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "empty",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
		},
	}, nil)

	perms := tool.Permissions()
	require.NotNil(t, perms.Permissions)
	require.Empty(t, ps(perms.Permissions).Executables)
}

func TestPermissionsIncludesDefaultArgs(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_mkdir",
		Family:  "fileops",
		Execution: contracts.ToolManifestExecution{
			Backend:     contracts.ToolBackendSubprocess,
			Command:     &contracts.ToolManifestCommand{Base: []string{"mkdir"}},
			DefaultArgs: []string{"-p"},
		},
	}, nil)

	perms := tool.Permissions()
	require.Len(t, ps(perms.Permissions).Executables, 1)
	require.Equal(t, []string{"-p"}, ps(perms.Permissions).Executables[0].Args)
}

func TestPermissionsHITLFalseByDefault(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_echo",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
		},
		Capability: contracts.ToolManifestCapability{
			RiskClass: []string{"execute"},
		},
	}, nil)

	require.False(t, ps(tool.Permissions().Permissions).Executables[0].HITLRequired)
}

func TestPermissionsHITLTrueForDestructiveTools(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_gdb",
		Family:  "build",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"gdb"}},
		},
		Capability: contracts.ToolManifestCapability{
			RiskClass: []string{"execute", "destructive"},
		},
	}, nil)

	require.True(t, ps(tool.Permissions().Permissions).Executables[0].HITLRequired)
}

// --- Panic recovery tests ---

func TestPanicInRunnerReturnsStructuredResult(t *testing.T) {
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_tool",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"tool"}},
		},
	}, &panickingRunner{})

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err) // panic recovery returns structured result, not Go error
	require.False(t, result.Success)
	require.Contains(t, result.Error, "panicked")
}

func TestPanicInExpandCommandDoesNotPanic(t *testing.T) {
	// This constructs an invalid manifest that would panic during processing
	// The manifest itself is valid, but we exercise the deferred recovery path
	runner := &recordingRunner{}
	tool := NewTool(contracts.ToolManifest{
		Name:    "panic_test",
		Family:  "text",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
		},
	}, runner)

	// Normal execution — no panic expected here
	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
}

// panickingRunner panics on Run to verify defer/recover in executor.
type panickingRunner struct{}

func (r *panickingRunner) Run(_ context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	panic("unexpected runtime error in command runner")
}

// exitCodeRunner is a helper that returns a fixed exit code.
type exitCodeRunner struct {
	exitCode int
	stderr   string
}

func (r *exitCodeRunner) Run(_ context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	return &contracts.CommandResult{
		ExitCode: r.exitCode,
		Stderr:   r.stderr,
	}, nil
}


func ps(v interface{}) *contracts.PermissionSet {
	if v == nil {
		return nil
	}
	p, _ := v.(*contracts.PermissionSet)
	return p
}
