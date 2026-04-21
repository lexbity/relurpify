package modes

import (
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// PlanningMode builds the phase machine for the planning interaction mode.
//
// Phases: scope → clarify → generate → compare → refine → commit
func PlanningMode(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
	RegisterPlanningTriggers(resolver)

	return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:     "planning",
		Emitter:  emitter,
		Resolver: resolver,
		Phases: []interaction.PhaseDefinition{
			{
				ID:      "scope",
				Label:   "Scope",
				Handler: &ScopePhase{},
			},
			{
				ID:        "clarify",
				Label:     "Clarify",
				Handler:   &ClarifyPhase{},
				Skippable: true,
				SkipWhen:  skipClarify,
			},
			{
				ID:      "generate",
				Label:   "Generate",
				Handler: &GeneratePhase{},
			},
			{
				ID:        "compare",
				Label:     "Compare",
				Handler:   &ComparePhase{},
				Skippable: true,
				SkipWhen:  skipCompare,
			},
			{
				ID:        "refine",
				Label:     "Refine",
				Handler:   &RefinePhase{},
				Skippable: true,
				SkipWhen:  skipRefine,
			},
			{
				ID:      "commit",
				Label:   "Commit",
				Handler: &CommitPhase{},
			},
		},
	})
}

// skipClarify skips when scope was confirmed without corrections and no ambiguity detected.
func skipClarify(state map[string]any, _ *interaction.ArtifactBundle) bool {
	// If user used "just plan it" trigger, skip.
	if v, _ := state["just_plan_it"].(bool); v {
		return true
	}
	// If scope was confirmed (not corrected), skip clarify.
	resp, _ := state["scope.response"].(string)
	return resp == "confirm"
}

// skipCompare skips when there's only one candidate or user already selected.
func skipCompare(state map[string]any, _ *interaction.ArtifactBundle) bool {
	if v, _ := state["just_plan_it"].(bool); v {
		return true
	}
	count, _ := state["generate.candidate_count"].(int)
	if count <= 1 {
		return true
	}
	_, selected := state["generate.selected"]
	return selected
}

// skipRefine skips when user said "just plan it".
func skipRefine(state map[string]any, _ *interaction.ArtifactBundle) bool {
	v, _ := state["just_plan_it"].(bool)
	return v
}

// RegisterPlanningTriggers registers agency triggers for the planning mode.
func RegisterPlanningTriggers(resolver *interaction.AgencyResolver) {
	if resolver == nil {
		return
	}
	resolver.RegisterTrigger("planning", interaction.AgencyTrigger{
		Phrases:      []string{"alternatives", "show alternatives", "more options"},
		CapabilityID: "euclo:design.alternatives",
		PhaseJump:    "generate",
		Description:  "Generate and compare more plan candidates",
	})
	resolver.RegisterTrigger("planning", interaction.AgencyTrigger{
		Phrases:     []string{"just plan it", "skip to plan"},
		Description: "Skip clarification and refinement, generate plan directly",
	})
	resolver.RegisterTrigger("planning", interaction.AgencyTrigger{
		Phrases:      []string{"what are the risks", "risks", "risk analysis"},
		CapabilityID: "euclo:planner.plan",
		Description:  "Analyze risks of the current plan",
	})
}

// PlanningPhaseIDs returns the ordered phase IDs for planning mode.
func PlanningPhaseIDs() []string {
	return []string{"scope", "clarify", "generate", "compare", "refine", "commit"}
}

// PlanningPhaseLabels returns phase labels for the help surface.
func PlanningPhaseLabels() []interaction.PhaseInfo {
	ids := PlanningPhaseIDs()
	labels := []string{"Scope", "Clarify", "Generate", "Compare", "Refine", "Commit"}
	out := make([]interaction.PhaseInfo, len(ids))
	for i := range ids {
		out[i] = interaction.PhaseInfo{ID: ids[i], Label: labels[i]}
	}
	return out
}

// planArtifact builds the plan artifact from state.
func planArtifact(state map[string]any) euclotypes.Artifact {
	summary, _ := state["generate.selected_summary"].(string)
	if summary == "" {
		summary = "plan generated"
	}
	return euclotypes.Artifact{
		ID:       "interaction_plan",
		Kind:     euclotypes.ArtifactKindPlan,
		Summary:  summary,
		Payload:  state["refine.items"],
		Status:   "produced",
		Metadata: map[string]any{"source": "planning_interaction"},
	}
}
