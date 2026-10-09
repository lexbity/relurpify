package compiler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// warningCollector captures compiler warnings.
type warningCollector struct {
	warnings []telemetry.Event
}

func (w *warningCollector) Emit(ev telemetry.Event) { w.warnings = append(w.warnings, ev) }

func (w *warningCollector) count(kind string) int {
	n := 0
	for _, ev := range w.warnings {
		if ev.Message == kind {
			n++
		}
	}
	return n
}

// TestEstimatorUsesRawBody: Raw-only chunks (no typed content field) are
// estimated from Body.Raw — previously they estimated as the literal
// "<nil>" (one token), silently voiding the budget for tool-ingested
// content.
func TestEstimatorUsesRawBody(t *testing.T) {
	store := newCompilerTestStore(t)
	c := NewCompiler(nil, nil, store)

	_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{
		ID:   "chunk:raw",
		Body: knowledge.ChunkBody{Raw: strings.Repeat("x", 400)},
	})
	require.NoError(t, err)

	if got := c.estimateChunkTokens("chunk:raw"); got != 100 {
		t.Fatalf("Raw-body chunk estimated at %d tokens, want 100", got)
	}
}

// TestEstimatorExcludesEmptyContent: a chunk with no estimable content
// estimates zero and is excluded from the budget with a gap message.
func TestEstimatorExcludesEmptyContent(t *testing.T) {
	store := newCompilerTestStore(t)
	c := NewCompiler(nil, nil, store)
	warnings := &warningCollector{}
	c.SetTelemetry(warnings)

	_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{
		ID:         "chunk:empty",
		TrustClass: "builtin.trusted",
		Body:       knowledge.ChunkBody{},
	})
	require.NoError(t, err)

	require.Equal(t, 0, c.estimateChunkTokens("chunk:empty"))

	ranked := []retrieval.RankedChunk{{ChunkID: "chunk:empty"}}
	result, _ := c.applyBudget(context.Background(), ranked, 64)
	require.Empty(t, result, "empty-content chunk must be excluded")
	require.Equal(t, 1, warnings.count("content_gap"))
}

// TestEstimatorCeilsPerFourChars: estimation is ceil(chars/4), minimum 1.
func TestEstimatorCeilsPerFourChars(t *testing.T) {
	store := newCompilerTestStore(t)
	c := NewCompiler(nil, nil, store)
	for _, tc := range []struct {
		content string
		want    int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"abcdefgh", 2},
	} {
		id := knowledge.ChunkID("chunk:ceil:" + string(rune('a'+len(tc.content))))
		_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{
			ID:   id,
			Body: knowledge.ChunkBody{Raw: tc.content, Fields: map[string]any{"content": tc.content}},
		})
		require.NoError(t, err)
		require.Equal(t, tc.want, c.estimateChunkTokens(id), "content %q", tc.content)
	}
}

// TestPinReservedBudgetClamps: pins keep their floor; content fits into
// max(0, maxTokens-reserved); pins alone exceeding the budget win with a
// counted overflow (content+pins never silently exceed maxTokens).
func TestPinReservedBudgetClamps(t *testing.T) {
	// Two pins (128 reserved) inside a 256 budget leaves 128 for content.
	content, overflow := applyPinReservedBudget(256, 2)
	require.Equal(t, 128, content)
	require.Equal(t, 0, overflow)

	// Pins alone exceed the budget: content gets zero, overflow counts the
	// excess (previously content kept the full budget, double-spending it).
	content, overflow = applyPinReservedBudget(100, 2)
	require.Equal(t, 0, content)
	require.Equal(t, 28, overflow)

	content, overflow = applyPinReservedBudget(128, 2)
	require.Equal(t, 0, content)
	require.Equal(t, 0, overflow)
}

// TestUnlimitedBudgetOptInIsLogged: maxTokens <= 0 maps to the explicit
// UnlimitedBudget constant and the opt-in is logged, never silent.
func TestUnlimitedBudgetOptInIsLogged(t *testing.T) {
	store := newCompilerTestStore(t)
	c := NewCompiler(nil, nil, store)
	warnings := &warningCollector{}
	c.SetTelemetry(warnings)

	now := time.Unix(1000, 0)
	c.SetTimeFunc(func() time.Time { return now })
	c.SetIDGenerator(func() string { return "unlimited" })

	// No chunks: compile with zero budget must still complete and admit
	// ranked order without dropping anything for budget reasons.
	_, record, err := c.Compile(context.Background(), CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "unlimited"},
		MaxTokens: 0,
	})
	require.NoError(t, err)
	require.Equal(t, UnlimitedBudget, 0)
	require.Equal(t, 1, warnings.count("unlimited_budget"))
	require.Equal(t, 0, record.BudgetShortfall)
}

// TestPinOverflowEmitsWarning: pins exceeding the budget emit the
// pin_reserved_overflow warning at request assembly.
func TestPinOverflowEmitsWarning(t *testing.T) {
	store := newCompilerTestStore(t)
	c := NewCompiler(nil, nil, store)
	warnings := &warningCollector{}
	c.SetTelemetry(warnings)
	now := time.Unix(1000, 0)
	c.SetTimeFunc(func() time.Time { return now })
	c.SetIDGenerator(func() string { return "pins" })

	// MaxActivePins worth of pin anchors on a budget smaller than the pin
	// floor: content budget is zero and the overflow is counted.
	req := CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text: "pins",
			Anchors: []retrieval.AnchorRef{
				{AnchorID: "pin:/tmp/one", Class: "session_pin"},
				{AnchorID: "pin:/tmp/two", Class: "session_pin"},
			},
		},
		MaxTokens: 64,
	}
	_, _, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, 1, warnings.count("pin_reserved_overflow"))
}

// BenchmarkCompileHot measures the hot cache path (compile repeated with the
// same logical request). The NFR-4 regression budget: p99 within 10% of the
// pre-slice baseline; the bounded cache must not tax the hit path.
func BenchmarkCompileHot(b *testing.B) {
	store := newBenchStore(b)
	registry := retrieval.NewRankerRegistry()
	registry.Register(&staticRanker{name: "bench", ids: []knowledge.ChunkID{"chunk:bench"}})
	c := NewCompiler(retrieval.NewRetriever(registry, store), nil, store)
	req := CompilationRequest{Query: retrieval.RetrievalQuery{Text: "bench"}, MaxTokens: 64}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := c.Compile(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}

func newBenchStore(b *testing.B) *knowledge.ChunkStore {
	b.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(b.TempDir()))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = engine.Close(context.Background()) })
	store := &knowledge.ChunkStore{Graph: engine}
	content := strings.Repeat("x", 200)
	if _, err := store.Save(context.Background(), knowledge.KnowledgeChunk{
		ID:         "chunk:bench",
		TrustClass: "builtin.trusted",
		Body:       knowledge.ChunkBody{Raw: content, Fields: map[string]any{"content": content}},
	}); err != nil {
		b.Fatal(err)
	}
	return store
}
