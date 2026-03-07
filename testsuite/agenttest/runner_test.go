package agenttest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/toolsys"
)

func TestFallbackManifestPath(t *testing.T) {
	workspace := t.TempDir()
	manifest := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
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
	before := &WorkspaceSnapshot{
		Files: map[string]string{
			"relurpify_cfg/sessions/workflow_state.db": "before",
		},
	}
	after := &WorkspaceSnapshot{
		Files: map[string]string{
			"relurpify_cfg/sessions/workflow_state.db": "after",
		},
	}

	changed := includeExpectedChangedFiles(nil, before, after, []string{"relurpify_cfg/sessions/workflow_state.db"})

	if len(changed) != 1 || changed[0] != "relurpify_cfg/sessions/workflow_state.db" {
		t.Fatalf("expected workflow_state.db to be restored, got %#v", changed)
	}
}
