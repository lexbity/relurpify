package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type reactExternalStateProvider struct{}

func (reactExternalStateProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Task == nil || ctx.Task.Context == nil {
		return prompt.ContextChunk{}
	}
	raw, ok := ctx.Task.Context["external_state_slice"]
	if !ok || raw == nil {
		return prompt.ContextChunk{}
	}
	var content string
	switch v := raw.(type) {
	case string:
		content = strings.TrimSpace(v)
	default:
		content = strings.TrimSpace(fmt.Sprint(raw))
	}
	if content == "" {
		return prompt.ContextChunk{}
	}
	return prompt.ContextChunk{Content: content}
}

func (reactExternalStateProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.external_state",
		Description: "Supplies the external state slice from task.Context[\"external_state_slice\"].",
		Paradigms:   []string{"react"},
	}
}
