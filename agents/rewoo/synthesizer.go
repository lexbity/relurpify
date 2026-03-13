package rewoo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
)

func synthesize(ctx context.Context, model core.LanguageModel, task *core.Task, results []RewooStepResult) (string, error) {
	if model == nil {
		return "", fmt.Errorf("rewoo: synthesizer model unavailable")
	}
	summary, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("rewoo: results summary: %w", err)
	}
	resp, err := model.Chat(ctx, []core.Message{
		{Role: "system", Content: "You are a synthesis assistant. Produce a concise final answer using only the structured tool results."},
		{Role: "user", Content: fmt.Sprintf("Task:\n%s\n\nTool results:\n%s", taskInstruction(task), string(summary))},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("rewoo: synthesis failed: %w", err)
	}
	return resp.Text, nil
}
