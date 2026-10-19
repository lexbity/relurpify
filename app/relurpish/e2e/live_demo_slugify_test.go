//go:build live

// Live demo acceptance: the documented "fix slugify and run the tests" turn,
// driven through the production runtime (real ollama backend, real gvisor
// sandbox) against an operator-nominated demo workspace. It is the headless
// counterpart of the TUI demo script in devdocs/relurpish-how-to.md §8: the
// agent must edit stringkit.py through its governed tools and run the shipped
// test to green.
//
// Required: RELURPIFY_LIVE_WORKSPACE (the demo workspace). Optional:
// RELURPIFY_MODEL_NAME / RELURPIFY_MODEL_PROVIDER.
package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/telemetry/otelleg"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestDemoSlugifyTurn(t *testing.T) {
	workspace := os.Getenv("RELURPIFY_LIVE_WORKSPACE")
	if workspace == "" {
		t.Skip("requires RELURPIFY_LIVE_WORKSPACE")
	}
	abs, err := filepath.Abs(workspace)
	require.NoError(t, err)
	require.DirExists(t, abs)

	_ = os.RemoveAll(filepath.Join(abs, ".relurpify_state", "events.db"))
	_ = os.RemoveAll(filepath.Join(abs, ".relurpify_state", "memory"))

	before, err := os.ReadFile(filepath.Join(abs, "stringkit.py"))
	require.NoError(t, err)

	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), abs)
	cfg.InferenceProvider = firstNonEmptyEnv("RELURPIFY_MODEL_PROVIDER", "ollama")
	cfg.InferenceModel = firstNonEmptyEnv("RELURPIFY_MODEL_NAME", "gemma4:e4b")
	cfg.InferenceNativeToolCalling = true
	cfg.HITLMode = "permissive"

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	require.NoError(t, err, "the production runtime must boot against the demo workspace")

	callable := make([]string, 0)
	for _, tool := range rt.Tools.ModelCallableTools(context.Background()) {
		callable = append(callable, tool.Name())
	}
	t.Logf("model-callable tools: %v", callable)
	require.Contains(t, callable, "python_pytest",
		"the declared python_pytest runner must be model-callable, not merely resolvable")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	start := time.Now()
	result, err := rt.SubmitTurn(ctx,
		"The slugify function is broken: it keeps punctuation. test_slugify_strips_punctuation fails. Fix slugify and run the tests to verify.",
		execution.TaskTypeExecute, map[string]any{"source": "demo-slugify"}, nil)
	t.Logf("turn elapsed: %s", time.Since(start))
	require.NoError(t, err, "the turn must complete without infra failure")

	if pending := rt.PendingInteractionFrame(); pending != nil {
		t.Fatalf("turn paused on an interaction frame: type=%s question=%q", pending.Type, pending.Question)
	}
	if result != nil {
		t.Logf("turn success=%v", result.Success)
		if result.Data != nil {
			for k, v := range result.Data.Fields() {
				t.Logf("  field %s: %v", k, v)
			}
		}
	}

	after, err := os.ReadFile(filepath.Join(abs, "stringkit.py"))
	require.NoError(t, err)
	t.Logf("stringkit.py after turn:\n%s", after)

	// Independent acceptance: the shipped test must pass when run directly.
	cmd := exec.Command("python3", "-m", "pytest", "-q")
	cmd.Dir = abs
	out, runErr := cmd.CombinedOutput()
	t.Logf("python3 -m pytest -q => err=%v\n%s", runErr, out)

	_ = rt.Close(context.Background())
	reportDemoTurn(t, filepath.Join(abs, ".relurpify_state", "telemetry", "otlp"))

	require.NotEqual(t, string(before), string(after), "the agent must have edited stringkit.py")
	require.NoError(t, runErr, "the shipped test must pass after the turn")
}

func reportDemoTurn(t *testing.T, segmentDir string) {
	t.Helper()
	records, err := otelleg.ReadSegmentDirLogRecords(segmentDir)
	if err != nil {
		t.Logf("telemetry unavailable: %v", err)
		return
	}
	for _, rec := range records {
		switch rec.Name {
		case "euclo.route.selected", "euclo.execution.complete", "tool_call", "capability_call",
			"llm_prompt", "llm_response", "step.operational_failure", "recipe.vocabulary_not_model_callable":
			attrs := make([]string, 0, 4)
			for _, k := range []string{"route_id", "decided_by", "tool", "capability", "category", "reason", "success", "unverified"} {
				if v := rec.Attr(k); strings.TrimSpace(v) != "" {
					attrs = append(attrs, k+"="+v)
				}
			}
			t.Logf("[telemetry] %-40s %s", rec.Name, strings.Join(attrs, " "))
		}
	}
}
