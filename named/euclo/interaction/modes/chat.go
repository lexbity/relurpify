package modes

import (
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/runtime/pretask"
)

// ChatMode builds the phase machine for the chat interaction mode.
//
// Phases: context_proposal → intent → present → reflect
//
// Chat mode is for conversational tasks: answering questions, explaining
// concepts, and optionally proposing transitions to code/debug for
// implementation or investigation work.
func ChatMode(
	emitter interaction.FrameEmitter,
	resolver *interaction.AgencyResolver,
	pipeline ContextEnrichmentPipeline,
	fileResolver *pretask.FileResolver,
) *interaction.PhaseMachine {
	RegisterChatTriggers(resolver)

	phases := []interaction.PhaseDefinition{}
	
	// Add context proposal phase if pipeline is provided
	if pipeline != nil && fileResolver != nil {
		phases = append(phases, interaction.PhaseDefinition{
			ID:      "context_proposal",
			Label:   "Context",
			Handler: &ContextProposalPhase{
				Pipeline:     pipeline,
				FileResolver: fileResolver,
			},
		})
	}
	
	// Add the original phases
	phases = append(phases, []interaction.PhaseDefinition{
		{
			ID:      "intent",
			Label:   "Intent",
			Handler: &ChatIntentPhase{},
		},
		{
			ID:      "present",
			Label:   "Present",
			Handler: &ChatPresentPhase{},
		},
		{
			ID:        "reflect",
			Label:     "Reflect",
			Handler:   &ChatReflectPhase{},
			Skippable: true,
			SkipWhen:  skipChatReflect,
		},
	}...)

	return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:     "chat",
		Emitter:  emitter,
		Resolver: resolver,
		Phases:   phases,
	})
}

// ChatModeLegacy provides backward compatibility for callers that don't provide pipeline.
func ChatModeLegacy(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
	return ChatMode(emitter, resolver, nil, nil)
}

// skipChatReflect skips the reflect phase when the user explicitly opts out
// or when the interaction was a simple ask with no follow-on actions.
func skipChatReflect(state map[string]any, _ *interaction.ArtifactBundle) bool {
	if v, _ := state["chat.skip_reflect"].(bool); v {
		return true
	}
	// Skip reflect for pure ask interactions that produced a direct answer.
	if subMode, _ := state["chat.sub_mode"].(string); subMode == "ask" {
		if _, answered := state["present.answered"].(bool); answered {
			return true
		}
	}
	return false
}

// RegisterChatTriggers registers agency triggers for chat mode.
func RegisterChatTriggers(resolver *interaction.AgencyResolver) {
	if resolver == nil {
		return
	}
	resolver.RegisterTrigger("chat", interaction.AgencyTrigger{
		Phrases:     []string{"implement this", "can you write", "write this"},
		Description: "Propose transition to code mode for implementation",
	})
	resolver.RegisterTrigger("chat", interaction.AgencyTrigger{
		Phrases:     []string{"debug this", "why is it failing"},
		Description: "Propose transition to debug mode for investigation",
	})
	resolver.RegisterTrigger("chat", interaction.AgencyTrigger{
		Phrases:     []string{"review this"},
		Description: "Propose transition to review mode",
	})
}

// ChatPhaseIDs returns the ordered phase IDs for chat mode.
func ChatPhaseIDs() []string {
	return []string{"context_proposal", "intent", "present", "reflect"}
}

// ChatPhaseLabels returns phase labels for the help surface.
func ChatPhaseLabels() []interaction.PhaseInfo {
	ids := ChatPhaseIDs()
	labels := []string{"Context", "Intent", "Present", "Reflect"}
	out := make([]interaction.PhaseInfo, len(ids))
	for i := range ids {
		out[i] = interaction.PhaseInfo{ID: ids[i], Label: labels[i]}
	}
	return out
}
