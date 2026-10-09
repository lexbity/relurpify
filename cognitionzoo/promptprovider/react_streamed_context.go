package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type reactStreamedContextProvider struct{}

func (reactStreamedContextProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{}
	}
	streamed := ctx.Envelope.ReferencesSnapshot().StreamedContext
	if len(streamed) == 0 {
		return prompt.ContextChunk{}
	}
	var lines []string
	for _, ref := range streamed {
		chunkID := strings.TrimSpace(string(ref.ChunkID))
		if chunkID == "" {
			continue
		}
		line := "- " + chunkID
		if ref.Source != "" {
			line += " [" + strings.TrimSpace(ref.Source) + "]"
		}
		if ref.Rank > 0 {
			line += fmt.Sprintf(" rank=%d", ref.Rank)
		}
		if ref.IsSummary {
			line += " (summary)"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return prompt.ContextChunk{}
	}
	return prompt.ContextChunk{Content: strings.Join(lines, "\n")}
}

func (reactStreamedContextProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.streamed_context",
		Description: "Supplies streamed context chunk references from the envelope.",
		Paradigms:   []string{"react"},
	}
}
