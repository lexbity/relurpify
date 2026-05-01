package testsuite

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextbudget"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"github.com/stretchr/testify/require"
)

type phase9Telemetry struct {
	mu     sync.Mutex
	events []contracts.Event
}

func (t *phase9Telemetry) Emit(event contracts.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *phase9Telemetry) Snapshot() []contracts.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]contracts.Event, len(t.events))
	copy(out, t.events)
	return out
}

type phase9UsageModel struct{}

func (phase9UsageModel) Generate(context.Context, string, *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        contracts.TokenUsageReport{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
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
		Usage:        contracts.TokenUsageReport{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

func (phase9UsageModel) ChatWithTools(context.Context, []llm.Message, []llm.LLMToolSpec, *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{
		Text:         "hello",
		FinishReason: "stop",
		Usage:        contracts.TokenUsageReport{PromptTokens: 600, CompletionTokens: 10, TotalTokens: 610},
	}, nil
}

type phase9StaticRanker struct {
	name string
	ids  []knowledge.ChunkID
}

func (r *phase9StaticRanker) Name() string { return r.name }

func (r *phase9StaticRanker) Rank(context.Context, retrieval.RetrievalQuery, *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	return append([]knowledge.ChunkID(nil), r.ids...), nil
}

func TestCompilationReplay_Determinism(t *testing.T) {
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })

	store := &knowledge.ChunkStore{Graph: engine}
	now := time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)
	source := knowledge.KnowledgeChunk{
		ID:          "chunk:source",
		WorkspaceID: "ws",
		Body:        knowledge.ChunkBody{Raw: "source", Fields: map[string]any{"content": "source"}},
		Freshness:   knowledge.FreshnessValid,
		Provenance:  knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: now},
	}
	_, err = store.Save(source)
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

	result, record, err := comp.Compile(context.Background(), compiler.CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "source"},
		MaxTokens: 32,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.RankedChunks, 1)

	data, err := json.Marshal(record)
	require.NoError(t, err)
	_, err = store.Save(knowledge.KnowledgeChunk{
		ID:           knowledge.ChunkID(record.RequestID),
		SourceOrigin: knowledge.SourceOrigin("compilation_record"),
		Body:         knowledge.ChunkBody{Raw: string(data), Fields: map[string]any{"content": string(data)}},
		Freshness:    knowledge.FreshnessValid,
		Provenance:   knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: now},
	})
	require.NoError(t, err)

	replayComp := compiler.NewCompiler(retriever, nil, store)
	replayComp.SetIDGenerator(func() string {
		seq++
		return fmt.Sprintf("id-%d", seq)
	})
	replayComp.SetTimeFunc(func() time.Time { return now })

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
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })

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
	_, err = store.Save(fileChunk)
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
	_, err = store.Save(summaryChunk)
	require.NoError(t, err)
	_, err = store.SaveEdge(knowledge.ChunkEdge{FromChunk: fileChunk.ID, ToChunk: summaryChunk.ID, Kind: knowledge.EdgeKindDerivesFrom, Weight: 1})
	require.NoError(t, err)

	ing := knowledge.NewOutputIngester(store, nil)
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.AddStreamedContextReference(contextdata.ChunkReference{ChunkID: contextdata.ChunkID(summaryChunk.ID), Source: "test", Rank: 1})
	ctx := knowledge.WithOutputIngester(contextdata.WithEnvelope(context.Background(), env), ing)
	saved, err := ing.IngestLLMResponseFull(ctx, &contracts.LLMResponse{Text: "grounded response", FinishReason: "stop"})
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, []knowledge.ChunkID{summaryChunk.ID}, saved.DerivedFrom)

	loadedSummary, ok, err := store.Load(summaryChunk.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []knowledge.ChunkID{fileChunk.ID}, loadedSummary.DerivedFrom)

	loadedResponse, ok, err := store.Load(saved.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, summaryChunk.ID, loadedResponse.DerivedFrom[0])
}

func TestBudgetExhaustion_ResetProtocol(t *testing.T) {
	advisor := &contextbudget.ContextBudgetAdvisor{ModelContextSize: 2048}
	telemetry := &phase9Telemetry{}
	model := llm.NewInstrumentedModel(phase9UsageModel{}, telemetry, false)
	ctx := contextbudget.WithAdvisor(context.Background(), advisor)
	ctx = contextbudget.WithSnapshotEmitter(ctx, contextbudget.NewSnapshotEmitter(advisor, telemetry, 1))

	_, err := model.Chat(ctx, []llm.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		for _, event := range telemetry.Snapshot() {
			if event.Type == contracts.EventSessionResetRequired {
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
		for _, event := range telemetry.Snapshot() {
			if event.Type == contracts.EventSessionResetRequired {
				count++
			}
		}
		return count >= 2
	}, time.Second, 10*time.Millisecond)
}
