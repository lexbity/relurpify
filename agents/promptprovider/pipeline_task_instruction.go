package promptprovider

import (
	"strings"

	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type pipelineTaskInstructionProvider struct{}

func (pipelineTaskInstructionProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Task == nil {
		return prompt.ContextChunk{}
	}
	s := strings.TrimSpace(ctx.Task.Instruction)
	if s == "" {
		return prompt.ContextChunk{}
	}
	return prompt.ContextChunk{Content: s}
}

func (pipelineTaskInstructionProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "pipeline.task_instruction",
		Description: "Supplies the raw task instruction from ctx.Task.Instruction.",
		Paradigms:   []string{"pipeline", "react"},
	}
}
