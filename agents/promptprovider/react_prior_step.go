package promptprovider

import (
	"codeburg.org/lexbit/relurpify/framework/prompt"
)

type reactPriorStepProvider struct{}

func (reactPriorStepProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	summary := envelopeGetString(ctx.Envelope, "architect.last_step_summary")
	if summary == "" {
		return prompt.ContextChunk{}
	}
	return prompt.ContextChunk{Content: summary}
}

func (reactPriorStepProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.prior_step_result",
		Description: "Supplies the previous step summary from architect.last_step_summary in the envelope.",
		Paradigms:   []string{"react"},
		ReadsKeys:   []string{"architect.last_step_summary"},
	}
}
