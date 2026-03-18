package euclo

import (
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/interaction/modes"
)

func defaultInteractionRegistry() *interaction.ModeMachineRegistry {
	reg := interaction.NewModeMachineRegistry()
	reg.Register("code", modes.CodeMode)
	reg.Register("debug", modes.DebugMode)
	reg.Register("planning", modes.PlanningMode)
	reg.Register("review", modes.ReviewMode)
	reg.Register("tdd", modes.TDDMode)
	return reg
}
