package contextbudget

import (
	"context"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ContextBudgetAdvisor tracks consumed token budget across LLM calls and
// advises the compiler on available compilation budget.
type ContextBudgetAdvisor struct {
	ModelContextSize     int
	ReservedOutputTokens int
	EstimationFallback   int

	mu               sync.Mutex
	consumedTokens   int
	callCount        int
	estimatedCalls   int
	lastPromptTokens int
	lastEstimated    bool
	resetNotified    bool
}

type advisorContextKey struct{}

// WithAdvisor stores the advisor in the context via the contracts.UsageObserver
// interface key so that platform/llm.InstrumentedModel can retrieve it without
// importing framework packages.
func WithAdvisor(ctx context.Context, advisor *ContextBudgetAdvisor) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return contracts.WithUsageObserver(ctx, advisor)
}

// AdvisorFromContext extracts the advisor from the contracts.UsageObserver context key.
func AdvisorFromContext(ctx context.Context) *ContextBudgetAdvisor {
	if ctx == nil {
		return nil
	}
	obs := contracts.UsageObserverFromContext(ctx)
	advisor, _ := obs.(*ContextBudgetAdvisor)
	return advisor
}

// RecordTokenUsage implements contracts.UsageObserver.
func (a *ContextBudgetAdvisor) RecordTokenUsage(usage contracts.TokenUsageReport) {
	a.RecordCall(usage)
}

// RecordCall updates internal accounting from an LLM response.
func (a *ContextBudgetAdvisor) RecordCall(usage contracts.TokenUsageReport) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.callCount++
	promptTokens := usage.PromptTokens
	if promptTokens == 0 {
		promptTokens = usage.TotalTokens
	}
	a.lastPromptTokens = promptTokens
	a.lastEstimated = usage.Estimated
	if usage.Estimated {
		a.estimatedCalls++
	}
	if promptTokens > 0 {
		a.consumedTokens += promptTokens
	}
}

// AvailableCompilationBudget returns the token count available to the compiler.
func (a *ContextBudgetAdvisor) AvailableCompilationBudget() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.availableCompilationBudgetLocked()
}

// ShouldReset reports whether the budget is close to exhaustion.
func (a *ContextBudgetAdvisor) ShouldReset() bool {
	if a == nil {
		return false
	}
	return a.AvailableCompilationBudget() < a.reservedOutputTokensLocked()*2
}

// Snapshot returns a point-in-time budget snapshot.
func (a *ContextBudgetAdvisor) Snapshot() BudgetSnapshot {
	if a == nil {
		return BudgetSnapshot{Timestamp: time.Now().UTC()}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	available := a.availableCompilationBudgetLocked()
	return BudgetSnapshot{
		ModelContextSize:     a.ModelContextSize,
		ConsumedTokens:       a.consumedTokens,
		ReservedOutputTokens: a.reservedOutputTokensLocked(),
		AvailableBudget:      available,
		CallCount:            a.callCount,
		EstimatedCallCount:   a.estimatedCalls,
		ShouldReset:          available < a.reservedOutputTokensLocked()*2,
		Timestamp:            time.Now().UTC(),
	}
}

// Reset clears consumed token accounting.
func (a *ContextBudgetAdvisor) Reset() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.consumedTokens = 0
	a.callCount = 0
	a.estimatedCalls = 0
	a.lastPromptTokens = 0
	a.lastEstimated = false
	a.resetNotified = false
}

// ConsumeResetNotice implements contracts.UsageObserver. Returns an opaque
// BudgetSnapshot value and true once per exhaustion cycle.
func (a *ContextBudgetAdvisor) ConsumeResetNotice() (any, bool) {
	snap, ok := a.ConsumeResetNoticeTyped()
	if !ok {
		return nil, false
	}
	return snap, true
}

// ConsumeResetNoticeTyped is the typed variant for framework-internal callers.
func (a *ContextBudgetAdvisor) ConsumeResetNoticeTyped() (BudgetSnapshot, bool) {
	if a == nil {
		return BudgetSnapshot{Timestamp: time.Now().UTC()}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	snapshot := BudgetSnapshot{
		ModelContextSize:     a.ModelContextSize,
		ConsumedTokens:       a.consumedTokens,
		ReservedOutputTokens: a.reservedOutputTokensLocked(),
		AvailableBudget:      a.availableCompilationBudgetLocked(),
		CallCount:            a.callCount,
		EstimatedCallCount:   a.estimatedCalls,
		ShouldReset:          a.availableCompilationBudgetLocked() < a.reservedOutputTokensLocked()*2,
		Timestamp:            time.Now().UTC(),
	}
	if !snapshot.ShouldReset {
		a.resetNotified = false
		return snapshot, false
	}
	if a.resetNotified {
		return snapshot, false
	}
	a.resetNotified = true
	return snapshot, true
}

// BudgetSnapshot is an observability snapshot of the budget advisor.
type BudgetSnapshot struct {
	ModelContextSize     int       `json:"model_context_size"`
	ConsumedTokens       int       `json:"consumed_tokens"`
	ReservedOutputTokens int       `json:"reserved_output_tokens"`
	AvailableBudget      int       `json:"available_budget"`
	CallCount            int       `json:"call_count"`
	EstimatedCallCount   int       `json:"estimated_call_count"`
	ShouldReset          bool      `json:"should_reset"`
	Timestamp            time.Time `json:"timestamp"`
}

func (a *ContextBudgetAdvisor) availableCompilationBudgetLocked() int {
	reserved := a.reservedOutputTokensLocked()
	fallback := a.estimationFallbackLocked()
	if a.ModelContextSize > 0 && !a.lastEstimated {
		return clampNonNegative(a.ModelContextSize - a.consumedTokens - reserved)
	}
	return clampNonNegative(fallback - a.lastPromptTokens - reserved)
}

func (a *ContextBudgetAdvisor) reservedOutputTokensLocked() int {
	if a.ReservedOutputTokens > 0 {
		return a.ReservedOutputTokens
	}
	return 512
}

func (a *ContextBudgetAdvisor) estimationFallbackLocked() int {
	if a.EstimationFallback > 0 {
		return a.EstimationFallback
	}
	return 4096
}

func clampNonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
