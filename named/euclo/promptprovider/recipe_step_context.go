package promptprovider

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

type recipeStepContextProvider struct{}

func (p *recipeStepContextProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	state := clarificationStateFromRuntime(ctx)
	if state == nil {
		return prompt.ContextChunk{Content: ""}
	}

	var lines []string
	if taskID := strings.TrimSpace(state.TaskID); taskID != "" {
		lines = append(lines, fmt.Sprintf("Task ID: %s", taskID))
	}
	if sessionID := strings.TrimSpace(state.SessionID); sessionID != "" {
		lines = append(lines, fmt.Sprintf("Session ID: %s", sessionID))
	}
	lines = append(lines, fmt.Sprintf("Clarification State Version: %d", state.StateVersion))
	if turnID := strings.TrimSpace(state.CurrentTurnID); turnID != "" {
		lines = append(lines, fmt.Sprintf("Current Turn ID: %s", turnID))
	}
	if recipeID := strings.TrimSpace(state.ActiveRecipeID); recipeID != "" {
		lines = append(lines, fmt.Sprintf("Active Recipe ID: %s", recipeID))
	}
	if question := firstNonEmpty(ctx.Variables["question"], ctx.Variables["instruction"]); question != "" {
		lines = append(lines, fmt.Sprintf("Current Question: %s", question))
	}
	if promptID := strings.TrimSpace(ctx.Variables["prompt_id"]); promptID != "" {
		lines = append(lines, fmt.Sprintf("Prompt ID: %s", promptID))
	}
	if state.Ambiguity != nil {
		lines = append(lines, fmt.Sprintf("Ambiguity Kind: %s", state.Ambiguity.Kind))
		lines = append(lines, fmt.Sprintf("Ambiguity Confidence: %.2f", state.Ambiguity.Confidence))
		if rationale := strings.TrimSpace(state.Ambiguity.Rationale); rationale != "" {
			lines = append(lines, fmt.Sprintf("Ambiguity Rationale: %s", rationale))
		}
	}
	if anchors := anchorSummaries(state.GroundedAnchors); len(anchors) > 0 {
		lines = append(lines, "Grounded Anchors: "+strings.Join(anchors, ", "))
	}
	if entities := entitySummaries(state.ConfirmedEntities); len(entities) > 0 {
		lines = append(lines, "Confirmed Entities: "+strings.Join(entities, ", "))
	}
	if scopes := scopeSummaries(state.ConfirmedScopes); len(scopes) > 0 {
		lines = append(lines, "Confirmed Scopes: "+strings.Join(scopes, ", "))
	}
	if questions := questionSummaries(state.PendingQuestions); len(questions) > 0 {
		lines = append(lines, "Pending Questions: "+strings.Join(questions, ", "))
	}
	if projections := projectionSummaries(state.PendingProjection); len(projections) > 0 {
		lines = append(lines, "Pending Projection: "+strings.Join(projections, ", "))
	}

	if len(lines) == 0 {
		return prompt.ContextChunk{Content: ""}
	}
	return prompt.ContextChunk{Content: "Clarification Runtime:\n" + strings.Join(lines, "\n")}
}

func (p *recipeStepContextProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "euclo.recipe_step_context",
		Description: "Provides clarification runtime context for recipe step prompts",
		Paradigms:   []string{"euclo"},
		ReadsKeys: []string{
			intentcontext.ClarificationStateKey,
			"euclo.intent.clarification.state_version",
			"euclo.intent.clarification.current_turn_id",
			"euclo.intent.clarification.active_recipe_id",
			"euclo.intent.clarification.grounded_anchor_ids",
			"euclo.intent.clarification.confirmed_entity_ids",
			"euclo.intent.clarification.confirmed_scope_ids",
			"euclo.intent.clarification.pending_projection_ids",
		},
	}
}

func clarificationStateFromRuntime(ctx prompt.RuntimeContext) *intentcontext.ClarificationState {
	if ctx.State != nil {
		if value, ok := ctx.State[intentcontext.ClarificationStateKey]; ok && value != nil {
			if state, ok := value.(*intentcontext.ClarificationState); ok {
				return state
			}
		}
		if value, ok := ctx.State["clarification_state"]; ok && value != nil {
			if state, ok := value.(*intentcontext.ClarificationState); ok {
				return state
			}
		}
	}
	if ctx.Envelope != nil {
		if state, err := intentcontext.NewStateStore().Read(nil, ctx.Envelope); err == nil {
			return state
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func anchorSummaries(anchors []retrieval.AnchorRef) []string {
	if len(anchors) == 0 {
		return nil
	}
	out := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		label := strings.TrimSpace(anchor.Term)
		if label == "" {
			label = strings.TrimSpace(anchor.AnchorID)
		}
		if label == "" {
			continue
		}
		if anchor.ChunkID != "" {
			out = append(out, fmt.Sprintf("%s (%s)", label, strings.TrimSpace(anchor.ChunkID)))
		} else {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func entitySummaries(entities []intentcontext.ConfirmedEntity) []string {
	if len(entities) == 0 {
		return nil
	}
	out := make([]string, 0, len(entities))
	for _, entity := range entities {
		label := strings.TrimSpace(entity.Name)
		if label == "" {
			label = strings.TrimSpace(entity.StableID)
		}
		if label == "" {
			continue
		}
		if kind := strings.TrimSpace(string(entity.Kind)); kind != "" {
			out = append(out, fmt.Sprintf("%s [%s]", label, kind))
		} else {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func scopeSummaries(scopes []intentcontext.ConfirmedScope) []string {
	if len(scopes) == 0 {
		return nil
	}
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		label := strings.TrimSpace(scope.Name)
		if label == "" {
			label = strings.TrimSpace(scope.StableID)
		}
		if label != "" {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func questionSummaries(questions []intentcontext.ClarificationQuestion) []string {
	if len(questions) == 0 {
		return nil
	}
	out := make([]string, 0, len(questions))
	for _, question := range questions {
		label := strings.TrimSpace(question.Text)
		if label == "" {
			label = strings.TrimSpace(question.StableID)
		}
		if label != "" {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func projectionSummaries(intents []intentcontext.ProjectionIntent) []string {
	if len(intents) == 0 {
		return nil
	}
	out := make([]string, 0, len(intents))
	for _, intent := range intents {
		label := strings.TrimSpace(intent.StableID)
		if label == "" {
			label = strings.TrimSpace(intent.IdempotencyKey)
		}
		if label != "" {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}
