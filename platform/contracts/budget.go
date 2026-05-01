package contracts

// BudgetItem describes any piece of streamed runtime state (file snippet,
// history chunk, etc.) that competes for the limited token budget.
type BudgetItem interface {
	GetID() string
	GetTokenCount() int
	GetPriority() int
	CanCompress() bool
	Compress() (BudgetItem, error)
	CanEvict() bool
}

// BudgetManager defines the interface for artifact budget management.
// This is implemented by framework/contextbudget.ArtifactBudget.
type BudgetManager interface {
	Allocate(category string, tokens int, item BudgetItem) error
	Free(category string, tokens int, itemID string)
	GetRemainingBudget(category string) int
	ShouldCompress() bool
	CanAddTokens(tokens int) bool
}

// ArtifactBudget is an alias for BudgetManager for backward compatibility.
// Use BudgetManager for new code.
type ArtifactBudget = BudgetManager

// EstimateTokens estimates the token count for a value.
// This is a simplified version; the framework provides the actual implementation.
func EstimateTokens(v interface{}) int {
	// Simple estimation: ~4 chars per token for string content
	switch val := v.(type) {
	case string:
		return len(val) / 4
	case []byte:
		return len(val) / 4
	default:
		return 0
	}
}

// TokenUsage captures the token accounting snapshot for artifact budgeting.
type TokenUsage struct {
	SystemTokens         int
	ToolTokens           int
	ArtifactTokens       int
	OutputTokens         int
	TotalTokens          int
	ArtifactUsagePercent float64
}
