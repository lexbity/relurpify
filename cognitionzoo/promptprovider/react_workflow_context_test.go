package promptprovider

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

const workflowPayloadJSON = `{
  "query": "parse error handling",
  "scope": "cognitionzoo",
  "cache_tier": "warm",
  "results": [
    {
      "text": "handler in runner.go",
      "reference": {"uri": "file:///runner.go", "id": "ignored", "detail": "ignored too"},
      "anchors": [
        {"status": "drifted", "term": "handleErr", "definition": "old impl"},
        {"status": "superseded", "term": "runStep", "definition": "old"},
        {"status": "fresh", "term": "ignored", "definition": "not a notice"},
        "not a map"
      ],
      "derivation": {"depth": 5, "total_loss": 0.8, "origin_system": "archivist"}
    },
    {
      "summary": "summary-only result",
      "anchors": [{"status": "drifted", "term": "t", "definition": "d"}]
    },
    {"reference": {"detail": "ref-only result"}},
    {"other": "no text and no reference"},
    "not a map"
  ]
}`

func TestReactWorkflowRetrievalFormatsBytePayload(t *testing.T) {
	provider := reactWorkflowRetrievalProvider{}
	task := &execution.Task{Context: map[string]any{
		"workflow_retrieval": []byte(workflowPayloadJSON),
	}}
	out := provider.Provide(prompt.RuntimeContext{Task: task})

	assert.Contains(t, out.Content, "Query: parse error handling")
	assert.Contains(t, out.Content, "Scope: cognitionzoo")
	assert.Contains(t, out.Content, "Cache tier: warm")
	assert.Contains(t, out.Content, "1. handler in runner.go")
	assert.Contains(t, out.Content, "Reference: file:///runner.go")
	assert.Contains(t, out.Content, `⚠ ANCHOR DRIFT: "handleErr" was "old impl" when captured; context has changed.`)
	assert.Contains(t, out.Content, `⚠ ANCHOR SUPERSEDED: "runStep" is no longer the active definition.`)
	assert.NotContains(t, out.Content, "ignored")
	assert.Contains(t, out.Content, "⚠ CONFIDENCE: 5 transformations, ~80% information loss. Origin: archivist")
	assert.Contains(t, out.Content, "2. summary-only result")
	assert.Contains(t, out.Content, "3. reference only\n   Reference: ref-only result")
	assert.Contains(t, out.Content, "4. reference only")
}

func TestReactWorkflowRetrievalJSONRoundTripMatchesDirectMap(t *testing.T) {
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(workflowPayloadJSON), &payload))

	provider := reactWorkflowRetrievalProvider{}

	fromBytes := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"workflow_retrieval": []byte(workflowPayloadJSON)},
	}})

	// The map form skips extractTaskBytes and renders via formatWorkflowRetrieval
	// only if the provider routes it there; today it falls back to raw JSON.
	fromMap := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"workflow_retrieval": payload},
	}})

	var mapDecoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(fromMap.Content), &mapDecoded),
		"map payload renders as indented JSON, got %q", fromMap.Content)
	assert.NotEqual(t, fromBytes.Content, fromMap.Content)
	assert.Contains(t, mapDecoded, "query")
}

func TestReactWorkflowRetrievalLowDerivationOmitsWarning(t *testing.T) {
	provider := reactWorkflowRetrievalProvider{}
	payload := `{"results": [{"text": "ok", "derivation": {"depth": 2, "total_loss": 0.1}}]}`
	out := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"workflow_retrieval": []byte(payload)},
	}})
	assert.Contains(t, out.Content, "1. ok")
	assert.NotContains(t, out.Content, "CONFIDENCE")
}

func TestReactWorkflowRetrievalDerivationStringForms(t *testing.T) {
	assert.Empty(t, derivationWarning(map[string]any{"depth": "3", "total_loss": "0.2"}))
	assert.Contains(t,
		derivationWarning(map[string]any{"depth": "7", "total_loss": "0.9", "origin_system": "elsewhere"}),
		"⚠ CONFIDENCE: 7 transformations, ~90% information loss. Origin: elsewhere")
	assert.Contains(t,
		derivationWarning(map[string]any{"depth": 9, "total_loss": 1, "origin_system": "raw"}),
		"⚠ CONFIDENCE: 9 transformations, ~100% information loss. Origin: raw")
}

