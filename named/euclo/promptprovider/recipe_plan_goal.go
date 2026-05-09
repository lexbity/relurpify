package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

// thoughtrecipePlanGoalProvider provides a structured clarification state view for plan prompts.
type thoughtrecipePlanGoalProvider struct{}

func (p *thoughtrecipePlanGoalProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	state := clarificationStateFromRuntime(ctx)
	if state == nil {
		return prompt.ContextChunk{Content: ""}
	}

	var parts []string
	if taskID := strings.TrimSpace(state.TaskID); taskID != "" {
		parts = append(parts, fmt.Sprintf("Task ID: %s", taskID))
	}
	if sessionID := strings.TrimSpace(state.SessionID); sessionID != "" {
		parts = append(parts, fmt.Sprintf("Session ID: %s", sessionID))
	}
	parts = append(parts, fmt.Sprintf("Clarification State Version: %d", state.StateVersion))
	if turnID := strings.TrimSpace(state.CurrentTurnID); turnID != "" {
		parts = append(parts, fmt.Sprintf("Current Turn ID: %s", turnID))
	}
	if thoughtrecipeID := strings.TrimSpace(state.ActiveThoughtRecipeID); thoughtrecipeID != "" {
		parts = append(parts, fmt.Sprintf("Active ThoughtRecipe ID: %s", thoughtrecipeID))
	}
	if checkpointID := strings.TrimSpace(state.LastCheckpointID); checkpointID != "" {
		parts = append(parts, fmt.Sprintf("Last Checkpoint ID: %s", checkpointID))
	}
	if checkpointSeq := state.LastCheckpointSeq; checkpointSeq > 0 {
		parts = append(parts, fmt.Sprintf("Last Checkpoint Seq: %d", checkpointSeq))
	}
	if instruction, ok := ctx.Variables["instruction"]; ok && instruction != "" {
		parts = append(parts, fmt.Sprintf("Task Goal: %s", instruction))
	}

	if evidence := intentEvidenceFromRuntime(ctx); evidence != nil {
		if action := strings.TrimSpace(evidence.ActionType); action != "" {
			parts = append(parts, fmt.Sprintf("Evidence Action Type: %s", action))
		}
		if target := strings.TrimSpace(evidence.Target); target != "" {
			parts = append(parts, fmt.Sprintf("Evidence Target: %s", target))
		}
		if scope := strings.TrimSpace(evidence.Scope); scope != "" {
			parts = append(parts, fmt.Sprintf("Evidence Scope: %s", scope))
		}
		if risk := strings.TrimSpace(evidence.RiskLevel); risk != "" {
			parts = append(parts, fmt.Sprintf("Evidence Risk Level: %s", risk))
		}
		if len(evidence.MissingFields) > 0 {
			parts = append(parts, fmt.Sprintf("Evidence Missing Fields: %s", strings.Join(evidence.MissingFields, ", ")))
		}
		if len(evidence.ReasonCodes) > 0 {
			parts = append(parts, fmt.Sprintf("Evidence Reason Codes: %s", strings.Join(evidence.ReasonCodes, ", ")))
		}
	}
	if interpretation := intentInterpretationFromRuntime(ctx); interpretation != nil {
		if action := strings.TrimSpace(interpretation.ActionType); action != "" {
			parts = append(parts, fmt.Sprintf("Interpretation Action Type: %s", action))
		}
		if target := strings.TrimSpace(interpretation.Target); target != "" {
			parts = append(parts, fmt.Sprintf("Interpretation Target: %s", target))
		}
		if scope := strings.TrimSpace(interpretation.Scope); scope != "" {
			parts = append(parts, fmt.Sprintf("Interpretation Scope: %s", scope))
		}
		if risk := strings.TrimSpace(interpretation.RiskLevel); risk != "" {
			parts = append(parts, fmt.Sprintf("Interpretation Risk Level: %s", risk))
		}
		if len(interpretation.MissingInfo) > 0 {
			parts = append(parts, fmt.Sprintf("Interpretation Missing Info: %s", strings.Join(interpretation.MissingInfo, ", ")))
		}
		if rationale := strings.TrimSpace(interpretation.Rationale); rationale != "" {
			parts = append(parts, fmt.Sprintf("Interpretation Rationale: %s", rationale))
		}
		if note := strings.TrimSpace(interpretation.ConfidenceNote); note != "" {
			parts = append(parts, fmt.Sprintf("Interpretation Confidence Note: %s", note))
		}
		if len(interpretation.ReasonCodes) > 0 {
			parts = append(parts, fmt.Sprintf("Interpretation Reason Codes: %s", strings.Join(interpretation.ReasonCodes, ", ")))
		}
	}

	if len(parts) == 0 {
		return prompt.ContextChunk{Content: ""}
	}

	return prompt.ContextChunk{Content: "Clarification Plan View:\n" + strings.Join(parts, "\n")}
}

func (p *thoughtrecipePlanGoalProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "euclo.thoughtrecipe_plan_goal",
		Description: "Provides structured clarification state for thoughtrecipe plan prompts",
		Paradigms:   []string{"euclo"},
		ReadsKeys: []string{
			intentcontext.ClarificationStateKey,
			intentcontext.IntentEvidenceKey,
			intentcontext.IntentInterpretationKey,
		},
	}
}
