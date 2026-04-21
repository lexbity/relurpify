package rewoo

import (
	"context"
	"encoding/json"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/contextmgr"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func synthesize(
	ctx context.Context,
	model core.LanguageModel,
	task *core.Task,
	results []RewooStepResult,
	policy *contextmgr.ContextPolicy,
	shared *core.SharedContext,
	state *core.Context,
) (string, error) {
	if model == nil {
		return "", fmt.Errorf("rewoo: synthesizer model unavailable")
	}

	// Enforce budget before building prompt
	if policy != nil && state != nil && shared != nil {
		policy.EnforceBudget(state, shared, model, nil, nil)
	}

	summary, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("rewoo: results summary: %w", err)
	}
	userPrompt := fmt.Sprintf("Task:\n%s\n\nTool results:\n%s", taskInstruction(task), string(summary))
	if sharedBlock := sharedContextPromptBlock(shared, policy); sharedBlock != "" {
		userPrompt += "\n\nShared context:\n" + sharedBlock
	}
	resp, err := model.Chat(ctx, []core.Message{
		{Role: "system", Content: "You are a synthesis assistant. Produce a concise final answer using only the structured tool results."},
		{Role: "user", Content: userPrompt},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("rewoo: synthesis failed: %w", err)
	}

	// Record the interaction
	if policy != nil && state != nil {
		policy.RecordLatestInteraction(state, nil)
	}

	return resp.Text, nil
}
