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
	"codeburg.org/lexbit/relurpify/execution/compiler"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

// realtimeCaptureNode captures a typed finding through the run's capture sink,
// grounding it at the epoch barrier. It records the streamed-context sources so
// the test can prove the grounds edge.
type realtimeCaptureNode struct {
	id       string
	sourceID knowledge.ChunkID
}

func (n *realtimeCaptureNode) ID() string                { return n.id }
func (n *realtimeCaptureNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

func (n *realtimeCaptureNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	sink := agentgraph.CaptureSinkFromContext(ctx)
	if sink == nil {
		return nil, fmt.Errorf("%s: no capture sink in context", n.id)
	}
	sink.EnqueueCapture(knowledge.GroundingItem{
		Value:          map[string]any{"text": "alpha findings"},
		TypeAnnotation: "ReviewFindings",
		Epistemics:     knowledge.EpistemicClaimed,
		Origin:         contextdata.OriginLLM,
		StateKey:       "state.findings",
		NodeID:         n.id,
		TaskID:         env.TaskID,
		SessionID:      env.SessionID,
		WorkspaceID:    "ws",
		Kind:           knowledge.ChunkKindCapture,
		SourceChunkIDs: []knowledge.ChunkID{n.sourceID},
	})
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

// realtimeCheckNode asserts the previous step's capture is already durable in
// the store when this node runs: read-your-writes across the epoch barrier.
type realtimeCheckNode struct {
	id    string
	store *knowledge.ChunkStore
}

func (n *realtimeCheckNode) ID() string                { return n.id }
func (n *realtimeCheckNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

func (n *realtimeCheckNode) Execute(_ context.Context, _ *contextdata.Envelope) (*execution.Result, error) {
	all, err := n.store.FindAll()
	if err != nil {
		return nil, err
	}
	for _, chunk := range all {
		if key, _ := chunk.Body.Fields["state_key"].(string); key == "state.findings" {
			return &execution.Result{NodeID: n.id, Success: true}, nil
		}
	}
	return nil, fmt.Errorf("%s: capture not durable at the barrier (read-your-writes violation)", n.id)
}

// TestKnowledgeRealtimeCaptureToCompile walks the full realtime chain through
// the real composition path: capture → epoch barrier → grounded chunk with a
// grounds edge → epoch.closed telemetry → retrieval sees it; then a revision
// change stales an affected file chunk and the next compile reports it as a
// skipped-stale gap instead of streaming it.
func TestKnowledgeRealtimeCaptureToCompile(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(workspace))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(ctx)) })

	runtime, err := envcomposition.BuildKnowledgeRuntime(envcomposition.KnowledgeRuntimeInput{
		GraphDB:       engine,
		WorkspaceRoot: workspace,
	})
	require.NoError(t, err)
	t.Cleanup(runtime.Close)

	// Streamed context the capture grounds against.
	source, err := runtime.KnowledgeStore.Save(ctx, knowledge.KnowledgeChunk{
		ID:          "chunk:context:source",
		WorkspaceID: "ws",
		TrustClass:  agentspec.TrustClassBuiltinTrusted,
		Freshness:   knowledge.FreshnessValid,
		Provenance:  knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: time.Now().UTC()},
		Body:        knowledge.ChunkBody{Raw: "context source", Fields: map[string]any{"content": "context source"}},
	})
	require.NoError(t, err)

	tel := &recordingTelemetrySink{}
	graph := agentgraph.NewGraph()
	require.NoError(t, graph.SetTelemetry(tel))
	graph.SetGrounder(runtime.Grounding)
	graph.SetDrain(runtime.Drain)
	require.NoError(t, graph.AddNode(&realtimeCaptureNode{id: "step1", sourceID: source.ID}))
	require.NoError(t, graph.AddNode(&realtimeCheckNode{id: "step2", store: runtime.KnowledgeStore}))
	require.NoError(t, graph.SetStart("step1"))
	require.NoError(t, graph.AddEdge("step1", "step2", nil, false))

	env := contextdata.NewEnvelope("task-realtime", "session-realtime")
	result, err := graph.Execute(ctx, env)
	require.NoError(t, err)
	require.True(t, result.Success)

	// (a) The capture is durable with a grounds edge to the streamed context.
	all, err := runtime.KnowledgeStore.FindAll()
	require.NoError(t, err)
	var captureChunk *knowledge.KnowledgeChunk
	for i := range all {
		if key, _ := all[i].Body.Fields["state_key"].(string); key == "state.findings" {
			captureChunk = &all[i]
			break
		}
	}
	require.NotNil(t, captureChunk, "the capture must be grounded")
	grounds, err := runtime.KnowledgeStore.LoadEdgesFrom(captureChunk.ID, knowledge.EdgeKindGrounds)
	require.NoError(t, err)
	require.Len(t, grounds, 1)
	require.Equal(t, source.ID, grounds[0].ToChunk)

	// (c) The barrier emitted epoch telemetry.
	require.GreaterOrEqual(t, tel.count(fwtelemetry.EventEpochClosed), 1, "the epoch barrier must emit epoch.closed")
	require.GreaterOrEqual(t, tel.count(fwtelemetry.EventCaptureGrounded), 1, "the capture must emit capture.grounded")
	require.Greater(t, env.AssemblyMetadataSnapshot().EpochID, uint64(0), "the envelope must carry a closed epoch")

	// (b) The retriever snapshot catches up on the same bus: the capture is
	// retrievable without waiting the snapshot TTL.
	require.Eventually(t, func() bool {
		retrieved, err := runtime.Retriever.Retrieve(ctx, retrieval.RetrievalQuery{Text: "alpha"})
		if err != nil || retrieved == nil {
			return false
		}
		for _, ranked := range retrieved.Ranked {
			if ranked.ChunkID == captureChunk.ID {
				return true
			}
		}
		return false
	}, 2*time.Second, time.Millisecond, "captured chunk must become retrievable after the barrier")

	// Revision drift: the git watcher publishes CodeRevisionChanged on the same
	// bus; the invalidation pass stales the affected file chunk.
	fileID := knowledge.CanonicalChunkID(knowledge.ChunkKindFile, []byte("tracked.go"))
	_, err = runtime.KnowledgeStore.Save(ctx, knowledge.KnowledgeChunk{
		ID:          fileID,
		WorkspaceID: "ws",
		TrustClass:  agentspec.TrustClassBuiltinTrusted,
		Freshness:   knowledge.FreshnessValid,
		Provenance: knowledge.ChunkProvenance{
			CodeStateRef: "rev-old",
			CompiledBy:   knowledge.CompilerDeterministic,
			Timestamp:    time.Now().UTC(),
		},
		Body: knowledge.ChunkBody{Raw: "tracked managed content", Fields: map[string]any{"file_path": "tracked.go"}},
	})
	require.NoError(t, err)

	runtime.KnowledgeEvents.EmitCodeRevisionChanged(knowledge.CodeRevisionChangedPayload{
		WorkspaceRoot: workspace,
		NewRevision:   "rev-new",
		AffectedPaths: []string{"tracked.go"},
	})
	require.Eventually(t, func() bool {
		fresh, err := runtime.KnowledgeStore.FindFreshByFilePath("tracked.go")
		return err == nil && len(fresh) == 0
	}, 2*time.Second, time.Millisecond, "revision change must stale the tracked file chunk")

	// (AC-5) The next compile seeded on the stale chunk reports it as a
	// skipped-stale gap rather than streaming it.
	compiled, _, err := runtime.Compiler.Compile(ctx, compiler.CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text:    "tracked",
			Anchors: []retrieval.AnchorRef{{ChunkID: string(fileID)}},
		},
		MaxTokens: 1000,
	})
	require.NoError(t, err)
	require.Contains(t, compiled.SkippedStaleChunks, fileID, "stale chunk must be excluded as a gap")

	// NFR-4: the grounding/stream/epoch paths leave no goroutines behind.
	testhelper.NoGoroutineLeak(t, func() {
		for i := 0; i < 10; i++ {
			runEnv := contextdata.NewEnvelope(fmt.Sprintf("task-leak-%d", i), "session-leak")
			if _, err := graph.Execute(ctx, runEnv); err != nil {
				t.Fatalf("run %d failed: %v", i, err)
			}
		}
	})
}
