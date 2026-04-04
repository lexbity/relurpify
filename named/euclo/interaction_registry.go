package euclo

import (
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/interaction/modes"
)

func defaultInteractionRegistry() *interaction.ModeMachineRegistry {
	reg := interaction.NewModeMachineRegistry()
	// Register a factory function that can create chat mode with context enrichment
	reg.Register("chat", func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		// We'll need to get the pipeline from somewhere
		// For now, use the legacy mode
		return modes.ChatModeLegacy(emitter, resolver)
	})
	reg.Register("code", modes.CodeMode)
	reg.Register("debug", modes.DebugMode)
	reg.Register("planning", modes.PlanningMode)
	reg.Register("review", modes.ReviewMode)
	reg.Register("tdd", modes.TDDMode)
	return reg
}
