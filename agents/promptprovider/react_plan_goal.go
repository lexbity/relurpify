package promptprovider

import (
	"strings"

	pl "codeburg.org/lexbit/relurpify/agents/plan"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type reactPlanGoalProvider struct{}

func (reactPlanGoalProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Envelope == nil {
		return prompt.ContextChunk{}
	}
	plan := resolvePlan(ctx)
	if plan == nil {
		return prompt.ContextChunk{}
	}
	if plan.Goal != "" {
		return prompt.ContextChunk{Content: plan.Goal}
	}
	if len(plan.Files) > 0 {
		return prompt.ContextChunk{Content: "Files in scope: " + strings.Join(plan.Files, ", ")}
	}
	return prompt.ContextChunk{}
}

func (reactPlanGoalProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.plan_goal",
		Description: "Supplies the plan goal or file scope from architect.plan or planner.plan in the envelope.",
		Paradigms:   []string{"react"},
		ReadsKeys:   []string{"architect.plan", "planner.plan"},
	}
}

// resolvePlan extracts a plan.Plan from the envelope (checks architect.plan then planner.plan).
func resolvePlan(ctx prompt.RuntimeContext) *pl.Plan {
	for _, key := range []string{"architect.plan", "planner.plan"} {
		raw, ok := envelopeGet(ctx.Envelope, key)
		if !ok || raw == nil {
			continue
		}
		if p, ok := raw.(pl.Plan); ok {
			return &p
		}
		if p, ok := raw.(*pl.Plan); ok && p != nil {
			return p
		}
	}
	return nil
}
