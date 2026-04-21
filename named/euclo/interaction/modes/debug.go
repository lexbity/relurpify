package modes

import (
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// DebugMode builds the phase machine for the debug interaction mode.
//
// Phases: intake → reproduce → localize → propose_fix → apply → confirm
func DebugMode(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
	RegisterDebugTriggers(resolver)

	return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:     "debug",
		Emitter:  emitter,
		Resolver: resolver,
		Phases: []interaction.PhaseDefinition{
			{
				ID:        "intake",
				Label:     "Intake",
				Handler:   &DiagnosticIntakePhase{},
				Skippable: true,
				SkipWhen:  skipIntake,
			},
			{
				ID:        "reproduce",
				Label:     "Reproduce",
				Handler:   &ReproductionPhase{},
				Skippable: true,
				SkipWhen:  skipReproduce,
			},
			{
				ID:      "localize",
				Label:   "Localize",
				Handler: &LocalizationPhase{},
			},
			{
				ID:      "propose_fix",
				Label:   "Propose Fix",
				Handler: &DebugFixProposalPhase{},
			},
			{
				ID:      "apply",
				Label:   "Apply",
				Handler: &CodeExecutionPhase{},
			},
			{
				ID:      "confirm",
				Label:   "Confirm",
				Handler: &VerificationPhase{},
			},
		},
	})
}

// skipIntake skips when the instruction already contains evidence
// (error text, stacktrace, test name).
func skipIntake(state map[string]any, _ *interaction.ArtifactBundle) bool {
	if v, _ := state["has_evidence"].(bool); v {
		return true
	}
	if v, _ := state["requires_evidence_before_mutation"].(bool); v {
		// Evidence IS required but already present in instruction signals.
		if _, ok := state["evidence_in_instruction"]; ok {
			return true
		}
	}
	return false
}

// skipReproduce skips when user said "skip reproduction" or "I know the cause".
func skipReproduce(state map[string]any, _ *interaction.ArtifactBundle) bool {
	if v, _ := state["skip_reproduction"].(bool); v {
		return true
	}
	if v, _ := state["known_cause"].(bool); v {
		return true
	}
	return false
}

// RegisterDebugTriggers registers agency triggers for the debug mode.
func RegisterDebugTriggers(resolver *interaction.AgencyResolver) {
	if resolver == nil {
		return
	}
	resolver.RegisterTrigger("debug", interaction.AgencyTrigger{
		Phrases:      []string{"investigate regression", "regression"},
		CapabilityID: "euclo:debug.investigate_regression",
		Description:  "Investigate if this is a regression",
	})
	resolver.RegisterTrigger("debug", interaction.AgencyTrigger{
		Phrases:      []string{"show trace", "trace", "trace this", "run with tracing"},
		CapabilityID: "euclo:trace.analyze",
		Description:  "Collect and display execution trace",
	})
	resolver.RegisterTrigger("debug", interaction.AgencyTrigger{
		Phrases:     []string{"skip reproduction", "I know the cause"},
		Description: "Skip reproduction and provide the cause directly",
	})
	resolver.RegisterTrigger("debug", interaction.AgencyTrigger{
		Phrases:     []string{"just fix it"},
		Description: "Skip intake and reproduction, go straight to localization",
	})
	// Trigger disambiguation: simple repair for direct fix imperatives
	resolver.RegisterTrigger("debug", interaction.AgencyTrigger{
		Phrases:      []string{"apply fix", "quick fix", "simple repair", "fix this"},
		CapabilityID: "euclo:debug.repair.simple",
		Description:  "Direct react-only repair for well-understood defects",
	})
	// Investigative vocabulary routes to investigate-repair (default behavior)
	resolver.RegisterTrigger("debug", interaction.AgencyTrigger{
		Phrases:      []string{"investigate", "root cause", "find the bug", "why is it failing"},
		CapabilityID: "euclo:debug.investigate-repair",
		Description:  "Hypothesis-driven debugging with investigation",
	})
}

// DebugPhaseIDs returns the ordered phase IDs for debug mode.
func DebugPhaseIDs() []string {
	return []string{"intake", "reproduce", "localize", "propose_fix", "apply", "confirm"}
}

// DebugPhaseLabels returns phase labels for the help surface.
func DebugPhaseLabels() []interaction.PhaseInfo {
	ids := DebugPhaseIDs()
	labels := []string{"Intake", "Reproduce", "Localize", "Propose Fix", "Apply", "Confirm"}
	out := make([]interaction.PhaseInfo, len(ids))
	for i := range ids {
		out[i] = interaction.PhaseInfo{ID: ids[i], Label: labels[i]}
	}
	return out
}
