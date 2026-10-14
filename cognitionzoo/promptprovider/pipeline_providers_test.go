package promptprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

func TestPipelineStageOutputsRendersStagesInCanonicalOrder(t *testing.T) {
	provider := pipelineStageOutputsProvider{}
	env := newEnvelope(t,
		"pipeline.code", "wrote feature",
		"pipeline.explore", "found entry points",
	)
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "[explore]\nfound entry points\n\n[code]\nwrote feature", out.Content,
		"stages must render in canonical order regardless of insertion order")
}

func TestPipelineStageOutputsTrimsAndSkipsEmptyStages(t *testing.T) {
	provider := pipelineStageOutputsProvider{}
	env := newEnvelope(t,
		"pipeline.plan", "  the plan  \n",
		"pipeline.analyze", "   ",
		"pipeline.verify", "",
	)
	out := provider.Provide(prompt.RuntimeContext{Envelope: env})
	assert.Equal(t, "[plan]\nthe plan", out.Content)
}

func TestPipelineStageOutputsMissingInputsYieldEmptyChunk(t *testing.T) {
	provider := pipelineStageOutputsProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Envelope: newEnvelope(t)}))
}

func TestPipelineTaskInstructionServesTrimmedInstruction(t *testing.T) {
	provider := pipelineTaskInstructionProvider{}
	out := provider.Provide(prompt.RuntimeContext{Task: &execution.Task{Instruction: "  fix the bug \n"}})
	assert.Equal(t, "fix the bug", out.Content)
}

func TestPipelineTaskInstructionMissingInputsYieldEmptyChunk(t *testing.T) {
	provider := pipelineTaskInstructionProvider{}
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{Task: &execution.Task{}}))
	assert.Equal(t, prompt.ContextChunk{}, provider.Provide(prompt.RuntimeContext{
		Task: &execution.Task{Instruction: " \t "},
	}))
}
