package modes

import (
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ReviewMode builds the phase machine for the review interaction mode.
//
// Phases: scope → sweep → triage → act → re_review
func ReviewMode(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
	RegisterReviewTriggers(resolver)

	return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:     "review",
		Emitter:  emitter,
		Resolver: resolver,
		Phases: []interaction.PhaseDefinition{
			{
				ID:      "scope",
				Label:   "Scope",
				Handler: &ReviewScopePhase{},
			},
			{
				ID:      "sweep",
				Label:   "Sweep",
				Handler: &ReviewSweepPhase{},
			},
			{
				ID:      "triage",
				Label:   "Triage",
				Handler: &TriagePhase{},
			},
			{
				ID:        "act",
				Label:     "Act",
				Handler:   &BatchFixPhase{},
				Skippable: true,
				SkipWhen:  skipAct,
			},
			{
				ID:        "re_review",
				Label:     "Re-review",
				Handler:   &ReReviewPhase{},
				Skippable: true,
				SkipWhen:  skipReReview,
			},
		},
	})
}

// skipAct skips when user chose "no fixes needed".
func skipAct(state map[string]any, _ *interaction.ArtifactBundle) bool {
	if v, _ := state["triage.no_fixes"].(bool); v {
		return true
	}
	return false
}

// skipReReview skips when user chose "accept without re-review".
func skipReReview(state map[string]any, _ *interaction.ArtifactBundle) bool {
	if v, _ := state["act.skip_re_review"].(bool); v {
		return true
	}
	// Also skip if act was skipped (no fixes).
	if v, _ := state["triage.no_fixes"].(bool); v {
		return true
	}
	return false
}

// RegisterReviewTriggers registers agency triggers for the review mode.
func RegisterReviewTriggers(resolver *interaction.AgencyResolver) {
	if resolver == nil {
		return
	}
	resolver.RegisterTrigger("review", interaction.AgencyTrigger{
		Phrases:      []string{"check compatibility", "compatibility"},
		CapabilityID: "euclo:review.compatibility",
		Description:  "Add compatibility analysis to the review sweep",
	})
	resolver.RegisterTrigger("review", interaction.AgencyTrigger{
		Phrases:      []string{"fix all critical"},
		CapabilityID: "euclo:review.implement_if_safe",
		Description:  "Fix all critical findings",
	})
	resolver.RegisterTrigger("review", interaction.AgencyTrigger{
		Phrases:     []string{"fix all"},
		Description: "Fix all findings",
	})
	resolver.RegisterTrigger("review", interaction.AgencyTrigger{
		Phrases:     []string{"narrow to file", "focus on"},
		Description: "Narrow review scope to specific file(s)",
	})
}

// ReviewPhaseIDs returns the ordered phase IDs for review mode.
func ReviewPhaseIDs() []string {
	return []string{"scope", "sweep", "triage", "act", "re_review"}
}

// ReviewPhaseLabels returns phase labels for the help surface.
func ReviewPhaseLabels() []interaction.PhaseInfo {
	ids := ReviewPhaseIDs()
	labels := []string{"Scope", "Sweep", "Triage", "Act", "Re-review"}
	out := make([]interaction.PhaseInfo, len(ids))
	for i := range ids {
		out[i] = interaction.PhaseInfo{ID: ids[i], Label: labels[i]}
	}
	return out
}
