package euclo

import (
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/interaction/modes"
)

// defaultInteractionRegistry creates a registry without pipeline injection.
// Used as a fallback when agent is not fully initialized.
func defaultInteractionRegistry() *interaction.ModeMachineRegistry {
	reg := interaction.NewModeMachineRegistry()
	reg.Register("chat", modes.ChatModeLegacy)
	reg.Register("code", modes.CodeMode)
	reg.Register("debug", modes.DebugMode)
	reg.Register("planning", modes.PlanningMode)
	reg.Register("review", modes.ReviewMode)
	reg.Register("tdd", modes.TDDMode)
	return reg
}
