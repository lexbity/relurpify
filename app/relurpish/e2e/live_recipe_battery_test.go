//go:build live

// Live lane: executes the DSL breadth battery (and the mixed-construct
// recipes) against the PRODUCTION runtime — one recipe per fresh workspace,
// offline backend (deterministic), governed HITL. This is the author-facing
// acceptance: a recipe written from the language guide must boot, dispatch,
// execute to an honest terminal state, and record telemetry — not merely
// compile. Compile-time coverage lives in the hermetic battery
// (thoughtrecipes.TestDSLBreadthBattery); this test owns "does it run".
package e2e

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// batteryInstruction is the proven classifier-clean instruction: it
// deterministically classifies family=debug with needsClarify=false, so the
// workspace's single recipe is dispatched instead of the clarify intent.
const batteryInstruction = "The file hello.py in this workspace misspells \"hello\" as \"helo\". Fix the spelling, then run the script to confirm it prints \"hello world\". Report what you changed and the final output."

func batteryRecipeDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
	return filepath.Join(repoRoot, "named", "euclo", "thoughtrecipes", "testdata", "battery")
}

func TestLiveRecipeBattery(t *testing.T) {
	dir := batteryRecipeDir(t)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".erpe") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(dir, name))
			require.NoError(t, err)
			// Single-candidate selection: retarget the recipe at the debug
			// family so the proven instruction dispatches it. Family is
			// selection metadata; the constructs under test are untouched.
			recipe := retargetFamily(string(source), "debug")
			require.NotEmpty(t, recipe, "family line must exist")

			workspace := t.TempDir()
			require.NoError(t, relurpishruntime.InitializeWorkspaceFromTemplates(relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace), false))
			recipesDir := filepath.Join(workspace, "relurpify_cfg", "euclo")
			// Single-recipe workspace: the shipped recipe set and the shipped
			// handoff table both leave — the workspace author's freedom this
			// scenario exercises.
			shipped, err := os.ReadDir(recipesDir)
			require.NoError(t, err)
			// The shipped recipes and the shipped handoff table (families.yaml)
			// both leave: this workspace author ships exactly one recipe.
			for _, e := range shipped {
				require.NoError(t, os.Remove(filepath.Join(recipesDir, e.Name())))
			}
			require.NoError(t, os.WriteFile(filepath.Join(recipesDir, name), []byte(recipe), 0o644))

			// Defined memory state: wipe both observation stores.
			require.NoError(t, os.RemoveAll(filepath.Join(workspace, ".relurpify_state", "events.db")))
			require.NoError(t, os.RemoveAll(filepath.Join(workspace, ".relurpify_state", "memory")))

			// The TDD fixture gives tool-driven recipes real work.
			require.NoError(t, os.WriteFile(filepath.Join(workspace, "hello.py"), []byte(
				"def greet():\n    return \"helo world\"\n\n\nif __name__ == \"__main__\":\n    print(greet())\n",
			), 0o644))

			cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
			cfg.InferenceProvider = "offline"
			// Permissive posture: governed HITL waits out the full 45s
			// HITLTimeout per permission request with no human to answer —
			// the offline battery would burn a minute per ask-gated tool.
			// Authored `ask user` frames are interaction frames, not HITL
			// permission requests; they still pause for the caller.
			cfg.HITLMode = "permissive"

			rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
			require.NoError(t, err, "recipe must boot the production runtime")
			defer func() { _ = rt.Close(context.Background()) }()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			result, err := rt.SubmitTurn(ctx, batteryInstruction, execution.TaskTypeExecute, map[string]any{"source": "live-recipe-battery"}, nil)
			require.NoError(t, err, "turn must reach its honest boundary without infra failure")

			if pending := rt.PendingInteractionFrame(); pending != nil {
				// Ask-bearing recipes pause at the frame; that is the
				// construct working, not a failure.
				t.Logf("paused on interaction frame: question=%q choices=%v", pending.Question, pending.Choices)
				return
			}
			require.NotNil(t, result, "no frame and no result: the turn vanished")
			t.Logf("turn success=%v", result.Success)
			if !result.Success {
				// Honest failure is acceptable for a battery run (a scripted
				// model cannot satisfy every authored goal); what is NOT
				// acceptable is an unclassified or infra-shaped failure.
				require.NotContains(t, result.Error, "boot:", "recipe must not fail the boot")
				require.NotContains(t, result.Error, "not registered", "recipe vocabulary must resolve")
			}
		})
	}
}

// retargetFamily replaces the recipe's family association line with the given
// family (idempotent when it already declares it). It fails loudly (empty
// result) when no family line exists.
func retargetFamily(recipe, family string) string {
	if strings.Contains(recipe, "family [\""+family+"\"]") {
		return recipe
	}
	start := strings.Index(recipe, "family [")
	if start < 0 {
		return ""
	}
	end := strings.Index(recipe[start:], "\n")
	if end < 0 {
		return ""
	}
	return recipe[:start] + "family [\"" + family + "\"]" + recipe[start+end:]
}
