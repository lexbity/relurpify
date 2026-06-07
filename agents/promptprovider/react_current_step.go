package promptprovider

import (
	"encoding/json"
	"fmt"
	"strings"

	pl "codeburg.org/lexbit/relurpify/agents/plan"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type reactCurrentStepProvider struct{}

func (reactCurrentStepProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Task == nil || ctx.Task.Context == nil {
		return prompt.ContextChunk{}
	}
	raw, ok := ctx.Task.Context["current_step"]
	if !ok || raw == nil {
		return prompt.ContextChunk{}
	}
	// Prefer concrete type for clean JSON output.
	if step, ok := raw.(pl.PlanStep); ok {
		encoded, err := json.MarshalIndent(step, "", "  ")
		if err == nil {
			return prompt.ContextChunk{Content: string(encoded)}
		}
		if step.Description != "" {
			return prompt.ContextChunk{Content: step.Description}
		}
	}
	// Fallback: marshal whatever we have.
	encoded, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return prompt.ContextChunk{Content: strings.TrimSpace(fmt.Sprint(raw))}
	}
	return prompt.ContextChunk{Content: string(encoded)}
}

func (reactCurrentStepProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.current_step",
		Description: "Supplies the current plan step from task.Context[\"current_step\"].",
		Paradigms:   []string{"react"},
	}
}
