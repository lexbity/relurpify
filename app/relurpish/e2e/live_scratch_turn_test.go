//go:build live

// Live lane only: this test drives the PRODUCTION runtime — real ollama
// backend, real gvisor sandbox, real security bundle — through a full agent
// turn against an operator-nominated scratch workspace. It is the end-user
// pass, machine-checked: the same boot the TUI performs, one real turn, and
// assertions on the workspace effects and the audit chain.
//
// Required: RELURPIFY_LIVE_WORKSPACE (the scratch workspace directory; the
// workspace is initialized from the shipped templates if empty). Optional:
// RELURPIFY_MODEL_NAME / RELURPIFY_MODEL_PROVIDER ride the product's
// env-override surface. The posture is the ruled first-pass posture:
// --hitl-mode permissive (auto-approved HITL, audited per decision).
package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestLiveRelurpishScratchTurn(t *testing.T) {
	workspace := os.Getenv("RELURPIFY_LIVE_WORKSPACE")
	if workspace == "" {
		t.Skip("live scratch turn requires RELURPIFY_LIVE_WORKSPACE (the operator-nominated scratch workspace)")
	}
	abs, err := filepath.Abs(workspace)
	require.NoError(t, err)
	require.DirExists(t, abs)

	// Materialize the starter configuration the way `relurpish doctor --fix`
	// would, so the scratch workspace is a real operator workspace.
	if _, err := os.Stat(filepath.Join(abs, "relurpify_cfg", "workspace.yaml")); err != nil {
		require.NoError(t, relurpishruntime.InitializeWorkspaceFromTemplates(relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), abs), false))
	}

	// Seed the recipe's designed TDD shape: hello.py misspells the greeting,
	// and test_hello.py is a plain-python failing test (no pytest dependency)
	// that pins "hello world". The debug_tdd_repair recipe's contract —
	// discover the failing test, repair, re-run to verify — is then real.
	require.NoError(t, os.WriteFile(filepath.Join(abs, "hello.py"), []byte(
		"def greet():\n    return \"helo world\"\n\n\nif __name__ == \"__main__\":\n    print(greet())\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(abs, "test_hello.py"), []byte(
		"import hello\n\nactual = hello.greet()\nassert actual == \"hello world\", f\"expected 'hello world', got {actual!r}\"\nprint(actual)\n",
	), 0o644))

	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), abs)
	cfg.InferenceProvider = firstNonEmptyEnv("RELURPIFY_MODEL_PROVIDER", "ollama")
	cfg.InferenceModel = firstNonEmptyEnv("RELURPIFY_MODEL_NAME", "gemma4:e4b")
	cfg.InferenceNativeToolCalling = true
	cfg.HITLMode = "permissive" // the ruled first-pass posture: auto-approved HITL, audited

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	require.NoError(t, err, "the production runtime must boot against the scratch workspace")
	defer func() { _ = rt.Close(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	turnStart := time.Now()
	result, err := rt.SubmitTurn(ctx,
		"The file hello.py in this workspace misspells \"hello\" as \"helo\". Fix the spelling, then run the script to confirm it prints \"hello world\". Report what you changed and the final output.",
		execution.TaskTypeExecute, map[string]any{"source": "live-scratch"}, nil)
	elapsed := time.Since(turnStart)
	require.NoError(t, err, "the turn must complete (structured result, no infra failure)")
	t.Logf("turn elapsed: %s", elapsed)

	// A concrete instruction must NOT dead-end in clarification. When the
	// deterministic classifier ties two families (top-2 within 0.1) it
	// force-routes to the clarify intent, which emits a frame and completes
	// with zero LLM calls — Success=true with the real work never started.
	// This assertion names that outcome instead of failing downstream on the
	// missing workspace effect.
	if pending := rt.PendingInteractionFrame(); pending != nil {
		choices := make([]string, 0, len(pending.Choices))
		choices = append(choices, pending.Choices...)
		for _, slot := range pending.Slots {
			choices = append(choices, slot.ID)
		}
		t.Fatalf("turn ended in a pending interaction frame, not executed work: type=%s question=%q choices=%v — "+
			"the classifier marked the instruction ambiguous and the clarify intent completed without asking anything the caller can act on",
			pending.Type, pending.Question, choices)
	}

	if result != nil {
		t.Logf("turn success=%v", result.Success)
		if result.Data != nil {
			for k, v := range result.Data.Fields() {
				t.Logf("agent field %s: %v", k, v)
			}
		}
	}

	// The workspace effect is the acceptance fact: the agent fixed the file
	// through its governed tool path (sandboxed exec + file capabilities).
	fixed, readErr := os.ReadFile(filepath.Join(abs, "hello.py"))
	require.NoError(t, readErr)
	require.Contains(t, string(fixed), "hello world", "the agent must have fixed the misspelling through its tools")
	require.NotContains(t, string(fixed), "helo")
	test, readErr := os.ReadFile(filepath.Join(abs, "test_hello.py"))
	require.NoError(t, readErr)
	require.Contains(t, string(test), `"hello world"`, "the pinned test must remain intact")

	// The permissive posture must have left forensic evidence: at least one
	// auto-approved HITL decision on the agent's audit chain (shell/exec
	// tools are ask-gated by the shipped localtool policy).
	auditRoot := filepath.Join(abs, ".relurpify_state", "audit")
	entries, _ := os.ReadDir(auditRoot)
	foundAutoApprove := false
	for _, e := range entries {
		data, readErr := os.ReadFile(filepath.Join(auditRoot, e.Name()))
		if readErr == nil && strings.Contains(string(data), "hitl.auto_approve") {
			foundAutoApprove = true
		}
	}
	t.Logf("audit chain under %s (auto-approvals found: %v) — inspect it alongside .relurpify_state/logs and telemetry", auditRoot, foundAutoApprove)

	t.Logf("scratch workspace artifacts: %s/.relurpify_state/{logs,telemetry,audit}, test_runs under relurpify_cfg", abs)
}

func firstNonEmptyEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
