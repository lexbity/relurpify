package contextmetric

import "codeburg.org/lexbit/relurpify/framework/core"

type ArtifactBudget = core.ArtifactBudget
type TokenUsage = core.TokenUsage
type BudgetState = core.BudgetState
type Telemetry = core.Telemetry

const (
	BudgetNeedsCompression = core.BudgetNeedsCompression
	BudgetCritical         = core.BudgetCritical
)

type tokenUsageSource interface {
	GetTokenUsage() *core.TokenUsage
}

func EstimateArtifactTokens(source tokenUsageSource) int {
	if source == nil {
		return 0
	}
	usage := source.GetTokenUsage()
	if usage == nil {
		return 0
	}
	return usage.TotalTokens
}

var EstimateCodeTokens = core.EstimateCodeTokens
var EstimateTokens = core.EstimateTokens
