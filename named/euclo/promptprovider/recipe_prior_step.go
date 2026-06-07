package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// thoughtrecipePriorStepProvider provides context from the last executed thoughtrecipe step.
type thoughtrecipePriorStepProvider struct{}

func (p *thoughtrecipePriorStepProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{Content: ""}
	}

	sv := surface.StateView{}

	if frameHistory, hasHistory := state.GetFrameHistory(ctx.Envelope); hasHistory && len(frameHistory) > 0 {
		lastFrame := frameHistory[len(frameHistory)-1]
		sv.PriorStepSummaryLines = append(sv.PriorStepSummaryLines, fmt.Sprintf("Last Frame: %s", lastFrame))
	}

	if jobRecords, hasRecords := state.GetJobRecords(ctx.Envelope); hasRecords && len(jobRecords) > 0 {
		lastJob := jobRecords[len(jobRecords)-1]
		sv.PriorStepSummaryLines = append(sv.PriorStepSummaryLines, fmt.Sprintf("Last Job: %s", lastJob.Type))
		if lastJob.Status != "" {
			sv.PriorStepSummaryLines = append(sv.PriorStepSummaryLines, fmt.Sprintf("Status: %s", lastJob.Status))
		}
		if lastJob.ID != "" {
			sv.PriorStepSummaryLines = append(sv.PriorStepSummaryLines, fmt.Sprintf("Job ID: %s", lastJob.ID))
		}
	}

	if outcome, hasOutcome := state.GetOutcomeCategory(ctx.Envelope); hasOutcome && outcome != "" {
		sv.PriorStepSummaryLines = append(sv.PriorStepSummaryLines, fmt.Sprintf("Previous Outcome: %s", outcome))
	}

	if artifacts, hasArtifacts := state.GetOutcomeArtifacts(ctx.Envelope); hasArtifacts && len(artifacts) > 0 {
		sv.PriorStepSummaryLines = append(sv.PriorStepSummaryLines, fmt.Sprintf("Previous Artifacts: %s", strings.Join(artifacts, ", ")))
	}

	if constraints, hasConstraints := state.GetNegativeConstraints(ctx.Envelope); hasConstraints && len(constraints) > 0 {
		sv.PriorStepSummaryLines = append(sv.PriorStepSummaryLines, fmt.Sprintf("Negative Constraints: %s", strings.Join(constraints, ", ")))
	}

	out := sv.RenderPriorStepSummary()
	if out == "" {
		return prompt.ContextChunk{Content: ""}
	}
	return prompt.ContextChunk{Content: out}
}

func (p *thoughtrecipePriorStepProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "euclo.thoughtrecipe_prior_step_result",
		Description: "Provides summary and results from the previously executed thoughtrecipe step",
		Paradigms:   []string{"euclo"},
		ReadsKeys:   surface.PromptReadsKeys(),
	}
}
