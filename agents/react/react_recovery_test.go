package react

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

func TestReactRecoveryProbeArgsAndInferenceHelpers(t *testing.T) {
	t.Run("file read path and directory", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "fail.go")
		if err := os.WriteFile(filePath, []byte("package main"), 0o644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}

		agent := &ReActAgent{
			Tools: capabilityRegistryWithTools(t, stubTool{name: "file_read", params: []core.ToolParameter{{Name: "path", Required: true}}}),
		}
		state := core.NewContext()
		state.Set("react.failure_workdir", dir)
		state.Set("react.failure_path", filePath)
		args := recoveryProbeArgs(agent, "file_read", state, &core.Task{}, map[string]interface{}{})
		if got := args["path"]; got != filePath {
			t.Fatalf("unexpected file_read probe args: %#v", args)
		}
		if got := primaryFailureDirectory(state, map[string]interface{}{}); got != dir {
			t.Fatalf("unexpected primary failure directory: %q", got)
		}
	})

	t.Run("search probe args", func(t *testing.T) {
		agent := &ReActAgent{
			Tools: capabilityRegistryWithTools(t, stubTool{name: "search_grep", params: []core.ToolParameter{{Name: "directory", Required: true}, {Name: "pattern", Required: true}}}),
		}
		state := core.NewContext()
		state.Set("react.failure_workdir", "/tmp/project")
		args := recoveryProbeArgs(agent, "search_grep", state, &core.Task{}, map[string]interface{}{"error": "cargo test failed"})
		if got := args["directory"]; got != "/tmp/project" {
			t.Fatalf("unexpected search probe directory: %#v", args)
		}
		if got := args["pattern"]; got != "map[error:cargo test failed]" {
			t.Fatalf("unexpected search probe pattern: %#v", args)
		}
	})

	t.Run("query ast and sqlite probes", func(t *testing.T) {
		agent := &ReActAgent{
			Tools: capabilityRegistryWithTools(
				t,
				stubTool{name: "query_ast", params: []core.ToolParameter{{Name: "action", Required: true}, {Name: "symbol", Required: false}}},
				stubTool{name: "sqlite_query", params: []core.ToolParameter{{Name: "database_path", Required: true}, {Name: "query", Required: true}}},
			),
		}
		state := core.NewContext()
		state.Set("react.tool_observations", []ToolObservation{
			{Tool: "file_read", Args: map[string]interface{}{"path": "data/app.sqlite3"}, Data: map[string]interface{}{"database_path": "data/app.sqlite3"}, Success: true},
		})
		astArgs := recoveryProbeArgs(agent, "query_ast", state, &core.Task{}, map[string]interface{}{"panic": "missing symbol Foo::bar"})
		if astArgs["action"] != "get_signature" || astArgs["symbol"] != "map" {
			t.Fatalf("unexpected query_ast args: %#v", astArgs)
		}
		sqliteArgs := recoveryProbeArgs(agent, "sqlite_query", state, &core.Task{}, map[string]interface{}{})
		if sqliteArgs["database_path"] != "data/app.sqlite3" {
			t.Fatalf("unexpected sqlite database arg: %#v", sqliteArgs)
		}
		if got := sqliteArgs["query"]; !strings.Contains(fmt.Sprint(got), "sqlite_master") {
			t.Fatalf("expected sqlite query string, got %#v", got)
		}

		if got := primaryFailureSearchPattern(map[string]interface{}{"error": "first line\nsecond line"}); got != "map[error:first line" {
			t.Fatalf("unexpected search pattern: %q", got)
		}
		if got := inferFailureSymbol(map[string]interface{}{"error": "panic at Foo::bar in module"}); got != "map" {
			t.Fatalf("unexpected inferred symbol: %q", got)
		}
		if got := inferredPathFromObservations(state, "database_path"); got != "data/app.sqlite3" {
			t.Fatalf("unexpected inferred path from observations: %q", got)
		}
		if got := inferredSQLiteDatabase(state); got != "data/app.sqlite3" {
			t.Fatalf("unexpected inferred sqlite db: %q", got)
		}
		if got := isSQLiteFailurePath("db.sqlite3"); !got {
			t.Fatal("expected sqlite suffix to be recognized")
		}
	})

	t.Run("manifest inference", func(t *testing.T) {
		state := core.NewContext()
		state.Set("react.tool_observations", []ToolObservation{
			{Tool: "rust_workspace_detect", Data: map[string]interface{}{"manifest_path": "Cargo.toml"}},
			{Tool: "python_workspace_detect", Data: map[string]interface{}{"manifest_path": "pyproject.toml"}},
			{Tool: "node_project_metadata", Data: map[string]interface{}{"manifest_path": "package.json"}},
			{Tool: "go_workspace_detect", Data: map[string]interface{}{"module_path": "go.work"}},
		})
		if got := inferredCargoManifest(state); got != "Cargo.toml" {
			t.Fatalf("unexpected cargo manifest: %q", got)
		}
		if got := inferredPythonManifest(state); got != "pyproject.toml" {
			t.Fatalf("unexpected python manifest: %q", got)
		}
		if got := inferredNodeManifest(state); got != "package.json" {
			t.Fatalf("unexpected node manifest: %q", got)
		}
		if got := inferredGoManifest(state); got != "go.work" {
			t.Fatalf("unexpected go manifest: %q", got)
		}
		if got := inferredManifestFromObservations(state, manifestInferenceRule{
			tools:      []string{"rust_workspace_detect"},
			dataKeys:   []string{"manifest_path"},
			pathSuffix: []string{"Cargo.toml"},
		}); got != "Cargo.toml" {
			t.Fatalf("unexpected generic manifest inference: %q", got)
		}
	})
}

