package stages

import (
	"codeburg.org/lexbit/relurpify/agents/chainer"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// SummarizeStage is a convenience wrapper for a summarization link.
// It prompts: "Summarize the available input." and stores raw text output.
//
// Use it for generic summarization steps without custom parsing.
func SummarizeStage(name string, inputKeys []string, outputKey string, model core.LanguageModel) *LinkStage {
	link := chainer.NewSummarizeLink(name, inputKeys, outputKey)
	return NewLinkStage(&link, model)
}

// TransformStage is a convenience wrapper for a transformation link with custom parsing.
// It prompts: "Transform the available input." and applies the provided parser.
//
// Use it for generic transformation steps that parse the LLM output.
func TransformStage(
	name string,
	inputKeys []string,
	outputKey string,
	parseFunc chainer.ParseFunc,
	model core.LanguageModel,
) *LinkStage {
	link := chainer.NewTransformLink(name, inputKeys, outputKey, parseFunc)
	return NewLinkStage(&link, model)
}
