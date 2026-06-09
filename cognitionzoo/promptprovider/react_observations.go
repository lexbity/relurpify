package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type reactObservationsProvider struct{}

func (reactObservationsProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{}
	}
	raw, ok := envelopeGet(ctx.Envelope, "react.tool_observations")
	if !ok || raw == nil {
		return prompt.ContextChunk{}
	}
	items := toSliceOfAny(raw)
	if len(items) == 0 {
		return prompt.ContextChunk{}
	}
	var lines []string
	for i, item := range items {
		toolName := extractStringField(item, "ToolName")
		if toolName == "" {
			toolName = extractStringField(item, "tool_name")
		}
		summary := extractStringField(item, "Summary")
		if summary == "" {
			summary = extractStringField(item, "summary")
		}
		if summary == "" {
			summary = extractStringField(item, "Output")
		}
		if summary == "" {
			summary = extractStringField(item, "output")
		}
		if summary == "" {
			continue
		}
		line := fmt.Sprintf("%d.", i+1)
		if toolName != "" {
			line += " [" + toolName + "]"
		}
		line += " " + truncate(summary, 300)
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return prompt.ContextChunk{}
	}
	return prompt.ContextChunk{Content: strings.Join(lines, "\n")}
}

func (reactObservationsProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.observations",
		Description: "Supplies tool observation summaries from react.tool_observations in the envelope.",
		Paradigms:   []string{"react"},
		ReadsKeys:   []string{"react.tool_observations"},
	}
}
