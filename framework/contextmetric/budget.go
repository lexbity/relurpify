package contextmetric

import "codeburg.org/lexbit/relurpify/framework/core"

type ContextBudget = core.ContextBudget
type TokenUsage = core.TokenUsage
type BudgetState = core.BudgetState
type Telemetry = core.Telemetry

const (
	BudgetNeedsCompression = core.BudgetNeedsCompression
	BudgetCritical         = core.BudgetCritical
)

func EstimateContextTokens(ctx *core.SharedContext) int {
	if ctx == nil {
		return 0
	}
	usage := ctx.GetTokenUsage()
	if usage == nil {
		return 0
	}
	return usage.Total
}

var EstimateCodeTokens = core.EstimateCodeTokens
var EstimateTokens = core.EstimateTokens
