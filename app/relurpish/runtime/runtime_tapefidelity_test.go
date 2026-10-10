//go:build live

// Live lane only: TestEucloTapeFidelity drives the full production runtime
// boot with the real model surface, so it belongs to the security/governance
// tier, not the hermetic sweep.
package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextstream"
	execution "codeburg.org/lexbit/relurpify/execution"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/sandbox"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestEucloTapeFidelity records a full RunTask pass through the production
// runtime. Its former skip premise — "empty recipe registry means no LLM calls
// to record" — no longer holds: recipes load from the copied workspace
// (workspace-truth resolution, D-8), so the boot sees the canonical recipe set.
func TestEucloTapeFidelity(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	cfg := ConfigForWorkspace(Config{AgentName: "euclo"}, workspace)
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, sandbox.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	defer func() {
		if err := rt.Close(context.Background()); err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	}()

	task := &execution.Task{
		ID:          "euclo-fidelity",
		Type:        string(execution.TaskTypeExecute),
		Instruction: "read the workspace and summarize it",
	}
	recordCtx := contextstream.WithTrigger(context.Background(), rt.Workspace.Environment.StreamTrigger)
	recordResult, err := rt.RunTask(recordCtx, task)
	if err != nil {
		t.Fatalf("record runtime run task: %v", err)
	}
	if recordResult == nil || !recordResult.Success {
		t.Fatalf("record runtime result = %#v", recordResult)
	}
}