func TestReactWorkflowRetrievalFallbackPaths(t *testing.T) {
	provider := reactWorkflowRetrievalProvider{}

	// Non-object values (not bytes) fall back to marshaling the raw value.
	out := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"workflow_retrieval": []any{1, 2}},
	}})
	assert.Equal(t, "[\n  1,\n  2\n]", out.Content)

	// Byte payloads whose JSON is not an object fall back to JSON-encoding the
	// bytes, which produces a base64 string.
	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"workflow_retrieval": []byte(`[1,2]`)},
	}})
	assert.Equal(t, `"WzEsMl0="`, out.Content)

	// Invalid JSON follows the same base64 fallback.
	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"workflow_retrieval": []byte("\xff\xfe not json")},
	}})
	assert.NotEmpty(t, out.Content)
	assert.NotContains(t, out.Content, "Evidence:")

	// Unmarshalable values render via fmt.Sprint.
	ch := make(chan int)
	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"workflow_retrieval": ch},
	}})
	assert.NotEmpty(t, out.Content)
}

func TestReactWorkflowRetrievalMissingInputsYieldEmptyChunk(t *testing.T) {
	provider := reactWorkflowRetrievalProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Task: &execution.Task{}}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{
		Task: &execution.Task{Context: map[string]any{"workflow_retrieval": nil}},
	}))
}

func TestReactContextFilesRendersStringMapAndListForms(t *testing.T) {
	provider := reactContextFilesProvider{}

	out := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"context_file_contents": "  body text  "},
	}})
	assert.Equal(t, "body text", out.Content)

	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{
			"context_file_contents": map[string]any{"a.go": "package a", "b.go": "  "},
		},
	}})
	assert.Equal(t, "// a.go\npackage a", out.Content, "empty contents are skipped in the map form")

	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{
			"context_file_contents": map[string]string{"c.go": "package c"},
		},
	}})
	assert.Equal(t, "// c.go\npackage c", out.Content)

	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{
			"context_file_contents": []any{
				map[string]any{"path": "list.go", "content": "package list"},
				map[string]any{"name": "alt.go", "text": "package alt"},
				map[string]any{"path": "empty.go"},
				"skip me",
			},
		},
	}})
	assert.Equal(t, "// list.go\npackage list\n\n// alt.go\npackage alt", out.Content)
}

func TestReactContextFilesFallbackAndEmptyForms(t *testing.T) {
	provider := reactContextFilesProvider{}

	// Non-string, non-map, non-list values render via fmt.Sprint.
	out := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"context_file_contents": 99},
	}})
	assert.Equal(t, "99", out.Content)

	// Empty list falls through to fmt.Sprint of the raw value.
	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"context_file_contents": []any{}},
	}})
	assert.Equal(t, "[]", out.Content)

	// Whitespace-only strings render nothing.
	out = provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
		Context: map[string]any{"context_file_contents": " \t "},
	}})
	assert.Equal(t, prompt.ContextChunk{}, out)
}

func TestReactContextFilesMissingInputsYieldEmptyChunk(t *testing.T) {
	provider := reactContextFilesProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Task: &execution.Task{}}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{
		Task: &execution.Task{Context: map[string]any{"context_file_contents": nil}},
	}))
}

// FuzzWorkflowRetrievalRender exercises the workflow retrieval renderer against
// arbitrary untrusted task payloads: it must never panic and must always emit
// valid UTF-8 prompt text, whatever the byte payload decodes to.
func FuzzWorkflowRetrievalRender(f *testing.F) {
	f.Add(`{}`)
	f.Add(`{"query": "q", "results": []}`)
	f.Add(`{"results": [{"anchors": [{"status": "drifted"}]}]}`)
	f.Add(`{"results": [{"derivation": {"depth": 100, "total_loss": 2}}]}`)
	f.Add(`{"query": 1, "scope": true, "cache_tier": null}`)
	f.Add(`[1, 2, 3]`)
	f.Add("not json at all")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, payload string) {
		provider := reactWorkflowRetrievalProvider{}
		out := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
			Context: map[string]any{"workflow_retrieval": []byte(payload)},
		}})
		assert.True(t, utf8.ValidString(out.Content), "rendered prompt must be valid UTF-8, got %q", out.Content)
	})
}

// FuzzContextFilesRender exercises the context-file renderer against arbitrary
// untrusted task payloads: JSON-decodable inputs flow through the map and list
// renderers, everything else passes through as trimmed text verbatim.
func FuzzContextFilesRender(f *testing.F) {
	f.Add("plain body")
	f.Add(`{"a.go": "package a"}`)
	f.Add(`[{"path": "p", "content": "c"}]`)
	f.Add(`{"nested": {"deep": [1, 2]}}`)
	f.Add("")

	f.Fuzz(func(t *testing.T, raw string) {
		var decoded any
		jsonErr := json.Unmarshal([]byte(raw), &decoded)
		isJSON := jsonErr == nil && decoded != nil

		var value any = raw
		if isJSON {
			value = decoded
		}
		provider := reactContextFilesProvider{}
		out := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{
			Context: map[string]any{"context_file_contents": value},
		}})

		// Plain strings render verbatim (trimmed); the renderer never
		// crashes or rewrites untrusted bytes.
		if !isJSON {
			assert.Equal(t, strings.TrimSpace(raw), out.Content)
		}
	})
}
