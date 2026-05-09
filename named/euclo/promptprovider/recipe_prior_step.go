package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// thoughtrecipePriorStepProvider provides context from the last executed thoughtrecipe step.
type thoughtrecipePriorStepProvider struct{}

func (p *thoughtrecipePriorStepProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{Content: ""}
	}

	var priorInfo []string

	// Try to get frame history (which contains step execution order)
	if frameHistory, hasHistory := state.GetFrameHistory(ctx.Envelope); hasHistory && len(frameHistory) > 0 {
		// Get the most recent frame (last step)
		lastFrame := frameHistory[len(frameHistory)-1]
		priorInfo = append(priorInfo, fmt.Sprintf("Last Frame: %s", lastFrame))
	}

	// Try to get job records (which contain step execution details)
	if jobRecords, hasRecords := state.GetJobRecords(ctx.Envelope); hasRecords && len(jobRecords) > 0 {
		// Get the most recent job record
		lastJob := jobRecords[len(jobRecords)-1]
		priorInfo = append(priorInfo, fmt.Sprintf("Last Job: %s", lastJob.Type))
		if lastJob.Status != "" {
			priorInfo = append(priorInfo, fmt.Sprintf("Status: %s", lastJob.Status))
		}
		if lastJob.ID != "" {
			priorInfo = append(priorInfo, fmt.Sprintf("Job ID: %s", lastJob.ID))
		}
	}

	// Try to get outcome category
	if outcome, hasOutcome := state.GetOutcomeCategory(ctx.Envelope); hasOutcome && outcome != "" {
		priorInfo = append(priorInfo, fmt.Sprintf("Previous Outcome: %s", outcome))
	}

	// Try to get outcome artifacts
	if artifacts, hasArtifacts := state.GetOutcomeArtifacts(ctx.Envelope); hasArtifacts && len(artifacts) > 0 {
		priorInfo = append(priorInfo, fmt.Sprintf("Previous Artifacts: %s", strings.Join(artifacts, ", ")))
	}

	// Try to get any negative constraints from previous steps
	if constraints, hasConstraints := state.GetNegativeConstraints(ctx.Envelope); hasConstraints && len(constraints) > 0 {
		priorInfo = append(priorInfo, fmt.Sprintf("Negative Constraints: %s", strings.Join(constraints, ", ")))
	}

	if len(priorInfo) == 0 {
		return prompt.ContextChunk{Content: ""}
	}

	// Format as a structured context block
	return prompt.ContextChunk{Content: "Previous Step Summary:\n" + strings.Join(priorInfo, "\n")}
}

func (p *thoughtrecipePriorStepProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "euclo.thoughtrecipe_prior_step_result",
		Description: "Provides summary and results from the previously executed thoughtrecipe step",
		Paradigms:   []string{"euclo"},
		ReadsKeys: []string{
			"euclo.frame_history",
			"euclo.job_records",
			"euclo.outcome_category",
			"euclo.outcome_artifacts",
			"euclo.negative_constraints",
		},
	}
}
