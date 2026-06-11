package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type reactDeclarativeMemoryProvider struct{}

func (reactDeclarativeMemoryProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{}
	}
	content := renderDeclarativeMemory(ctx.Envelope)
	if content == "" {
		return prompt.ContextChunk{}
	}
	return prompt.ContextChunk{Content: content}
}

func (reactDeclarativeMemoryProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.declarative_memory",
		Description: "Supplies relevant memory from graph.declarative_memory* envelope keys.",
		Paradigms:   []string{"react"},
		ReadsKeys:   []string{"graph.declarative_memory", "graph.declarative_memory_payload", "graph.declarative_memory_refs"},
	}
}

// renderDeclarativeMemory assembles memory content from envelope, checking
// payload, refs, and raw forms in order.
func renderDeclarativeMemory(env *contextdata.Envelope) string {
	// Check payload form first (structured map with results[]).
	if raw, ok := envelopeGet(env, "graph.declarative_memory_payload"); ok && raw != nil {
		if payload, ok := raw.(map[string]any); ok {
			if s := renderMemoryPayload(payload); s != "" {
				return s
			}
		}
	}

	// Check refs form.
	if raw, ok := envelopeGet(env, "graph.declarative_memory_refs"); ok && raw != nil {
		if refs, ok := raw.([]contextdata.ChunkReference); ok {
			if s := renderChunkRefs(refs); s != "" {
				return s
			}
		}
	}

	// Check generic declarative_memory key.
	raw, ok := envelopeGet(env, "graph.declarative_memory")
	if !ok || raw == nil {
		return ""
	}
	if payload, ok := raw.(map[string]any); ok {
		return renderMemoryPayload(payload)
	}
	return ""
}

// renderMemoryPayload formats a map with a "results" key containing summaries.
func renderMemoryPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	items := toSliceOfAny(payload["results"])
	if len(items) == 0 {
		return ""
	}
	var parts []string
	for _, item := range items {
		summary := extractStringField(item, "summary")
		if summary == "" {
			summary = extractStringField(item, "text")
		}
		if summary == "" || summary == "<nil>" {
			continue
		}
		line := "- " + summary
		if src := extractStringField(item, "source"); src != "" && src != "<nil>" {
			line += " [" + src + "]"
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, "\n")
}

// renderChunkRefs formats a slice of chunk references.
func renderChunkRefs(refs []contextdata.ChunkReference) string {
	var lines []string
	for _, ref := range refs {
		label := strings.TrimSpace(string(ref.ChunkID))
		if label == "" {
			continue
		}
		line := "- Reference: " + label
		if ref.IsSummary {
			line += " (summary)"
		}
		if ref.Source != "" {
			line += " [" + strings.TrimSpace(ref.Source) + "]"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}


// Ensure unused import doesn't slip through — used in renderChunkRefs.
var _ = fmt.Sprintf
