package promptprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

func TestReactDeclarativeMemoryRendersPayloadResults(t *testing.T) {
	provider := reactDeclarativeMemoryProvider{}
	env := newEnvelope(t, "graph.declarative_memory_payload", map[string]any{
		"results": []any{
			map[string]any{"summary": "first finding", "source": "index"},
			map[string]any{"text": "second finding"},
			map[string]any{"summary": ""},
			map[string]any{"summary": nil},
		},
	})
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "- first finding [index]\n- second finding", out.Content)
}

func TestReactDeclarativeMemoryFallsThroughEmptyPayloadToRefs(t *testing.T) {
	provider := reactDeclarativeMemoryProvider{}
	env := newEnvelope(t,
		"graph.declarative_memory_payload", map[string]any{"results": []any{}},
		"graph.declarative_memory_refs", []contextdata.ChunkReference{
			{ChunkID: "chunk-1", Source: "ranker-a", IsSummary: true},
			{ChunkID: "chunk-2"},
			{ChunkID: "", Source: "skipped"},
		},
	)
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "- Reference: chunk-1 (summary) [ranker-a]\n- Reference: chunk-2", out.Content)
}

func TestReactDeclarativeMemoryUsesGenericKeyAsLastResort(t *testing.T) {
	provider := reactDeclarativeMemoryProvider{}
	env := newEnvelope(t, "graph.declarative_memory", map[string]any{
		"results": []any{map[string]any{"summary": "generic result"}},
	})
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "- generic result", out.Content)
}

func TestReactDeclarativeMemoryMissingOrUnsupportedValuesYieldEmptyChunk(t *testing.T) {
	provider := reactDeclarativeMemoryProvider{}

	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}),
		"nil envelope must yield an empty chunk")
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: newEnvelope(t)}),
		"absent keys must yield an empty chunk")

	// Generic key holding a non-map value renders nothing.
	env := newEnvelope(t, "graph.declarative_memory", "just a string")
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: env}))

	// Payload key holding a non-map value falls through to nothing.
	env = newEnvelope(t, "graph.declarative_memory_payload", 42)
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: env}))

	// Refs key holding the wrong slice type falls through to nothing.
	env = newEnvelope(t, "graph.declarative_memory_refs", []string{"wrong"})
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: env}))
}

func TestReactHistoryPrefersCompressedHistory(t *testing.T) {
	provider := reactHistoryProvider{}
	env := newEnvelope(t,
		"react.history_compressed", []any{
			map[string]any{"Summary": "did step one"},
			map[string]any{"summary": ""},
			map[string]any{"summary": "did step two"},
		},
		"_interactions", []any{map[string]any{"role": "user", "content": "raw log line"}},
	)
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	// Numbering reflects the item's original position; entries without a
	// summary are skipped but still consume their slot.
	assert.Equal(t, "1. did step one\n3. did step two", out.Content)
}

func TestReactHistoryFallsBackToInteractions(t *testing.T) {
	provider := reactHistoryProvider{}
	env := newEnvelope(t, "_interactions", []any{
		map[string]any{"role": "assistant", "content": "  reply text  "},
		map[string]any{"text": "text-only entry"},
		map[string]any{"content": ""},
		"not a map",
		map[string]any{"role": "user", "content": "question"},
	})
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "assistant: reply text\nturn: text-only entry\nuser: question", out.Content)
}

func TestReactHistoryEmptyInputsYieldEmptyChunk(t *testing.T) {
	provider := reactHistoryProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: newEnvelope(t)}))

	// Compressed present but without summaries falls through to absent interactions.
	env := newEnvelope(t, "react.history_compressed", []any{map[string]any{"Summary": ""}})
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: env}))
}

func TestReactObservationsRendersNumberedSummaries(t *testing.T) {
	provider := reactObservationsProvider{}
	env := newEnvelope(t, "react.tool_observations", []any{
		map[string]any{"ToolName": "read", "Summary": "read main.go"},
		map[string]any{"tool_name": "edit", "output": "applied patch"},
		map[string]any{"Summary": ""},
		"skipped non-map",
	})
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "1. [read] read main.go\n2. [edit] applied patch", out.Content)
}

func TestReactObservationsMissingInputsYieldEmptyChunk(t *testing.T) {
	provider := reactObservationsProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))

	env := newEnvelope(t, "react.tool_observations", []any{})
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: env}))

	env = newEnvelope(t, "react.tool_observations", []any{map[string]any{"Summary": " "}})
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: env}),
		"items without usable summaries render nothing")
}

func TestReactObservationsTruncatesLongSummaries(t *testing.T) {
	provider := reactObservationsProvider{}
	env := newEnvelope(t, "react.tool_observations", []any{
		map[string]any{"Summary": "x"},
	})
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	require.Equal(t, "1. x", out.Content)

	long := make([]rune, 0, 400)
	for i := 0; i < 350; i++ {
		long = append(long, 'y')
	}
	env = newEnvelope(t, "react.tool_observations", []any{
		map[string]any{"Summary": string(long)},
	})
	out = provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "1. "+string(long[:300])+"…", out.Content)
}
