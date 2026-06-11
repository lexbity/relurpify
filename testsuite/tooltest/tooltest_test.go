package tooltest

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestLoadToolTests(t *testing.T) {
	dir := t.TempDir()
	requireWriteFile(t, filepath.Join(dir, "test_jq.tooltest.yaml"), `
tool: cli_jq
args:
  args: ["."]
stdout: '{"key":"value"}'
expect:
  exit_code: 0
  stdout_contains:
    - "key"
`)

	tcs, err := LoadToolTests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tcs) != 1 {
		t.Fatalf("expected 1 test case, got %d", len(tcs))
	}
	if tcs[0].Tool != "cli_jq" {
		t.Errorf("tool = %q, want cli_jq", tcs[0].Tool)
	}
}

func TestLoadToolTestsSkipsNonTooltestFiles(t *testing.T) {
	dir := t.TempDir()
	requireWriteFile(t, filepath.Join(dir, "not_a_test.yaml"), "foo: bar")
	tcs, err := LoadToolTests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tcs) != 0 {
		t.Errorf("expected 0 test cases, got %d", len(tcs))
	}
}

func TestLoadToolTestMissingDir(t *testing.T) {
	tcs, err := LoadToolTests("/nonexistent/path")
	if err != nil {
		t.Fatal(err)
	}
	if len(tcs) != 0 {
		t.Errorf("expected 0 test cases, got %d", len(tcs))
	}
}

func TestRunToolTestSuccess(t *testing.T) {
	workspace := workspaceRoot(t)
	tc := &ToolTestCase{
		Path:   "inline_test",
		Tool:   "cli_echo",
		Stdout: "hello",
		Args:   map[string]any{"args": []any{"hello"}},
		Expect: ToolTestExpect{
			StdoutContains: []string{"hello"},
		},
	}
	RunToolTest(t, workspace, tc)
}

func TestRunToolTestJSONOutput(t *testing.T) {
	workspace := workspaceRoot(t)
	tc := &ToolTestCase{
		Path:   "inline_json_test",
		Tool:   "cli_jq",
		Stdout: `{"key":"value"}`,
		Expect: ToolTestExpect{
			StdoutJSON: &ToolTestJSONExpect{Type: "object"},
		},
	}
	RunToolTest(t, workspace, tc)
}

func TestRunToolTestErrorMapping(t *testing.T) {
	workspace := workspaceRoot(t)
	tc := &ToolTestCase{
		Path:     "inline_error_test",
		Tool:     "cli_jq",
		Stderr:   "parse error",
		ExitCode: 1,
		Expect: ToolTestExpect{
			Success:       boolPtr(false),
			ExitCode:      1,
			ErrorContains: []string{"parse error"},
		},
	}
	RunToolTest(t, workspace, tc)
}

func TestRunToolTestFlagInjection(t *testing.T) {
	workspace := workspaceRoot(t)
	// cli_sed has allow_flags: true, so flags should work
	tc := &ToolTestCase{
		Path:   "inline_flag_test",
		Tool:   "cli_sed",
		Stdout: "transformed",
		Args:   map[string]any{"args": []any{"s/foo/bar/", "--flags-allowed"}},
		Expect: ToolTestExpect{
			StdoutContains: []string{"transformed"},
		},
	}
	RunToolTest(t, workspace, tc)
}

func TestRunToolTestNetworkEgressBlocked(t *testing.T) {
	workspace := workspaceRoot(t)
	tc := &ToolTestCase{
		Path:   "inline_egress_test",
		Tool:   "cli_curl",
		Stdout: "should not be reached",
		Args:   map[string]any{"args": []any{"http://127.0.0.1:8080/"}},
		Expect: ToolTestExpect{
			Success:       boolPtr(false),
			ErrorContains: []string{"denied"},
		},
	}
	RunToolTest(t, workspace, tc)
}

func requireWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(path, []byte(content)); err != nil {
		t.Fatal(err)
	}
}

func workspaceRoot(t *testing.T) string {
	t.Helper()
	// Walk up to find the workspace root (where go.mod is)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("workspace root not found")
		}
		dir = parent
	}
}

func boolPtr(b bool) *bool { return &b }
