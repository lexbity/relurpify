package surface

import "strings"

// RecipeRegistryLookup is the minimal interface the TUI needs to rehydrate a
// recipe projection from the registry on session resume.
type RecipeRegistryLookup interface {
	LookupRecipe(recipeID string) (*RecipeProjection, bool)
}

// StateView provides a structured, UX-agnostic projection of Euclo runtime
// state that can be rendered as a text block by both LLM prompt providers
// and UI panes. Each field carries pre-formatted line content; the Render*
// methods join them with the appropriate header.
type StateView struct {
	// ClarificationRuntimeLines holds the body lines for the Clarification
	// Runtime block (one line per entry, no trailing newline).
	ClarificationRuntimeLines []string

	// PlanGoalViewLines holds the body lines for the Clarification Plan View block.
	PlanGoalViewLines []string

	// PriorStepSummaryLines holds the body lines for the Prior Step Summary block.
	PriorStepSummaryLines []string
}

// RenderClarificationRuntime joins the runtime lines with the standard header.
func (sv StateView) RenderClarificationRuntime() string {
	if len(sv.ClarificationRuntimeLines) == 0 {
		return ""
	}
	return "Clarification Runtime:\n" + strings.Join(sv.ClarificationRuntimeLines, "\n")
}

// RenderPlanGoalView joins the plan/goal lines with the standard header.
func (sv StateView) RenderPlanGoalView() string {
	if len(sv.PlanGoalViewLines) == 0 {
		return ""
	}
	return "Clarification Plan View:\n" + strings.Join(sv.PlanGoalViewLines, "\n")
}

// RenderPriorStepSummary joins the prior-step lines with the standard header.
func (sv StateView) RenderPriorStepSummary() string {
	if len(sv.PriorStepSummaryLines) == 0 {
		return ""
	}
	return "Previous Step Summary:\n" + strings.Join(sv.PriorStepSummaryLines, "\n")
}

// PromptReadsKeys returns the canonical set of envelope/state keys read by
// Euclo's prompt providers. Both the providers and the UI should source
// this list from here to stay in sync.
func PromptReadsKeys() []string {
	return []string{
		"euclo.frame_history",
		"euclo.job_records",
		"euclo.outcome_category",
		"euclo.outcome_artifacts",
		"euclo.negative_constraints",

		"euclo.intent.clarification.state",
		"euclo.intent.clarification.ambiguity",
		"euclo.intent.clarification.turns",
		"euclo.intent.clarification.confirmed_entities",
		"euclo.intent.clarification.confirmed_scopes",
		"euclo.intent.clarification.relation_intents",
		"euclo.intent.clarification.grounded_anchors",
		"euclo.intent.clarification.pending_projection",
		"euclo.intent.clarification.projected_mutations",
		"euclo.intent.clarification.active_thoughtrecipe",
		"euclo.intent.clarification.last_checkpoint_id",
		"euclo.intent.clarification.last_checkpoint_seq",
		"euclo.intent.clarification.evidence",
		"euclo.intent.clarification.interpretation",
	}
}
