package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/prompt"
)

type reactHistoryProvider struct{}

func (reactHistoryProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{}
	}

	// Prefer compressed history if present.
	if raw, ok := envelopeGet(ctx.Envelope, "react.history_compressed"); ok && raw != nil {
		if s := renderCompressedHistory(raw); s != "" {
			return prompt.ContextChunk{Content: s}
		}
	}

	// Fall back to raw interaction log.
	if raw, ok := envelopeGet(ctx.Envelope, "_interactions"); ok && raw != nil {
		if s := renderInteractions(raw); s != "" {
			return prompt.ContextChunk{Content: s}
		}
	}

	return prompt.ContextChunk{}
}

func (reactHistoryProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.history",
		Description: "Supplies interaction history from react.history_compressed or _interactions in the envelope.",
		Paradigms:   []string{"react"},
		ReadsKeys:   []string{"react.history_compressed", "_interactions"},
	}
}

func renderCompressedHistory(raw any) string {
	items := toSliceOfAny(raw)
	if len(items) == 0 {
		return ""
	}
	var lines []string
	for i, item := range items {
		summary := extractStringField(item, "Summary")
		if summary == "" {
			summary = extractStringField(item, "summary")
		}
		if summary == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, truncate(summary, 400)))
	}
	return strings.Join(lines, "\n")
}

func renderInteractions(raw any) string {
	items := toSliceOfAny(raw)
	if len(items) == 0 {
		return ""
	}
	var lines []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := extractStringField(m, "role")
		content := extractStringField(m, "content")
		if content == "" {
			content = extractStringField(m, "text")
		}
		if content == "" {
			continue
		}
		prefix := "turn"
		if role != "" {
			prefix = role
		}
		lines = append(lines, prefix+": "+truncate(content, 300))
	}
	return strings.Join(lines, "\n")
}
