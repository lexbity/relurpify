package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const e2eStreamRecipe = `thoughtrecipe e2e_stream_loop
"E2E product-gate fixture: a capturing react step grounds its result, and a second step streams the grounded finding back into its prompt."

trigger as capability:
  may read workspace
  keyword ["e2e-stream-corpus"]
  family ["review"]

input workspace: "**/*"
input prompt: user.request

agent collector uses react
agent summarizer uses react

run collector:
  from input.prompt
  goal "Collect the corpus."
  capture result -> state.findings

run summarizer:
  from state.findings
  stream "offline corpus findings" max 256 mode blocking
  goal "Summarize the captured findings."
  capture result -> state.summary
`

// TestBootTurnStreamsContextAndGroundsCaptures is the product gate: through
// the real runtime boot (offline mind, canonical recipe registry plus one
// authored stream fixture), one turn must (a) dispatch the stream-clause
// recipe, (b) render the compiled knowledge slice into a model call —
// contextstream.injected with chunks>0 on the runtime's event stream — and
// (c) grow the grounded-chunk count through the capturing steps.
func TestBootTurnStreamsContextAndGroundsCaptures(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
	})
	testhelper.InitGitRepo(t, workspace)
	recipePath := filepath.Join(workspace, "relurpify_cfg", "euclo", "e2e_stream_loop.erpe")
	if err := fs.WriteFileSecure(recipePath, []byte(e2eStreamRecipe)); err != nil {
		t.Fatal(err)
	}

	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer cancelHITL()

	events, cancelEvents := rt.SubscribeExecutionEvents()
	defer cancelEvents()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := rt.SubmitTurn(ctx, "review e2e-stream-corpus.md", execution.TaskTypeAnalysis, nil, nil); err != nil {
		t.Fatalf("submit turn: %v", err)
	}

	// The knowledge bridge forwards bus events asynchronously: wait on the
	// live subscription until both halves of the loop are observed, bounded.
	deadline := time.Now().Add(10 * time.Second)
	injected, chunks, committed := 0, 0, 0
	collect := func() {
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return
				}
				switch ev.Type {
				case telemetry.EventContextStreamInjected:
					injected++
					if n, ok := ev.Metadata["chunks"].(int); ok {
						chunks += n
					}
				case telemetry.EventChunkCommitted:
					committed++
				}
			case <-time.After(50 * time.Millisecond):
				return
			}
		}
	}
	for injected == 0 || committed == 0 {
		collect()
		if (injected > 0 && committed > 0) || time.Now().After(deadline) {
			break
		}
	}
	cancelEvents()
	if err := rt.Close(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	if injected == 0 {
		t.Fatalf("contextstream.injected never fired through the product runtime on a capturing stream recipe")
	}
	if chunks == 0 {
		t.Errorf("contextstream.injected fired with zero chunks")
	}
	if committed == 0 {
		t.Errorf("no chunk.committed observed: the capturing steps grounded nothing")
	}
}
