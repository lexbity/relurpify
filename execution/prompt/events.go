package prompt

import "context"

// PromptTelemetry is the telemetry sink for prompt resolution events.
// NewRegistry uses a no-op sink. NewRegistryWithTelemetry wires the real sink.
// All methods accept a context for correlation stamping.
type PromptTelemetry interface {
	EmitPromptResolved(ctx context.Context, e ResolvedEvent)
	EmitPromptResolveFailed(ctx context.Context, e ResolveFailedEvent)
	EmitPromptContextMissing(ctx context.Context, e ContextMissingEvent)
	EmitPromptValidationIssue(ctx context.Context, e ValidationIssueEvent)
	EmitPromptProviderFailed(ctx context.Context, e ProviderFailedEvent)
}

// ResolvedEvent is emitted after a successful prompt resolution.
type ResolvedEvent struct {
	ID             string
	Paradigm       string
	OutputLength   int
	BlocksIncluded int
	BlocksExcluded int
	ProvidersUsed  []string
	DurationMs     int64
	CacheHit       bool
}

// ResolveFailedEvent is emitted when Resolve returns an error.
type ResolveFailedEvent struct {
	ID         string
	Paradigm   string
	Error      string
	DurationMs int64
}

// ContextMissingEvent is emitted when a when-expression references an undefined
// state key, or a provider block references an unregistered provider.
type ContextMissingEvent struct {
	PromptID string
	BlockID  string
	Key      string
	Message  string
}

// ValidationIssueEvent is emitted for each ValidationIssue found during load or
// ValidateProviders.
type ValidationIssueEvent struct {
	Issue ValidationIssue
}

// ProviderFailedEvent is emitted when a FailableProvider returns an error.
type ProviderFailedEvent struct {
	PromptID     string
	BlockID      string
	ProviderName string
	Error        string
}

// noopTelemetry is the default no-op sink used when no telemetry is provided.
type noopTelemetry struct{}

func (noopTelemetry) EmitPromptResolved(context.Context, ResolvedEvent)               {}
func (noopTelemetry) EmitPromptResolveFailed(context.Context, ResolveFailedEvent)     {}
func (noopTelemetry) EmitPromptContextMissing(context.Context, ContextMissingEvent)   {}
func (noopTelemetry) EmitPromptValidationIssue(context.Context, ValidationIssueEvent) {}
func (noopTelemetry) EmitPromptProviderFailed(context.Context, ProviderFailedEvent)   {}
