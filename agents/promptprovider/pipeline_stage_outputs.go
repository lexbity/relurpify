package promptprovider

import (
	"strings"

	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type pipelineStageOutputsProvider struct{}

var pipelineStages = []string{
	"pipeline.explore",
	"pipeline.analyze",
	"pipeline.plan",
	"pipeline.code",
	"pipeline.verify",
}

func (pipelineStageOutputsProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{}
	}
	var sections []string
	for _, key := range pipelineStages {
		s := envelopeGetString(ctx.Envelope, key)
		if s == "" {
			continue
		}
		stage := strings.TrimPrefix(key, "pipeline.")
		sections = append(sections, "["+stage+"]\n"+strings.TrimSpace(s))
	}
	if len(sections) == 0 {
		return prompt.ContextChunk{}
	}
	return prompt.ContextChunk{Content: strings.Join(sections, "\n\n")}
}

func (pipelineStageOutputsProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "pipeline.stage_outputs",
		Description: "Supplies pipeline stage outputs (explore, analyze, plan, code, verify) from the envelope.",
		Paradigms:   []string{"pipeline"},
		ReadsKeys:   pipelineStages,
	}
}
