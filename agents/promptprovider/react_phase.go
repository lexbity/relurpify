package promptprovider

import (
	"codeburg.org/lexbit/relurpify/framework/prompt"
)

type reactPhaseProvider struct{}

func (reactPhaseProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	phase := envelopeGetString(ctx.Envelope, "react.phase")
	if phase == "" {
		return prompt.ContextChunk{}
	}
	return prompt.ContextChunk{Content: "Execution phase: " + phase}
}

func (reactPhaseProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.phase",
		Description: "Supplies the current react execution phase (explore/edit/verify) from the envelope.",
		Paradigms:   []string{"react"},
		ReadsKeys:   []string{"react.phase"},
	}
}
