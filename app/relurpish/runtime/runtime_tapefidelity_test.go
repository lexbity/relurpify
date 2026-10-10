//go:build live

// Live lane only: TestEucloTapeFidelity drives the full production runtime
// boot, so it belongs to the security/governance tier, not the hermetic sweep.
package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextstream"
	execution "codeburg.org/lexbit/relurpify/execution"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/sandbox"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const fidelityInstruction = "read the workspace and summarize it"

// TestEucloTapeFidelity proves the recorded live shape replays through the
// fixed boot: the full production runtime runs the committed smoke tape twice
// and both passes must produce byte-identical result flags — the recorded
// model surface, no network, no Ollama required. Its former skip premise
// ("empty recipe registry means no LLM calls to record") no longer holds:
// recipes load from the copied workspace (workspace-truth resolution, D-8),
// so the boot sees the canonical recipe set.
func TestEucloTapeFidelity(t *testing.T) {
	tapePath, err := filepath.Abs(filepath.Join("..", "..", "..", "testsuite", "tapes", "euclo_gemma4_smoke.tape.jsonl"))
	if err != nil {
		t.Fatalf("resolve committed tape: %v", err)
	}
	runOnce := func(attempt string) *execution.Result {
		t.Helper()
		workspace := t.TempDir()
		copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

		cfg := ConfigForWorkspace(Config{AgentName: "euclo"}, workspace)
		cfg.SecurityRunner = fakeCommandRunner{}
		cfg.SandboxBackendFactory = func(context.Context, string, sandbox.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
			return &fakeSandboxRuntime{}, nil
		}
		// Replay the committed smoke tape: the recorded mind, consumed
		// deterministically (one fresh tape model per boot).
		cfg.InferenceProvider = "tape"
		cfg.InferenceTapePath = tapePath

		rt, err := New(context.Background(), cfg, config.Secrets{})
		if err != nil {
			t.Fatalf("%s: boot runtime: %v", attempt, err)
		}
		// The production broker is fail-closed: the explicit test-side
		// approver answers its requests, exactly as an operator would.
		stopApprover := testhelper.AutoApproveHITL(t, rt)
		defer stopApprover()
		defer func() {
			if err := rt.Close(context.Background()); err != nil {
				t.Fatalf("%s: close runtime: %v", attempt, err)
			}
		}()

		task := &execution.Task{
			ID:          "euclo-fidelity",
			Type:        string(execution.TaskTypeExecute),
			Instruction: fidelityInstruction,
		}
		ctx := contextstream.WithTrigger(context.Background(), rt.Workspace.Environment.StreamTrigger)
		result, err := rt.RunTask(ctx, task)
		if err != nil {
			t.Fatalf("%s: runtime run task: %v", attempt, err)
		}
		if result == nil {
			t.Fatalf("%s: runtime result = nil", attempt)
		}
		return result
	}

	first := runOnce("first")
	if !first.Success {
		t.Fatalf("first replay not successful: error=%q", first.Error)
	}
	second := runOnce("second")
	if !second.Success {
		t.Fatalf("second replay not successful: error=%q", second.Error)
	}
	if first.Success != second.Success || first.Error != second.Error {
		t.Fatalf("replay determinism: result flags diverge\nfirst:  success=%v error=%q\nsecond: success=%v error=%q", first.Success, first.Error, second.Success, second.Error)
	}
	if got, want := fmt.Sprint(first.Data), fmt.Sprint(second.Data); got != want {
		t.Fatalf("replay determinism: result payload diverges\nfirst:  %s\nsecond: %s", got, want)
	}
}