func TestReactObserveFailureAndLoopHelpers(t *testing.T) {
	if !valueIndicatesFailure(map[string]interface{}{"success": false}) {
		t.Fatal("expected explicit success=false to indicate failure")
	}
	if !valueIndicatesFailure(map[string]interface{}{"error": "assertion failed"}) {
		t.Fatal("expected error text to indicate failure")
	}
	if !hasFailureFromState(coreContextWithLastResult(map[string]interface{}{"error": "panic occurred"})) {
		t.Fatal("expected failure from state")
	}
	if got := iterationExhaustionReason(&core.Task{Instruction: "fix the bug"}, core.NewContext()); got != "iteration budget exhausted before making any file changes" {
		t.Fatalf("unexpected iteration exhaustion reason: %q", got)
	}
	if got := iterationExhaustionReason(&core.Task{Instruction: "summarize the repo"}, core.NewContext()); got != "iteration budget exhausted before task completion" {
		t.Fatalf("unexpected analysis iteration exhaustion reason: %q", got)
	}

	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_read", Args: map[string]interface{}{"path": "a.go"}, Success: true},
		{Tool: "file_read", Args: map[string]interface{}{"path": "a.go"}, Success: true},
		{Tool: "file_read", Args: map[string]interface{}{"path": "a.go"}, Success: true},
	})
	task := &core.Task{Instruction: "Fix the bug in a.go"}
	if got := repeatedReadTarget(state); got != "a.go" {
		t.Fatalf("unexpected repeated read target: %q", got)
	}
	if repeated, reason := detectRepeatedToolLoop(state, task); !repeated || !strings.Contains(reason, "file_read") {
		t.Fatalf("expected repeated file_read loop, got %v %q", repeated, reason)
	}

	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "cli_cargo", Success: true, Data: map[string]interface{}{"stderr": "error: failed\n"}},
	})
	if summary, ok := repeatedFailureAnalysis(task, state, map[string]interface{}{"error": "failed"}); ok || summary != "" {
		t.Fatalf("expected repeatedFailureAnalysis to ignore editing tasks, got %q %v", summary, ok)
	}
	analysisTask := &core.Task{Instruction: "Inspect the build failure"}
	if summary, ok := repeatedFailureAnalysis(analysisTask, state, map[string]interface{}{"error": "failed"}); !ok || !strings.Contains(summary, "cli_cargo failed repeatedly") {
		t.Fatalf("unexpected repeated failure analysis: %q %v", summary, ok)
	}
	if summary, ok := analysisSummaryFromFailure(analysisTask, state, map[string]interface{}{"error": "failed"}); !ok || !strings.Contains(summary, "cli_cargo failed") {
		t.Fatalf("unexpected analysis summary: %q %v", summary, ok)
	}
}

func TestReactRecoveryProbeTrackingAndClassification(t *testing.T) {
	state := core.NewContext()
	recordRecoveryProbeUsage(state, "sig-1", "search_grep")
	recordRecoveryProbeUsage(state, "sig-1", "search_grep")
	used := recoveryProbesForSignature(state, "sig-1")
	if !used["search_grep"] {
		t.Fatal("expected recorded recovery probe to be returned")
	}
	if got := failureSignature(map[string]interface{}{"error": "boom"}); got == "" {
		t.Fatal("expected failure signature")
	}

	task := &core.Task{Instruction: "Fix the bug and run tests"}
	if !taskNeedsEditing(task) {
		t.Fatal("expected editing task")
	}
	if !hasFailure(map[string]interface{}{"error": "panic"}) {
		t.Fatal("expected failure helper")
	}
}

func capabilityRegistryWithTools(t *testing.T, tools ...stubTool) *capability.Registry {
	t.Helper()
	reg := capability.NewRegistry()
	for _, tool := range tools {
		if err := reg.Register(tool); err != nil {
			t.Fatalf("register tool %s: %v", tool.name, err)
		}
	}
	return reg
}

func coreContextWithLastResult(value map[string]interface{}) *core.Context {
	state := core.NewContext()
	state.Set("react.last_tool_result", value)
	return state
}
