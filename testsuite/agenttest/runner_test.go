package agenttest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"github.com/lexcodex/relurpify/framework/workspacecfg"
)

func TestFallbackManifestPath(t *testing.T) {
	workspace := t.TempDir()
	manifest := workspacecfg.New(workspace).ManifestFile()
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(manifest, []byte("test"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got := fallbackManifestPath(filepath.Join(workspace, "testsuite", "agent.manifest.yaml"), workspace)
	if got != manifest {
		t.Fatalf("expected %s, got %s", manifest, got)
	}
}

func TestApplyCaseControlFlowOverrideSetsCodingMode(t *testing.T) {
	agent := &agents.CodingAgent{Tools: toolsys.NewToolRegistry()}
	if err := agent.Initialize(nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	err := applyCaseControlFlowOverride(agent, CaseSpec{
		Context: map[string]any{"mode": "code"},
		Overrides: CaseOverrideSpec{
			ControlFlow: "pipeline",
		},
	})
	if err != nil {
		t.Fatalf("applyCaseControlFlowOverride: %v", err)
	}

	if got := agent.ModeProfiles()[agents.ModeCode].ControlFlow; got != agents.ControlFlowPipeline {
		t.Fatalf("expected pipeline control flow, got %s", got)
	}
}

func TestIncludeExpectedChangedFilesRestoresIgnoredExpectation(t *testing.T) {
	workflowStateRel := filepath.ToSlash(filepath.Join(workspacecfg.DirName, "sessions", "workflow_state.db"))
	before := &WorkspaceSnapshot{
		Files: map[string]string{
			workflowStateRel: "before",
		},
	}
	after := &WorkspaceSnapshot{
		Files: map[string]string{
			workflowStateRel: "after",
		},
	}

	changed := includeExpectedChangedFiles(nil, before, after, []string{workflowStateRel})

	if len(changed) != 1 || changed[0] != workflowStateRel {
		t.Fatalf("expected workflow_state.db to be restored, got %#v", changed)
	}
}

func TestNewRunCaseLayoutUsesStructuredRunSubdirectories(t *testing.T) {
	runRoot := filepath.Join("/tmp", "run-1")
	layout := newRunCaseLayout(runRoot, "Write Docs", "llama3.2")

	caseKey := "Write_Docs__llama3_2"
	if got := layout.ArtifactsDir; got != filepath.Join(runRoot, "artifacts", caseKey) {
		t.Fatalf("ArtifactsDir = %q", got)
	}
	if got := layout.TmpDir; got != filepath.Join(runRoot, "tmp", caseKey) {
		t.Fatalf("TmpDir = %q", got)
	}
	if got := layout.WorkspaceDir; got != filepath.Join(runRoot, "tmp", caseKey, "workspace") {
		t.Fatalf("WorkspaceDir = %q", got)
	}
	if got := layout.LogPath; got != filepath.Join(runRoot, "logs", caseKey+".log") {
		t.Fatalf("LogPath = %q", got)
	}
	if got := layout.TelemetryPath; got != filepath.Join(runRoot, "telemetry", caseKey+".jsonl") {
		t.Fatalf("TelemetryPath = %q", got)
	}
	if got := layout.TapePath; got != filepath.Join(runRoot, "artifacts", caseKey, "tape.jsonl") {
		t.Fatalf("TapePath = %q", got)
	}
}
