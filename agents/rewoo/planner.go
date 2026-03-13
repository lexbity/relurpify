package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

type rewooPlannerNode struct {
	Model   core.LanguageModel
	Options core.LLMOptions
}

func (n *rewooPlannerNode) Plan(ctx context.Context, task *core.Task, toolSpecs []core.LLMToolSpec) (*RewooPlan, error) {
	if n == nil || n.Model == nil {
		return nil, fmt.Errorf("rewoo: planner model unavailable")
	}
	resp, err := n.Model.Chat(ctx, []core.Message{
		{Role: "system", Content: buildPlannerPrompt(task, toolSpecs)},
		{Role: "user", Content: taskInstruction(task)},
	}, &n.Options)
	if err != nil {
		return nil, fmt.Errorf("rewoo: planning failed: %w", err)
	}
	plan, err := ParsePlan(resp.Text)
	if err != nil {
		return nil, fmt.Errorf("rewoo: parse plan: %w", err)
	}
	return plan, nil
}

// ParsePlan parses a JSON-encoded ReWOO plan.
func ParsePlan(raw string) (*RewooPlan, error) {
	var plan RewooPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func buildPlannerPrompt(task *core.Task, toolSpecs []core.LLMToolSpec) string {
	toolJSON, _ := json.MarshalIndent(toolSpecs, "", "  ")
	contextBlock := plannerContextBlock(task)
	return fmt.Sprintf(`You are a ReWOO planner.
Create a tool execution plan for the task using only the provided tools.
Output JSON only. Do not use markdown. Do not explain anything.

Task instruction:
%s

Planning context:
%s

Available tools:
%s

Return JSON matching this exact structure:
{
  "goal": "short goal",
  "steps": [
    {
      "id": "step_1",
      "description": "what this step does",
      "tool": "tool_name",
      "params": {},
      "depends_on": [],
      "on_failure": "skip"
    }
  ]
}

Rules:
- Use at most one tool per step.
- Use only tool names from the available tools list.
- Step ids must be unique.
- depends_on must reference prior step ids only.
- on_failure must be one of "skip", "abort", or "replan".`, taskInstruction(task), contextBlock, string(toolJSON))
}

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}

func plannerContextBlock(task *core.Task) string {
	if task == nil || len(task.Context) == 0 {
		return "None."
	}
	parts := make([]string, 0, 3)
	if raw, ok := task.Context["workflow_retrieval"]; ok {
		if text := strings.TrimSpace(fmt.Sprint(raw)); text != "" && text != "<nil>" {
			parts = append(parts, "Workflow retrieval:\n"+text)
		}
	}
	if raw, ok := task.Context["rewoo_replan_context"]; ok {
		if text := strings.TrimSpace(fmt.Sprint(raw)); text != "" && text != "<nil>" {
			parts = append(parts, "Replan context:\n"+text)
		}
	}
	if len(parts) == 0 {
		return "None."
	}
	return strings.Join(parts, "\n\n")
}
