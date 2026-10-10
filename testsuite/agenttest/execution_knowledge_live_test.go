//go:build live

// Live lane only: this smoke case drives the harness against a real model
// backend, so it belongs to the security/governance tier (NFR-8: live-tagged
// additions stay out of the hermetic sweep).
package agenttest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	liveSmokeSeedBody = "live smoke corpus: gemma4 contextstream sentinel"
	liveSmokeRecipe   = `thoughtrecipe live_smoke_stream
"Live-lane fixture: one react run behind a blocking stream clause over the live smoke corpus."

trigger as capability:
  may read workspace
  keyword ["live-smoke-corpus"]
  family ["live-smoke-fixtures"]

input workspace: "**/*"
input prompt: user.request

agent worker uses react

run worker:
  from input.prompt
  stream "live smoke corpus notes" max 256 mode blocking
  goal "Summarize the streamed live smoke corpus."
  capture result -> state.summary
`
)

// TestLiveContextStreamInjectedSmoke observes the bidirectional loop on a
// real-model run: a stream-clause recipe over a seeded graph must render the
// compiled slice into a model call (contextstream.injected with chunks>0),
// and the case report must carry the aggregate. The backend resolves through
// the product's own env-override surface (RELURPIFY_MODEL_PROVIDER /
// RELURPIFY_MODEL_NAME / RELURPIFY_OLLAMA_HOST); without a configured model
// the case skips — the live lane, not the hermetic sweep, owns real models.
func TestLiveContextStreamInjectedSmoke(t *testing.T) {
	overrides, err := config.LoadEnvOverrides(os.Environ())
	if err != nil {
		t.Fatalf("load env overrides: %v", err)
	}
	if overrides.ModelName == "" {
		t.Skip("live smoke requires RELURPIFY_MODEL_NAME (optionally RELURPIFY_MODEL_PROVIDER, RELURPIFY_OLLAMA_HOST)")
	}

	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	recipePath := filepath.Join(ws, "relurpify_cfg", "euclo", "live_smoke_stream.erpe")
	if err := fs.MkdirAllSecure(filepath.Dir(recipePath)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(recipePath, []byte(liveSmokeRecipe)); err != nil {
		t.Fatal(err)
	}
	desc.Instruction = "summarize live-smoke-corpus.md"
	desc.ModelName = overrides.ModelName
	if overrides.ModelProvider != "" {
		desc.BackendProvider = overrides.ModelProvider
		desc.BackendFamily = overrides.ModelProvider
	}
	if overrides.OllamaHost != "" {
		desc.BackendEndpoint = overrides.OllamaHost
	}

	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})
	ctx := context.Background()
	if err := exec.buildSecurity(ctx, desc); err != nil {
		t.Fatalf("security: %v", err)
	}
	if err := exec.buildCapability(ctx, desc); err != nil {
		t.Fatalf("capability: %v", err)
	}
	exec.telemetry = exec.buildTelemetry(desc)
	if err := exec.buildKnowledge(); err != nil {
		t.Fatalf("knowledge: %v", err)
	}
	if err := exec.buildModel(ctx, desc); err != nil {
		t.Fatalf("model: %v", err)
	}

	// Seed the graph so the stream clause has content to compile: grounded
	// through the same write boundary a capture uses.
	seedReport, err := exec.knowledge.Grounding.Ground(ctx, []knowledge.GroundingItem{{
		Value:          liveSmokeSeedBody,
		TypeAnnotation: "text",
		Kind:           knowledge.ChunkKindCapture,
		StateKey:       "live.seed",
		TaskID:         desc.RunID,
	}})
	if err != nil {
		t.Fatalf("seed grounding: %v", err)
	}
	if len(seedReport.Grounded) != 1 {
		t.Fatalf("seed not grounded: %+v", seedReport)
	}

	workspace := firstNonEmpty(desc.DerivedWorkspaceRoot, desc.WorkspaceRoot)
	deps := exec.assembleDeps(desc, exec.telemetry)
	if err := exec.createAgent(deps, workspace); err != nil {
		t.Fatalf("agent: %v", err)
	}
	defer exec.cleanup()

	task := &execution.Task{ID: desc.RunID, Type: "chat", Instruction: desc.Instruction}
	env := contextdata.NewEnvelope(desc.RunID, desc.RunID)
	result, execErr := exec.currentExecutor().Execute(ctx, task, env)
	if execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if result == nil || !result.Success {
		t.Fatalf("execute result = %#v", result)
	}

	// The streamed slice reached the model: telemetry carries the injection,
	// with the compiler's chunk accounting.
	injected := 0
	chunks := 0
	for _, ev := range exec.recorder.Events() {
		if ev.Type != telemetry.EventContextStreamInjected {
			continue
		}
		injected++
		if n, ok := ev.Metadata["chunks"].(int); ok {
			chunks += n
		}
	}
	if injected == 0 {
		t.Fatalf("contextstream.injected never fired on a real-model run with a seeded graph")
	}
	if chunks == 0 {
		t.Errorf("contextstream.injected fired with zero chunks")
	}

	report := &CaseReport{}
	applyRecordedTelemetry(report, exec.recorder.Events())
	if report.ContextStream.Injected == 0 || report.ContextStream.Chunks == 0 {
		t.Errorf("case report carries no context-stream aggregate: %+v", report.ContextStream)
	}
	if report.Knowledge.GroundedChunks == 0 {
		t.Errorf("case report carries no knowledge aggregate: %+v", report.Knowledge)
	}
}
