package tooltest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// stubRunner implements ports.CommandRunner with canned output.
type stubRunner struct {
	stdout   string
	stderr   string
	exitCode int
	requests []ports.CommandRequest
}

func (r *stubRunner) Run(_ context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	r.requests = append(r.requests, req)
	return &ports.CommandResult{
		Stdout:      r.stdout,
		Stderr:      r.stderr,
		ExitCode:    r.exitCode,
		StdoutBytes: int64(len(r.stdout)),
		StderrBytes: int64(len(r.stderr)),
	}, nil
}

// RunToolTest executes a single tooltest case and reports failures.
func RunToolTest(t *testing.T, workspace string, tc *ToolTestCase) {
	t.Helper()

	t.Logf("tooltest: %s (tool=%s)", tc.Path, tc.Tool)

	// Load the tool manifest.
	manifestDir := config.DefaultToolManifestDir(workspace)
	manifests, err := config.LoadToolManifests(manifestDir)
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	var manifest *toolcapabilities.ToolManifest
	for _, m := range manifests {
		if toolcapabilities.NormalizeToolName(m.Name) == toolcapabilities.NormalizeToolName(tc.Tool) {
			manifest = m
			break
		}
	}
	if manifest == nil {
		t.Fatalf("tool %q not found in manifests", tc.Tool)
	}

	// Build a stub runner with canned output.
	runner := &stubRunner{
		stdout:   tc.Stdout,
		stderr:   tc.Stderr,
		exitCode: tc.ExitCode,
	}

	// Create a subprocess tool and execute it.
	tool := subprocess.NewTool(*manifest, runner)
	result, err := tool.Execute(context.Background(), tc.Args)

	// Assertions.
	assertResult(t, result, err, tc.Expect, runner)
}

func assertResult(t *testing.T, result *ports.ToolResult, err error, expect ToolTestExpect, runner *stubRunner) {
	t.Helper()

	// Check Go error.
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
	if result == nil {
		t.Error("nil result")
		return
	}

	// Success
	expectSuccess := true
	if expect.Success != nil {
		expectSuccess = *expect.Success
	}
	if result.Success != expectSuccess {
		t.Errorf("Success = %v, want %v", result.Success, expectSuccess)
	}

	// Exit code
	exitCode := 0
	if ec, ok := result.Data["exit_code"]; ok {
		switch v := ec.(type) {
		case int:
			exitCode = v
		case int64:
			exitCode = int(v)
		case float64:
			exitCode = int(v)
		}
	}
	wantExitCode := expect.ExitCode
	if wantExitCode != 0 || exitCode != 0 {
		if exitCode != wantExitCode {
			t.Errorf("exit_code = %d, want %d", exitCode, wantExitCode)
		}
	}

	// Stdout
	stdout := stringField(result.Data, "stdout")
	for _, substr := range expect.StdoutContains {
		if !strings.Contains(stdout, substr) {
			t.Errorf("stdout does not contain %q\nstdout:\n%s", substr, stdout)
		}
	}

	// Stdout JSON
	if expect.StdoutJSON != nil {
		var parsed any
		if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
			t.Errorf("stdout_json expected but stdout is not valid JSON: %v\nstdout:\n%s", err, stdout)
		} else if expect.StdoutJSON.Type != "" {
			actualType := jsonType(parsed)
			if actualType != expect.StdoutJSON.Type {
				t.Errorf("stdout JSON type = %q, want %q", actualType, expect.StdoutJSON.Type)
			}
		}
	}

	// Stderr exact match
	if expect.Stderr != "" {
		stderr := stringField(result.Data, "stderr")
		if stderr != expect.Stderr {
			t.Errorf("stderr = %q, want %q", stderr, expect.Stderr)
		}
	}

	// Error contains
	if len(expect.ErrorContains) > 0 {
		errMsg := result.Error
		for _, substr := range expect.ErrorContains {
			if !strings.Contains(errMsg, substr) {
				t.Errorf("error does not contain %q\nerror: %s", substr, errMsg)
			}
		}
	}

	// Artifact refs
	if expect.HasStdoutRef {
		if ref := result.Data["stdout_ref"]; ref == nil || fmt.Sprint(ref) == "" {
			t.Error("expected stdout_ref to be non-empty")
		}
	}
	if expect.HasStderrRef {
		if ref := result.Data["stderr_ref"]; ref == nil || fmt.Sprint(ref) == "" {
			t.Error("expected stderr_ref to be non-empty")
		}
	}
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func jsonType(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}
