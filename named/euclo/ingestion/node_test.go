package ingestion

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestIngestionNodeFilesOnlyUsesFrameworkPipeline(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "note.txt", "first line\nsecond line\nthird line\n")

	node := NewIngestionNode("ingest-files", IngestionSpec{
		Mode:          IngestionModeFilesOnly,
		WorkspaceRoot: root,
	})

	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetTaskEnvelope(env, &intake.TaskEnvelope{
		TaskID:        "task-1",
		SessionID:     "session-1",
		ExplicitFiles: []string{"note.txt"},
	})

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if got, ok := core.ResultField(result.Data, "mode"); !ok || got != IngestionModeFilesOnly {
		t.Fatalf("unexpected mode: %v", got)
	}
	if got, ok := core.ResultField(result.Data, "user_files_ingested"); !ok || got != 0 {
		t.Fatalf("expected user file count 0, got %v", got)
	}
	if got, ok := core.ResultField(result.Data, "session_pins_ingested"); !ok || got != 0 {
		t.Fatalf("expected session pin count 0, got %v", got)
	}
	if got, ok := core.ResultField(result.Data, "chunks_created"); !ok || got == 0 {
		t.Fatal("expected chunks to be created")
	}
	if _, ok := contextdata.GetTyped[*IngestionResult](env, "euclo.ingestion_result"); !ok {
		t.Fatal("expected ingestion result in envelope")
	}
	if _, ok := contextdata.GetTyped[map[string]any](env, "euclo.ingested.file."+sanitize("note.txt")); !ok {
		t.Fatal("expected per-file summary in envelope")
	}
}

func TestIngestionNodeFullScanUsesWorkspaceScanner(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "workspace.txt", "alpha\nbeta\ngamma\n")

	node := NewIngestionNode("ingest-full", IngestionSpec{
		Mode:          IngestionModeFull,
		WorkspaceRoot: root,
	})

	env := contextdata.NewEnvelope("task-2", "session-2")
	state.SetTaskEnvelope(env, &intake.TaskEnvelope{
		TaskID:    "task-2",
		SessionID: "session-2",
	})

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if got, ok := core.ResultField(result.Data, "files_scanned"); !ok || got == 0 {
		t.Fatal("expected files_scanned to be populated")
	}
	if got, ok := core.ResultField(result.Data, "chunks_created"); !ok || got == 0 {
		t.Fatal("expected chunks_created to be populated")
	}
}

func TestIngestionNodeIncrementalScanUsesGitDiff(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeTestFile(t, root, "tracked.txt", "first\n")
	gitRun(t, root, "add", "tracked.txt")
	gitRun(t, root, "commit", "-m", "initial")
	base := gitOutput(t, root, "rev-parse", "HEAD")
	writeTestFile(t, root, "tracked.txt", "first\nsecond\n")
	gitRun(t, root, "add", "tracked.txt")
	gitRun(t, root, "commit", "-m", "update")

	node := NewIngestionNode("ingest-incremental", IngestionSpec{
		Mode:          IngestionModeIncremental,
		WorkspaceRoot: root,
		SinceRef:      strings.TrimSpace(base),
	})

	env := contextdata.NewEnvelope("task-3", "session-3")
	state.SetTaskEnvelope(env, &intake.TaskEnvelope{
		TaskID:           "task-3",
		SessionID:        "session-3",
		IncrementalSince: strings.TrimSpace(base),
	})

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if got, ok := core.ResultField(result.Data, "since_ref"); !ok || got != strings.TrimSpace(base) {
		t.Fatalf("unexpected since_ref: %v", got)
	}
	if got, ok := core.ResultField(result.Data, "files_scanned"); !ok || got == 0 {
		t.Fatal("expected files_scanned to be populated")
	}
}

func TestIngestionNodeHandlesMissingTaskEnvelope(t *testing.T) {
	node := NewIngestionNode("ingest-missing", IngestionSpec{Mode: IngestionModeFilesOnly})
	env := contextdata.NewEnvelope("task-4", "session-4")

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if got, ok := core.ResultField(result.Data, "skipped"); !ok || got != true {
		t.Fatalf("expected skipped result, got %v", got)
	}
}

func writeTestFile(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return path
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	gitRun(t, root, "init")
	gitRun(t, root, "config", "user.email", "codex@example.com")
	gitRun(t, root, "config", "user.name", "Codex")
}

func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return string(out)
}
