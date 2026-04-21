package modes

import (
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// CodeMode builds the phase machine for the code interaction mode.
//
// Phases: understand → propose → execute → verify → present
func CodeMode(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
	RegisterCodeTriggers(resolver)

	return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:     "code",
		Emitter:  emitter,
		Resolver: resolver,
		Phases: []interaction.PhaseDefinition{
			{
				ID:      "understand",
				Label:   "Understand",
				Handler: &IntentPhase{},
			},
			{
				ID:        "propose",
				Label:     "Propose",
				Handler:   &EditProposalPhase{},
				Skippable: true,
				SkipWhen:  skipPropose,
			},
			{
				ID:      "execute",
				Label:   "Execute",
				Handler: &CodeExecutionPhase{},
			},
			{
				ID:      "verify",
				Label:   "Verify",
				Handler: &VerificationPhase{},
			},
			{
				ID:      "present",
				Label:   "Present",
				Handler: &CodePresentPhase{},
			},
		},
	})
}

// skipPropose skips when change is estimated small or user said "just do it".
func skipPropose(state map[string]any, _ *interaction.ArtifactBundle) bool {
	if v, _ := state["just_do_it"].(bool); v {
		return true
	}
	if small, _ := state["understand.small_change"].(bool); small {
		return true
	}
	return false
}

// RegisterCodeTriggers registers agency triggers for the code mode.
func RegisterCodeTriggers(resolver *interaction.AgencyResolver) {
	if resolver == nil {
		return
	}
	resolver.RegisterTrigger("code", interaction.AgencyTrigger{
		Phrases:     []string{"verify", "verify this"},
		PhaseJump:   "verify",
		Description: "Re-run verification",
	})
	resolver.RegisterTrigger("code", interaction.AgencyTrigger{
		Phrases:      []string{"try different approach", "different approach"},
		CapabilityID: "euclo:edit_verify_repair",
		Description:  "Re-enter execute with paradigm switch",
	})
	resolver.RegisterTrigger("code", interaction.AgencyTrigger{
		Phrases:     []string{"debug this", "debug"},
		Description: "Propose transition to debug mode",
	})
	resolver.RegisterTrigger("code", interaction.AgencyTrigger{
		Phrases:     []string{"plan first", "show plan"},
		Description: "Transition to planning, return with plan artifact",
	})
	resolver.RegisterTrigger("code", interaction.AgencyTrigger{
		Phrases:     []string{"just do it"},
		Description: "Skip proposal phase, fast-path through execution",
	})
}

// CodePhaseIDs returns the ordered phase IDs for code mode.
func CodePhaseIDs() []string {
	return []string{"understand", "propose", "execute", "verify", "present"}
}

// CodePhaseLabels returns phase labels for the help surface.
func CodePhaseLabels() []interaction.PhaseInfo {
	ids := CodePhaseIDs()
	labels := []string{"Understand", "Propose", "Execute", "Verify", "Present"}
	out := make([]interaction.PhaseInfo, len(ids))
	for i := range ids {
		out[i] = interaction.PhaseInfo{ID: ids[i], Label: labels[i]}
	}
	return out
}
