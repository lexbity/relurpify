package testsuite

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/execution/compiler"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type phase9Telemetry struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (t *phase9Telemetry) Emit(event telemetry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *phase9Telemetry) Snapshot() []telemetry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]telemetry.Event, len(t.events))
	copy(out, t.events)
	return out
}

// phase9EventSink adapts phase9Telemetry into the ctx-carrying
// model.EventSink the instrumented model consumes. With one Event struct
// there is nothing to copy — the adapter only carries the ctx through.
type phase9EventSink struct {
	parent *phase9Telemetry
}

func (o *phase9EventSink) Emit(_ context.Context, event any) {
	ev, ok := event.(telemetry.Event)
	if !ok {
		return
	}
	o.parent.Emit(ev)
}

type phase9UsageModel struct{}

func (phase9UsageModel) Generate(context.Context, string, *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        model.TokenUsage{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

func (phase9UsageModel) GenerateStream(context.Context, string, *llm.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (phase9UsageModel) Chat(context.Context, []llm.Message, *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        model.TokenUsage{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

func (phase9UsageModel) ChatWithTools(context.Context, []llm.Message, []llm.LLMToolSpec, *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        model.TokenUsage{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

type phase9StaticRanker struct {
	name string
	ids  []knowledge.ChunkID
}

func (r *phase9StaticRanker) Name() string { return r.name }

func (r *phase9StaticRanker) Rank(context.Context, retrieval.RetrievalQuery, *retrieval.CorpusSnapshot) ([]knowledge.ChunkID, error) {
	return append([]knowledge.ChunkID(nil), r.ids...), nil
}

func TestCompilationReplay_Determinism(t *testing.T) {
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(context.Background())) })

	store := &knowledge.ChunkStore{Graph: engine}
	now := time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)
	source := knowledge.KnowledgeChunk{
		ID:          "chunk:source",
		WorkspaceID: "ws",
		Body:        knowledge.ChunkBody{Raw: "source", Fields: map[string]any{"content": "source"}},
		Freshness:   knowledge.FreshnessValid,
		Provenance:  knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: now},
	}
	_, err = store.Save(context.TODO(), source)
	require.NoError(t, err)

	registry := retrieval.NewRankerRegistry()
	registry.Register(&phase9StaticRanker{name: "source", ids: []knowledge.ChunkID{source.ID}})
	retriever := retrieval.NewRetriever(registry, store)
	comp := compiler.NewCompiler(retriever, nil, store)
	var seq int
	comp.SetIDGenerator(func() string {
		seq++
		return fmt.Sprintf("id-%d", seq)
	})
	comp.SetTimeFunc(func() time.Time { return now })

	// Records persist through the O(1) graph repository keyed by request ID.
	repo := compiler.NewCompilerRepository(engine)
	comp.SetRepository(repo)

	result, record, err := comp.Compile(context.Background(), compiler.CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "source"},
		MaxTokens: 32,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.RankedChunks, 1)

	replayComp := compiler.NewCompiler(retriever, nil, store)
	replayComp.SetIDGenerator(func() string {
		seq++
		return fmt.Sprintf("id-%d", seq)
	})
	replayComp.SetTimeFunc(func() time.Time { return now })
	replayComp.SetRepository(repo)

	replayed, replayRecord, diff, err := replayComp.Replay(context.Background(), record.RequestID, compiler.StrictReplay)
	require.NoError(t, err)
	require.NotNil(t, replayed)
	require.NotNil(t, replayRecord)
	require.NotNil(t, diff)
	require.True(t, diff.DeterminismMatch)
	require.Equal(t, record.DeterministicDigest, replayRecord.DeterministicDigest)
	require.Equal(t, result.RankedChunks, replayed.RankedChunks)
}

func TestProvenance_FullChain(t *testing.T) {
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(context.Background())) })

	store := &knowledge.ChunkStore{Graph: engine}
	now := time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)
	fileChunk := knowledge.KnowledgeChunk{
		ID:           "chunk:file",
		WorkspaceID:  "ws",
		SourceOrigin: knowledge.SourceOriginFile,
		Body:         knowledge.ChunkBody{Raw: "file content", Fields: map[string]any{"content": "file content", "file_path": "src/file.go"}},
		Freshness:    knowledge.FreshnessValid,
		Provenance:   knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: now},
	}
	_, err = store.Save(context.TODO(), fileChunk)
	require.NoError(t, err)

	summaryChunk := knowledge.KnowledgeChunk{
		ID:               "chunk:summary",
		WorkspaceID:      "ws",
		SourceOrigin:     knowledge.SourceOriginDerivation,
		DerivedFrom:      []knowledge.ChunkID{fileChunk.ID},
		DerivationMethod: knowledge.DerivationMethodSummary,
		Body:             knowledge.ChunkBody{Raw: "summary content", Fields: map[string]any{"content": "summary content"}},
		Freshness:        knowledge.FreshnessValid,
		Provenance: knowledge.ChunkProvenance{
			Sources:    []knowledge.ProvenanceSource{{Kind: "chunk", Ref: string(fileChunk.ID)}},
			CompiledBy: knowledge.CompilerDeterministic,
			Timestamp:  now,
		},
	}
	_, err = store.Save(context.TODO(), summaryChunk)
	require.NoError(t, err)
	_, err = store.SaveEdge(context.TODO(), knowledge.ChunkEdge{FromChunk: fileChunk.ID, ToChunk: summaryChunk.ID, Kind: knowledge.EdgeKindDerivesFrom, Weight: 1})
	require.NoError(t, err)

	// The provenance proof lives on the boundary that owns it: a capture
	// grounded through the GroundingService carries DerivedFrom provenance to
	// the streamed-context chunk it was derived from (D-5 migration; the
	// orphaned response-ingest path is gone).
	grounding := knowledge.NewGroundingService(store, &knowledge.EventBus{}, nil, nil)
	report, err := grounding.Ground(context.Background(), []knowledge.GroundingItem{{
		Value:         "grounded capture derived from the summary chunk",
		Epistemics:    knowledge.EpistemicClaimed,
		Origin:        contextdata.OriginTool,
		StateKey:      "state.findings",
		NodeID:        "node-capture",
		TaskID:        "task-1",
		Kind:          knowledge.ChunkKindCapture,
		ForwardedFrom: []knowledge.ChunkID{summaryChunk.ID},
	}})
	require.NoError(t, err)
	require.Len(t, report.Grounded, 1)
	savedID := report.Grounded[0].ChunkID
	saved, ok, err := store.Load(savedID)
	require.NoError(t, err)
	require.True(t, ok, "grounded chunk %s must be loadable", savedID)
	require.Equal(t, []knowledge.ChunkID{summaryChunk.ID}, saved.DerivedFrom)

	edges, err := store.LoadEdgesFrom(saved.ID, knowledge.EdgeKindDerivesFrom)
	require.NoError(t, err)
	foundSummary := false
	for _, edge := range edges {
		if edge.ToChunk == summaryChunk.ID {
			foundSummary = true
		}
	}
	require.True(t, foundSummary, "grounded capture must carry a derives_from edge to its source chunk")

	// And the chain closes to the file chunk through the summary's own
	// DerivedFrom provenance.
	loadedSummary, ok, err := store.Load(summaryChunk.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []knowledge.ChunkID{fileChunk.ID}, loadedSummary.DerivedFrom)
}

func TestBudgetExhaustion_ResetProtocol(t *testing.T) {
	advisor := &telemetry.ContextBudgetAdvisor{ModelContextSize: 2048}
	tel := &phase9Telemetry{}
	model := llm.NewInstrumentedModel(phase9UsageModel{}, &phase9EventSink{parent: tel}, false)
	ctx := telemetry.WithAdvisor(context.Background(), advisor)

	_, err := model.Chat(ctx, []llm.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		for _, event := range tel.Snapshot() {
			if event.Type == telemetry.EventSessionResetRequired {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)

	snapshot := advisor.Snapshot()
	require.True(t, snapshot.ShouldReset)
	advisor.Reset()
	require.False(t, advisor.ShouldReset())

	_, err = model.Chat(ctx, []llm.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		count := 0
		for _, event := range tel.Snapshot() {
			if event.Type == telemetry.EventSessionResetRequired {
				count++
			}
		}
		return count >= 2
	}, time.Second, 10*time.Millisecond)
}
