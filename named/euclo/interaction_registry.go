package euclo

import (
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction/modes"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

// defaultInteractionRegistry creates a registry without pipeline injection.
// Used as a fallback when agent is not fully initialized.
func defaultInteractionRegistry() *interaction.ModeMachineRegistry {
	reg := interaction.NewModeMachineRegistry()
	reg.Register(euclorelurpic.ModeChat, modes.ChatModeLegacy)
	reg.Register(euclorelurpic.ModeCode, modes.CodeMode)
	reg.Register(euclorelurpic.ModeDebug, modes.DebugMode)
	reg.Register(euclorelurpic.ModePlanning, modes.PlanningMode)
	reg.Register(euclorelurpic.ModeReview, modes.ReviewMode)
	reg.Register(euclorelurpic.ModeTDD, modes.TDDMode)
	return reg
}
