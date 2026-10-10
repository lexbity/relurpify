package rewoo

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
)

// SynthesizeWithModel composes rewoo.final_output from the aggregated step
// results with exactly ONE LLM call. It is the third phase of the paradigm;
// RewooOptions.Synthesize=false skips it in favor of the mechanical summary
// (the synthesize node owns that branch, including its mode=mechanical
// telemetry).
func (a *RewooAgent) SynthesizeWithModel(ctx context.Context, env *contextdata.Envelope) (string, error) {
	if a == nil || a.Model == nil {
		return "", fmt.Errorf("rewoo: language model unavailable for synthesis")
	}
	plan := planFromEnvelope(env)
	results := stepResultsFromEnvelope(env)
	a.emitLLMPhase(ctx, env, "synthesize", "llm")
	streamed, err := paradigm.StreamedSection(ctx, env, "rewoo")
	if err != nil {
		return "", err
	}
	messages := []model.Message{
		{Role: "system", Content: a.resolvePhasePrompt(ctx, synthPromptID, nil, env)},
	}
	if streamed != "" {
		messages = append(messages, model.Message{Role: "system", Content: streamed})
	}
	if guidance := strings.TrimSpace(a.Options.SynthesizeGuidance); guidance != "" {
		messages = append(messages, model.Message{Role: "system", Content: synthesizeGuidanceMessage(guidance)})
	}
	messages = append(messages, model.Message{Role: "user", Content: synthUserPrompt(plan, results)})
	resp, err := a.Model.Chat(ctx, messages, &model.LLMOptions{
		Model:       a.modelID(),
		Temperature: 0.1,
		MaxTokens:   512,
	})
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(resp.Text)
	if summary == "" {
		summary = mechanicalSummary(results)
	}
	return summary, nil
}

// synthesizeGuidanceMessage renders the recipe-authored synthesizer guidance
// as an authoritative system instruction. The default synthesizer prompt still
// applies wherever the guidance is silent.
func synthesizeGuidanceMessage(guidance string) string {
	return fmt.Sprintf("Synthesizer guidance from recipe (authoritative):\n%s\n\nDefault instructions apply where guidance is silent.", guidance)
}

// MechanicalSummary is the synthesis-free final output: a deterministic
// rendering of the step results.
func MechanicalSummary(results []RewooStepResult) string {
	return mechanicalSummary(results)
}

func mechanicalSummary(results []RewooStepResult) string {
	if summary := summarizeRewooStepResults(results); summary != "" {
		return "Mechanical step summary: " + summary
	}
	return "No steps were executed."
}

func synthUserPrompt(plan *RewooPlan, results []RewooStepResult) string {
	var b strings.Builder
	goal := ""
	if plan != nil {
		goal = plan.Goal
	}
	fmt.Fprintf(&b, "Task: %s\n\nExecuted steps:\n", goal)
	if len(results) == 0 {
		b.WriteString("(no steps ran)\n")
	}
	for _, result := range results {
		status := "ok"
		if !result.Success {
			status = "FAILED"
		}
		fmt.Fprintf(&b, "- %s (%s): %s\n", result.StepID, result.Tool, status)
		if result.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", strings.TrimSpace(result.Error))
		}
		if len(result.Output) > 0 {
			fmt.Fprintf(&b, "  output: %v\n", result.Output)
		}
	}
	b.WriteString("\nCompose the final answer to the task from these results. No tool calls, no questions.")
	return b.String()
}

func planFromEnvelope(env *contextdata.Envelope) *RewooPlan {
	if env == nil {
		return nil
	}
	if plan, ok := contextdata.GetTyped[*RewooPlan](env, "rewoo.plan"); ok {
		return plan
	}
	return nil
}

func stepResultsFromEnvelope(env *contextdata.Envelope) []RewooStepResult {
	if env == nil {
		return nil
	}
	if results, ok := contextdata.GetTyped[[]RewooStepResult](env, "rewoo.tool_results"); ok {
		return results
	}
	return nil
}
