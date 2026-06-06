package telemetry

import (
	"context"
	"time"
)

// EventLog is the append-only event log interface.
type EventLog interface {
	Append(ctx context.Context, partition string, events []Event) ([]uint64, error)
	Read(ctx context.Context, partition string, afterSeq uint64, limit int, follow bool) ([]Event, error)
	ReadByType(ctx context.Context, partition string, typePrefix string, afterSeq uint64, limit int) ([]Event, error)
	LastSeq(ctx context.Context, partition string) (uint64, error)
	TakeSnapshot(ctx context.Context, partition string, seq uint64, data []byte) error
	LoadSnapshot(ctx context.Context, partition string) (uint64, []byte, error)
	Close() error
}

// UsageObserver records token usage from LLM calls.
type UsageObserver interface {
	RecordTokenUsage(usage TokenUsage)
	ConsumeResetNotice() (any, bool)
}

// SnapshotObserver records periodic budget snapshots.
type SnapshotObserver interface {
	Observe()
}

// ResponseIngester indexes LLM responses into the knowledge graph.
type ResponseIngester interface {
	IngestLLMResponse(ctx context.Context, resp interface{}) error
}

// TokenUsage records token consumption for a model invocation.
type TokenUsage struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Estimated        bool   `json:"estimated"`
	EstimationMethod string `json:"estimation_method,omitempty"`
}

// BudgetItem is a single item in a context budget.
type BudgetItem interface {
	GetID() string
	GetTokenCount() int
	GetPriority() int
	CanCompress() bool
	Compress() (BudgetItem, error)
	CanEvict() bool
}

// BudgetManager manages context budgets for categories.
type BudgetManager interface {
	Allocate(category string, tokens int, item BudgetItem) error
	Free(category string, tokens int, itemID string)
	GetRemainingBudget(category string) int
	ShouldCompress() bool
	CanAddTokens(tokens int) bool
}

// EventLog-related context keys for telemetry context.
type usageObserverKey struct{}
type snapshotObserverKey struct{}
type responseIngesterKey struct{}

// WithUsageObserver attaches a UsageObserver to the context.
func WithUsageObserver(ctx context.Context, obs UsageObserver) context.Context {
	return context.WithValue(ctx, usageObserverKey{}, obs)
}

// UsageObserverFromContext extracts the UsageObserver from context, or nil.
func UsageObserverFromContext(ctx context.Context) UsageObserver {
	v, _ := ctx.Value(usageObserverKey{}).(UsageObserver)
	return v
}

// WithSnapshotObserver attaches a SnapshotObserver to the context.
func WithSnapshotObserver(ctx context.Context, obs SnapshotObserver) context.Context {
	return context.WithValue(ctx, snapshotObserverKey{}, obs)
}

// SnapshotObserverFromContext extracts the SnapshotObserver from context, or nil.
func SnapshotObserverFromContext(ctx context.Context) SnapshotObserver {
	v, _ := ctx.Value(snapshotObserverKey{}).(SnapshotObserver)
	return v
}

// WithResponseIngester attaches a ResponseIngester to the context.
func WithResponseIngester(ctx context.Context, ing ResponseIngester) context.Context {
	return context.WithValue(ctx, responseIngesterKey{}, ing)
}

// ResponseIngesterFromContext extracts the ResponseIngester from context, or nil.
func ResponseIngesterFromContext(ctx context.Context) ResponseIngester {
	v, _ := ctx.Value(responseIngesterKey{}).(ResponseIngester)
	return v
}

// PerfStats aggregates performance statistics per capability/tool.
type PerfStats struct {
	TotalInvocations int
	TotalDuration    time.Duration
	MinDuration      time.Duration
	MaxDuration      time.Duration
	ErrorCount       int
}

// Record records a single invocation duration for the stats.
func (s *PerfStats) Record(duration time.Duration, err error) {
	s.TotalInvocations++
	s.TotalDuration += duration
	if s.MinDuration == 0 || duration < s.MinDuration {
		s.MinDuration = duration
	}
	if duration > s.MaxDuration {
		s.MaxDuration = duration
	}
	if err != nil {
		s.ErrorCount++
	}
}

// AverageDuration returns the average invocation duration.
func (s *PerfStats) AverageDuration() time.Duration {
	if s.TotalInvocations == 0 {
		return 0
	}
	return s.TotalDuration / time.Duration(s.TotalInvocations)
}

// SuccessRate returns the fraction of invocations that succeeded.
func (s *PerfStats) SuccessRate() float64 {
	if s.TotalInvocations == 0 {
		return 0
	}
	return float64(s.TotalInvocations-s.ErrorCount) / float64(s.TotalInvocations)
}

// TokenUsageReport is an alias for backward compatibility.
type TokenUsageReport = TokenUsage
