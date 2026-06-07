package prompt

// PromptTelemetry is the telemetry sink for prompt resolution events.
// NewRegistry uses a no-op sink. NewRegistryWithTelemetry wires the real sink.
type PromptTelemetry interface {
	EmitPromptResolved(e ResolvedEvent)
	EmitPromptResolveFailed(e ResolveFailedEvent)
	EmitPromptContextMissing(e ContextMissingEvent)
	EmitPromptValidationIssue(e ValidationIssueEvent)
	EmitPromptProviderFailed(e ProviderFailedEvent)
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

func (noopTelemetry) EmitPromptResolved(ResolvedEvent)               {}
func (noopTelemetry) EmitPromptResolveFailed(ResolveFailedEvent)     {}
func (noopTelemetry) EmitPromptContextMissing(ContextMissingEvent)   {}
func (noopTelemetry) EmitPromptValidationIssue(ValidationIssueEvent) {}
func (noopTelemetry) EmitPromptProviderFailed(ProviderFailedEvent)   {}
