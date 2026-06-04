package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// thoughtrecipePlanGoalProvider provides a structured clarification state view for plan prompts.
type thoughtrecipePlanGoalProvider struct{}

func (p *thoughtrecipePlanGoalProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	state := clarificationStateFromRuntime(ctx)
	if state == nil {
		return prompt.ContextChunk{Content: ""}
	}

	sv := surface.StateView{}
	if taskID := strings.TrimSpace(state.TaskID); taskID != "" {
		sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Task ID: %s", taskID))
	}
	if sessionID := strings.TrimSpace(state.SessionID); sessionID != "" {
		sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Session ID: %s", sessionID))
	}
	sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Clarification State Version: %d", state.StateVersion))
	if turnID := strings.TrimSpace(state.CurrentTurnID); turnID != "" {
		sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Current Turn ID: %s", turnID))
	}
	if thoughtrecipeID := strings.TrimSpace(state.ActiveThoughtRecipeID); thoughtrecipeID != "" {
		sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Active ThoughtRecipe ID: %s", thoughtrecipeID))
	}
	if checkpointID := strings.TrimSpace(state.LastCheckpointID); checkpointID != "" {
		sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Last Checkpoint ID: %s", checkpointID))
	}
	if checkpointSeq := state.LastCheckpointSeq; checkpointSeq > 0 {
		sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Last Checkpoint Seq: %d", checkpointSeq))
	}
	if instruction, ok := ctx.Variables["instruction"]; ok && instruction != "" {
		sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Task Goal: %s", instruction))
	}

	if evidence := intentEvidenceFromRuntime(ctx); evidence != nil {
		if action := strings.TrimSpace(evidence.ActionType); action != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Evidence Action Type: %s", action))
		}
		if target := strings.TrimSpace(evidence.Target); target != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Evidence Target: %s", target))
		}
		if scope := strings.TrimSpace(evidence.Scope); scope != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Evidence Scope: %s", scope))
		}
		if risk := strings.TrimSpace(evidence.RiskLevel); risk != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Evidence Risk Level: %s", risk))
		}
		if len(evidence.MissingFields) > 0 {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Evidence Missing Fields: %s", strings.Join(evidence.MissingFields, ", ")))
		}
		if len(evidence.ReasonCodes) > 0 {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Evidence Reason Codes: %s", strings.Join(evidence.ReasonCodes, ", ")))
		}
	}
	if interpretation := intentInterpretationFromRuntime(ctx); interpretation != nil {
		if action := strings.TrimSpace(interpretation.ActionType); action != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Interpretation Action Type: %s", action))
		}
		if target := strings.TrimSpace(interpretation.Target); target != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Interpretation Target: %s", target))
		}
		if scope := strings.TrimSpace(interpretation.Scope); scope != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Interpretation Scope: %s", scope))
		}
		if risk := strings.TrimSpace(interpretation.RiskLevel); risk != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Interpretation Risk Level: %s", risk))
		}
		if len(interpretation.MissingInfo) > 0 {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Interpretation Missing Info: %s", strings.Join(interpretation.MissingInfo, ", ")))
		}
		if rationale := strings.TrimSpace(interpretation.Rationale); rationale != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Interpretation Rationale: %s", rationale))
		}
		if note := strings.TrimSpace(interpretation.ConfidenceNote); note != "" {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Interpretation Confidence Note: %s", note))
		}
		if len(interpretation.ReasonCodes) > 0 {
			sv.PlanGoalViewLines = append(sv.PlanGoalViewLines, fmt.Sprintf("Interpretation Reason Codes: %s", strings.Join(interpretation.ReasonCodes, ", ")))
		}
	}

	out := sv.RenderPlanGoalView()
	if out == "" {
		return prompt.ContextChunk{Content: ""}
	}
	return prompt.ContextChunk{Content: out}
}

func (p *thoughtrecipePlanGoalProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "euclo.thoughtrecipe_plan_goal",
		Description: "Provides structured clarification state for thoughtrecipe plan prompts",
		Paradigms:   []string{"euclo"},
		ReadsKeys:   surface.PromptReadsKeys(),
	}
}
