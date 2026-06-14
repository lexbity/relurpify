package observability

import (
	"context"
	"math"
	"sync"
	"time"
)

// EventType categorizes structured runtime events.
type EventType string

const (
	EventGraphStart            EventType = "graph_start"
	EventGraphFinish           EventType = "graph_finish"
	EventNodeStart             EventType = "node_start"
	EventNodeFinish            EventType = "node_finish"
	EventNodeError             EventType = "node_error"
	EventAgentStart            EventType = "agent_start"
	EventAgentFinish           EventType = "agent_finish"
	EventLLMPrompt             EventType = "llm_prompt"
	EventLLMResponse           EventType = "llm_response"
	EventDelegationStart       EventType = "delegation_start"
	EventDelegationFinish      EventType = "delegation_finish"
	EventDelegationCancel      EventType = "delegation_cancel"
	EventCapabilityCall        EventType = "capability_call"
	EventCapabilityResult      EventType = "capability_result"
	EventToolCall              EventType = "tool_call"
	EventToolResult            EventType = "tool_result"
	EventStateChange           EventType = "state_change"
	EventInferenceError        EventType = "inference_error"
	EventInferenceTimeout      EventType = "inference_timeout"
	EventInferenceAbort        EventType = "inference_abort"
	EventBackendStateChange    EventType = "backend_state_change"
	EventBackendWarm           EventType = "backend_warm"
	EventBackendClose          EventType = "backend_close"
	EventBackendRestart        EventType = "backend_restart"
	EventChunkCommitted        EventType = "chunk_committed"
	EventSummaryCommitted      EventType = "summary_committed"
	EventContextPolicyReloaded EventType = "context_policy_reloaded"
	EventProviderSessionEnded  EventType = "provider_session_ended"
	EventCompilerWarning       EventType = "compiler_warning"
	EventBootstrapComplete     EventType = "bootstrap_complete"
	EventBudgetSnapshot                  = "budget.snapshot"
	EventSessionResetRequired            = "session.reset_required"
)

// Actor identifies the origin of an event.
type Actor struct {
	Kind  string `json:"kind,omitempty"`
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
}

// Event captures structured runtime data.
type Event struct {
	Type      EventType      `json:"type"`
	NodeID    string         `json:"node_id,omitempty"`
	TaskID    string         `json:"task_id,omitempty"`
	Message   string         `json:"message,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Seq       uint64         `json:"seq,omitempty"`
	Partition string         `json:"partition,omitempty"`
	Payload   []byte         `json:"payload,omitempty"`
	Actor     Actor          `json:"actor,omitempty"`
}

// Telemetry captures structured events.
type Telemetry interface {
	Emit(event Event)
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
	IngestLLMResponse(ctx context.Context, resp any) error
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

// ContextBudgetAdvisor tracks consumed token budget across LLM calls and advises the compiler.
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

// WithAdvisor stores the advisor in the context.
func WithAdvisor(ctx context.Context, advisor *ContextBudgetAdvisor) context.Context {
	return WithUsageObserver(ctx, advisor)
}

// AdvisorFromContext extracts the advisor from context.
func AdvisorFromContext(ctx context.Context) *ContextBudgetAdvisor {
	obs := UsageObserverFromContext(ctx)
	advisor, _ := obs.(*ContextBudgetAdvisor)
	return advisor
}

// RecordTokenUsage implements UsageObserver.
func (a *ContextBudgetAdvisor) RecordTokenUsage(usage TokenUsage) { a.RecordCall(usage) }

// RecordCall updates internal accounting from an LLM response.
func (a *ContextBudgetAdvisor) RecordCall(usage TokenUsage) {
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
	a.lastPromptTokens = 0
	a.lastEstimated = false
	a.resetNotified = false
}

// ConsumeResetNotice implements UsageObserver.
func (a *ContextBudgetAdvisor) ConsumeResetNotice() (any, bool) {
	snap, ok := a.ConsumeResetNoticeTyped()
	if !ok {
		return nil, false
	}
	return snap, true
}

// ConsumeResetNoticeTyped is the typed variant for internal callers.
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

func (a *ContextBudgetAdvisor) availableCompilationBudgetLocked() int {
	reserved := a.reservedOutputTokensLocked()
	if a.ModelContextSize > 0 && a.estimatedCalls == 0 {
		return clampNonNegative(a.ModelContextSize - a.consumedTokens - reserved)
	}
	return clampNonNegative(reserved*2 - a.consumedTokens - 512)
}

func (a *ContextBudgetAdvisor) reservedOutputTokensLocked() int {
	if a.ReservedOutputTokens > 0 {
		return a.ReservedOutputTokens
	}
	return 512
}

func clampNonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// EstimateTokens estimates token usage for free-form text.
func EstimateTokens(v any) int {
	switch val := v.(type) {
	case string:
		return EstimateTextTokens(val)
	default:
		return 0
	}
}

// EstimateTextTokens estimates tokens from plain text.
func EstimateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	return max(1, int(math.Ceil(float64(len(text))/4.0)))
}

// EstimateCodeTokens estimates tokens for code snippets.
func EstimateCodeTokens(code string) int {
	if code == "" {
		return 0
	}
	return max(1, int(math.Ceil(float64(len(code))/2.5)))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
