package compiler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"github.com/stretchr/testify/require"
)

func TestCompilationAuditor_ReportAndDigest(t *testing.T) {
	store := newCompilerTestStore(t)
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	chunk := knowledge.KnowledgeChunk{
		ID:          "chunk:alpha",
		WorkspaceID: "ws",
		Body:        knowledge.ChunkBody{Raw: "alpha content", Fields: map[string]any{"content": "alpha content"}},
		Freshness:   knowledge.FreshnessValid,
		Provenance:  knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: now},
	}
	_, err := store.Save(chunk)
	require.NoError(t, err)

	registry := retrieval.NewRankerRegistry()
	registry.Register(&staticRanker{name: "alpha", ids: []knowledge.ChunkID{chunk.ID}})
	retriever := retrieval.NewRetriever(registry, store)
	compiler := NewCompiler(retriever, nil, store)
	compiler.SetIDGenerator(func() string { return "req-1" })
	compiler.SetTimeFunc(func() time.Time { return now })
	result, record, err := compiler.Compile(context.Background(), CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "alpha"},
		MaxTokens: 64,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

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

	auditor := NewCompilationAuditor(compiler)
	report, err := auditor.Audit(context.Background(), record.RequestID)
	require.NoError(t, err)
	require.True(t, report.DigestMatch)
	require.Equal(t, []knowledge.ChunkID{chunk.ID}, report.LoadedChunkIDs)
	require.Contains(t, report.Text, "Compilation audit report")
	require.Contains(t, report.Text, string(chunk.ID))
	require.Contains(t, report.Text, "Digest verified: true")
}

func TestCompilerEmitsWarningOnPersistenceFailure(t *testing.T) {
	compiler := NewCompiler(nil, nil, nil)
	telemetry := &warningTelemetryCollector{}
	compiler.SetTelemetry(telemetry)

	_, _, err := compiler.Compile(context.Background(), CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "warn"},
		MaxTokens: 16,
	})
	require.NoError(t, err)
	require.Len(t, telemetry.events, 1)
	require.Equal(t, core.EventCompilerWarning, telemetry.events[0].Type)
	require.Equal(t, "compilation persistence failed", telemetry.events[0].Message)
}

type warningTelemetryCollector struct {
	events []core.Event
}

func (w *warningTelemetryCollector) Emit(event core.Event) {
	w.events = append(w.events, event)
}
