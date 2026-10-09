package framework

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/app/envcomposition"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

// configurableCaptureNode enqueues one GroundingItem through the run's capture
// sink with full provenance (workspace/session/recipe), so the grounded chunk
// is scoped for re-ground queries.
type configurableCaptureNode struct {
	id          string
	value       any
	epistemics  knowledge.Epistemics
	origin      contextdata.OriginClass
	stateKey    string
	workspaceID string
	recipeID    string
	sessionID   string
	sourceIDs   []knowledge.ChunkID
}

func (n *configurableCaptureNode) ID() string                { return n.id }
func (n *configurableCaptureNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

func (n *configurableCaptureNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	sink := agentgraph.CaptureSinkFromContext(ctx)
	if sink == nil {
		return nil, fmt.Errorf("%s: no capture sink in context", n.id)
	}
	sink.EnqueueCapture(knowledge.GroundingItem{
		Value:          n.value,
		TypeAnnotation: "Text",
		Epistemics:     n.epistemics,
		Origin:         n.origin,
		StateKey:       n.stateKey,
		NodeID:         n.id,
		TaskID:         env.TaskID,
		SessionID:      n.sessionID,
		WorkspaceID:    n.workspaceID,
		RecipeID:       n.recipeID,
		Kind:           knowledge.ChunkKindCapture,
		SourceChunkIDs: n.sourceIDs,
	})
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

// newIntegrityRuntime boots the real composition over a temp Badger store.
func newIntegrityRuntime(t *testing.T, workspace string) (*envcomposition.KnowledgeRuntime, *graphdb.Engine) {
	t.Helper()
	ctx := context.Background()
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(workspace))
	require.NoError(t, err)
	runtime, err := envcomposition.BuildKnowledgeRuntime(envcomposition.KnowledgeRuntimeInput{
		GraphDB:       engine,
		WorkspaceRoot: workspace,
	})
	require.NoError(t, err)
	t.Cleanup(runtime.Close)
	t.Cleanup(func() { _ = engine.Close(ctx) })
	return runtime, engine
}

// groundedCapture locates the grounded chunk for a state key in a store.
func groundedCapture(t *testing.T, store *knowledge.ChunkStore, stateKey string) *knowledge.KnowledgeChunk {
	t.Helper()
	all, err := store.FindByWorkspace("ws")
	require.NoError(t, err)
	for i := range all {
		if key, _ := all[i].Body.Fields["state_key"].(string); key == stateKey {
			return &all[i]
		}
	}
	t.Fatalf("no grounded chunk for state key %q", stateKey)
	return nil
}

// TestCaptureEpochRestartRegroundsMemory proves AC-3 in its full form: a step
// capture is durable memory (not envelope-only), a later run's stream query
// retrieves it, and after a store restart the re-ground query returns it —
// cross-run read-your-writes through the engine (the #10 decision made real).
func TestCaptureEpochRestartRegroundsMemory(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()

	runtime, engine := newIntegrityRuntime(t, workspace)

	// Run 1: a recipe step (node "step-1") captures a typed finding.
	graph := agentgraph.NewGraph()
	graph.SetGrounder(runtime.Grounding)
	graph.SetDrain(runtime.Drain)
	require.NoError(t, graph.AddNode(&configurableCaptureNode{
		id: "step-1", value: "restart reground phrase",
		epistemics: knowledge.EpistemicClaimed, origin: contextdata.OriginLLM,
		stateKey: "state.findings", workspaceID: "ws", recipeID: "review", sessionID: "session-reground",
	}))
	require.NoError(t, graph.SetStart("step-1"))
	_, err := graph.Execute(ctx, contextdata.NewEnvelope("run-1-task", "session-reground"))
	require.NoError(t, err)

	captured := groundedCapture(t, runtime.KnowledgeStore, "state.findings")
	require.Equal(t, knowledge.EpistemicClaimed, knowledge.Epistemics(captured.Epistemics))

	// Run 2 (same runtime, next epoch): a stream query finds the prior capture.
	require.Eventually(t, func() bool {
		results, err := runtime.Retriever.Retrieve(ctx, retrieval.RetrievalQuery{Text: "reground"})
		if err != nil || results == nil {
			return false
		}
		for _, ranked := range results.Ranked {
			if ranked.ChunkID == captured.ID {
				return true
			}
		}
		return false
	}, 2*time.Second, time.Millisecond, "cross-run RYW: a later run's stream must retrieve run 1's capture")

	// Restart: the engine closes and reopens on the same directory — the chunk
	// corpus is durable runtime state, not envelope memory.
	runtime.Close()
	require.NoError(t, engine.Close(ctx))
	require.NoError(t, ctx.Err())

	restarted, reopened := newIntegrityRuntime(t, workspace)
	_ = reopened

	// The D5 re-ground port returns the prior run's final capture.
	result, err := restarted.Grounding.Reground(ctx, knowledge.RegroundRequest{
		WorkspaceID: "ws",
		SessionID:   "session-reground",
		RecipeID:    "review",
		MaxEntries:  64,
	})
	require.NoError(t, err)
	require.True(t, result.Grounded, "a prior grounded run must re-ground after restart")
	require.NotEmpty(t, result.Entries)
	var entry *knowledge.RegroundEntry
	for i := range result.Entries {
		if result.Entries[i].StateKey == "state.findings" {
			entry = &result.Entries[i]
			break
		}
	}
	require.NotNil(t, entry, "the re-ground payload must carry state.findings")
	require.Equal(t, string(captured.ID), entry.ChunkID, "re-ground entry must reference the durable chunk")

	// And retrieval against the restarted runtime resolves the same memory.
	require.Eventually(t, func() bool {
		results, err := restarted.Retriever.Retrieve(ctx, retrieval.RetrievalQuery{Text: "reground"})
		if err != nil || results == nil {
			return false
		}
		for _, ranked := range results.Ranked {
			if ranked.ChunkID == captured.ID {
				return true
			}
		}
		return false
	}, 2*time.Second, time.Millisecond, "restarted runtime must serve the durable capture")
}

// TestCaptureEpistemicsInAssembledSystem proves AC-6 through the real
// composition: an `as given` capture whose dataflow origin is llm grounds as
// claimed llm-generated (the origin floor beats the annotation), while an
// `as given` capture fed by a user-origin value grounds as given
// workspace-trusted — trust only descends.
func TestCaptureEpistemicsInAssembledSystem(t *testing.T) {
	ctx := context.Background()
	runtime, _ := newIntegrityRuntime(t, t.TempDir())

	// User-origin `as given`: grounds as given workspace-trusted.
	givenEnv := contextdata.NewEnvelope("task-given", "session-given")
	givenEnv.SetWorkingValueWithOrigin("user.request", "user provided", contextdata.MemoryClassTask, contextdata.OriginUser)
	givenGraph := agentgraph.NewGraph()
	givenGraph.SetGrounder(runtime.Grounding)
	givenGraph.SetDrain(runtime.Drain)
	require.NoError(t, givenGraph.AddNode(&configurableCaptureNode{
		id: "step-given", value: "user provided",
		epistemics: knowledge.EpistemicGiven, origin: contextdata.OriginUser,
		stateKey: "state.answer", workspaceID: "ws", recipeID: "review", sessionID: "session-given",
	}))
	require.NoError(t, givenGraph.SetStart("step-given"))
	result, err := givenGraph.Execute(ctx, givenEnv)
	require.NoError(t, err)
	require.True(t, result.Success)

	given := groundedCapture(t, runtime.KnowledgeStore, "state.answer")
	require.Equal(t, string(knowledge.EpistemicGiven), given.Epistemics)
	require.Equal(t, agentspec.TrustClassWorkspaceTrusted, given.TrustClass)

	// LLM-origin `as given`: the annotation cannot elevate an llm dataflow; the
	// chunk grounds as claimed llm-generated.
	llmGraph := agentgraph.NewGraph()
	llmGraph.SetGrounder(runtime.Grounding)
	llmGraph.SetDrain(runtime.Drain)
	require.NoError(t, llmGraph.AddNode(&configurableCaptureNode{
		id: "step-claimed", value: "agent synthesis",
		epistemics: knowledge.EpistemicGiven, origin: contextdata.OriginLLM,
		stateKey: "state.summary", workspaceID: "ws", recipeID: "review", sessionID: "session-claimed",
	}))
	require.NoError(t, llmGraph.SetStart("step-claimed"))
	result, err = llmGraph.Execute(ctx, contextdata.NewEnvelope("task-claimed", "session-claimed"))
	require.NoError(t, err)
	require.True(t, result.Success)

	claimed := groundedCapture(t, runtime.KnowledgeStore, "state.summary")
	require.Equal(t, string(knowledge.EpistemicClaimed), claimed.Epistemics,
		"an llm-origin given capture must downgrade to claimed")
	require.Equal(t, agentspec.TrustClassLLMGenerated, claimed.TrustClass,
		"the origin floor must beat the annotation; trust never elevates")
}

// TestCaptureEpochLeaksNoGoroutines proves AC-7 at the composition level: one
// hundred sequential grounded runs leave no goroutines behind.
func TestCaptureEpochLeaksNoGoroutines(t *testing.T) {
	runtime, _ := newIntegrityRuntime(t, t.TempDir())
	graph := agentgraph.NewGraph()
	graph.SetGrounder(runtime.Grounding)
	graph.SetDrain(runtime.Drain)
	require.NoError(t, graph.AddNode(&configurableCaptureNode{
		id: "step-0", value: "leak finding",
		epistemics: knowledge.EpistemicClaimed, origin: contextdata.OriginLLM,
		stateKey: "state.finding", workspaceID: "ws", recipeID: "review", sessionID: "session-leak",
	}))
	require.NoError(t, graph.AddNode(agentgraphTerminal("done")))
	require.NoError(t, graph.AddEdge("step-0", "done", nil, false))
	require.NoError(t, graph.SetStart("step-0"))

	testhelper.NoGoroutineLeak(t, func() {
		for i := 0; i < 100; i++ {
			env := contextdata.NewEnvelope(fmt.Sprintf("task-leak-%d", i), "session-leak")
			if _, err := graph.Execute(context.Background(), env); err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
		}
	})
}

// TestCaptureIngestToQueryableLatency measures the NFR-2 mechanism: after a
// capture barrier, the committed chunk becomes retrievable without waiting the
// snapshot TTL — far inside the one-second tripwire.
func TestCaptureIngestToQueryableLatency(t *testing.T) {
	ctx := context.Background()
	runtime, _ := newIntegrityRuntime(t, t.TempDir())

	graph := agentgraph.NewGraph()
	graph.SetGrounder(runtime.Grounding)
	graph.SetDrain(runtime.Drain)
	require.NoError(t, graph.AddNode(&configurableCaptureNode{
		id: "step-latency", value: "latency probe term",
		epistemics: knowledge.EpistemicClaimed, origin: contextdata.OriginLLM,
		stateKey: "state.probe", workspaceID: "ws", recipeID: "review", sessionID: "session-latency",
	}))
	require.NoError(t, graph.SetStart("step-latency"))
	_, err := graph.Execute(ctx, contextdata.NewEnvelope("task-latency", "session-latency"))
	require.NoError(t, err)
	captured := groundedCapture(t, runtime.KnowledgeStore, "state.probe")

	started := time.Now()
	require.Eventually(t, func() bool {
		results, err := runtime.Retriever.Retrieve(ctx, retrieval.RetrievalQuery{Text: "latency probe"})
		if err != nil || results == nil {
			return false
		}
		for _, ranked := range results.Ranked {
			if ranked.ChunkID == captured.ID {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond, "committed capture must become retrievable")
	t.Logf("ingest→queryable latency: %s", time.Since(started))
	require.Less(t, time.Since(started), 500*time.Millisecond,
		"NFR-2 tripwire: capture commit → retrieval far under the snapshot TTL")
}

// agentgraphTerminal is a minimal terminating node for the leak battery.
type agentgraphTerminal string

func (n agentgraphTerminal) ID() string { return string(n) }
func (n agentgraphTerminal) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeTerminal
}
func (n agentgraphTerminal) Execute(context.Context, *contextdata.Envelope) (*execution.Result, error) {
	return &execution.Result{NodeID: string(n), Success: true}, nil
}
