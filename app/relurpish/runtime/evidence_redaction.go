package runtime

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/telemetry"
)

const (
	metadataPromptID = "prompt_id"
	metadataBlockID  = "block_id"
)

// RedactionPolicy defines what data is redacted in evidence.
type RedactionPolicy struct {
	// RedactSecrets controls whether known secret values are redacted.
	RedactSecrets bool

	// RedactPromptBodies controls whether prompt bodies are redacted.
	RedactPromptBodies bool

	// RedactResponseBodies controls whether response bodies are redacted.
	RedactResponseBodies bool

	// MaxPreviewLength is the maximum length for previews (0 = no preview).
	MaxPreviewLength int

	// SecretPatterns are patterns that should be redacted (e.g., API keys, passwords).
	SecretPatterns []string
}

// DefaultRedactionPolicy returns the default redaction policy.
// By default, secrets are redacted but previews are allowed with limits.
func DefaultRedactionPolicy() RedactionPolicy {
	return RedactionPolicy{
		RedactSecrets:        true,
		RedactPromptBodies:   false, // Allow previews by default
		RedactResponseBodies: false,
		MaxPreviewLength:     256,
		SecretPatterns: []string{
			"api_key",
			"apikey",
			"password",
			"secret",
			"token",
			"credential",
			"auth",
		},
	}
}

// StrictRedactionPolicy returns a policy that redacts all bodies.
func StrictRedactionPolicy() RedactionPolicy {
	return RedactionPolicy{
		RedactSecrets:        true,
		RedactPromptBodies:   true,
		RedactResponseBodies: true,
		MaxPreviewLength:     0,
		SecretPatterns: []string{
			"api_key",
			"apikey",
			"password",
			"secret",
			"token",
			"credential",
			"auth",
		},
	}
}

// EvidenceRedactor redacts sensitive data from evidence.
type EvidenceRedactor struct {
	mu     sync.RWMutex
	policy RedactionPolicy

	// secretCache caches known secret hashes for efficient redaction.
	secretCache map[string]struct{}

	// hmacKey is used to generate HMACs for redacted content.
	hmacKey []byte
}

// NewEvidenceRedactor creates a new evidence redactor with the given policy.
func NewEvidenceRedactor(policy RedactionPolicy) *EvidenceRedactor {
	return &EvidenceRedactor{
		policy:      policy,
		secretCache: make(map[string]struct{}),
		hmacKey:     generateRandomKey(),
	}
}

// generateRandomKey generates a random 32-byte key for HMAC.
func generateRandomKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("evidence redaction: key entropy unavailable: %v", err))
	}
	return key
}

// SetPolicy updates the redaction policy.
func (r *EvidenceRedactor) SetPolicy(policy RedactionPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy = policy
}

// Policy returns the current redaction policy.
func (r *EvidenceRedactor) Policy() RedactionPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.policy
}

// RedactPromptEvidence redacts sensitive data from prompt evidence.
func (r *EvidenceRedactor) RedactPromptEvidence(ev *PromptEvidence) *PromptEvidence {
	if r == nil || ev == nil {
		return ev
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	redacted := *ev

	// Redact source reference if it contains secrets
	if r.policy.RedactSecrets {
		redacted.SourceReference = r.redactString(ev.SourceReference)
	}

	// Redact block sources
	for i := range redacted.IncludedBlocks {
		redacted.IncludedBlocks[i].Source = r.redactString(ev.IncludedBlocks[i].Source)
		redacted.IncludedBlocks[i].Reason = r.redactString(ev.IncludedBlocks[i].Reason)
	}
	for i := range redacted.ExcludedBlocks {
		redacted.ExcludedBlocks[i].Source = r.redactString(ev.ExcludedBlocks[i].Source)
		redacted.ExcludedBlocks[i].Reason = r.redactString(ev.ExcludedBlocks[i].Reason)
	}

	// Redact providers
	for i := range redacted.ProvidersUsed {
		redacted.ProvidersUsed[i] = r.redactString(ev.ProvidersUsed[i])
	}

	// Redact error messages
	redacted.Error = r.redactString(ev.Error)

	return &redacted
}

// RedactModelAttemptEvidence redacts sensitive data from model attempt evidence.
func (r *EvidenceRedactor) RedactModelAttemptEvidence(ev *ModelAttemptEvidence) *ModelAttemptEvidence {
	if r == nil || ev == nil {
		return ev
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	redacted := *ev

	// Redact model and provider names if they contain secrets
	if r.policy.RedactSecrets {
		redacted.Model = r.redactString(ev.Model)
		redacted.Provider = r.redactString(ev.Provider)
		redacted.ProfileRevision = r.redactString(ev.ProfileRevision)
		redacted.ToolCallingMode = r.redactString(ev.ToolCallingMode)
		redacted.FinishReason = r.redactString(ev.FinishReason)
	}

	// Redact prompt preview if policy requires
	if r.policy.RedactPromptBodies {
		redacted.PromptPreview = "[REDACTED]"
	} else if r.policy.MaxPreviewLength > 0 && len(redacted.PromptPreview) > r.policy.MaxPreviewLength {
		redacted.PromptPreview = redacted.PromptPreview[:r.policy.MaxPreviewLength] + "...[truncated]"
	}

	// Redact error messages
	redacted.Error = r.redactString(ev.Error)

	return &redacted
}

// RedactKnowledgeFlowEvidence redacts sensitive data from knowledge flow evidence.
func (r *EvidenceRedactor) RedactKnowledgeFlowEvidence(ev *KnowledgeFlowEvidence) *KnowledgeFlowEvidence {
	if r == nil || ev == nil {
		return ev
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	redacted := *ev

	// Redact IDs and hashes if they contain secrets
	if r.policy.RedactSecrets {
		redacted.ID = r.redactID(ev.ID)
		redacted.ChunkID = r.redactID(ev.ChunkID)
		redacted.ContentHash = r.redactString(ev.ContentHash)
		redacted.CompilerRevision = r.redactString(ev.CompilerRevision)
		redacted.Error = r.redactString(ev.Error)
	}

	// Redact source and target identities
	for i := range redacted.SourceIdentities {
		redacted.SourceIdentities[i] = r.redactID(ev.SourceIdentities[i])
	}
	for i := range redacted.TargetIdentities {
		redacted.TargetIdentities[i] = r.redactID(ev.TargetIdentities[i])
	}

	// Redact lineage
	for i := range redacted.Lineage {
		redacted.Lineage[i].Step = r.redactString(ev.Lineage[i].Step)
		redacted.Lineage[i].Operation = r.redactString(ev.Lineage[i].Operation)
		for j := range redacted.Lineage[i].Inputs {
			redacted.Lineage[i].Inputs[j] = r.redactID(ev.Lineage[i].Inputs[j])
		}
		for j := range redacted.Lineage[i].Outputs {
			redacted.Lineage[i].Outputs[j] = r.redactID(ev.Lineage[i].Outputs[j])
		}
	}

	return &redacted
}

// RedactCallEvidenceBundle redacts all evidence in a call bundle.
func (r *EvidenceRedactor) RedactCallEvidenceBundle(bundle *CallEvidenceBundle) *CallEvidenceBundle {
	if r == nil || bundle == nil {
		return bundle
	}

	redacted := &CallEvidenceBundle{
		CallID: bundle.CallID,
	}

	if bundle.ModelAttempt != nil {
		redacted.ModelAttempt = r.RedactModelAttemptEvidence(bundle.ModelAttempt)
	}

	if bundle.PromptPreview != nil {
		redacted.PromptPreview = r.RedactPromptEvidence(bundle.PromptPreview)
	}

	if bundle.PromptRuntime != nil {
		redacted.PromptRuntime = r.RedactPromptEvidence(bundle.PromptRuntime)
	}

	if len(bundle.KnowledgeFlows) > 0 {
		redacted.KnowledgeFlows = make([]*KnowledgeFlowEvidence, 0, len(bundle.KnowledgeFlows))
		for _, flow := range bundle.KnowledgeFlows {
			redacted.KnowledgeFlows = append(redacted.KnowledgeFlows, r.RedactKnowledgeFlowEvidence(flow))
		}
	}

	return redacted
}

// redactString redacts sensitive patterns from a string.
func (r *EvidenceRedactor) redactString(s string) string {
	if s == "" || !r.policy.RedactSecrets {
		return s
	}

	// Check for secret patterns
	lower := strings.ToLower(s)
	for _, pattern := range r.policy.SecretPatterns {
		if strings.Contains(lower, pattern) {
			return "[REDACTED]"
		}
	}

	return s
}

// redactID redacts an ID if it looks like it might contain sensitive data.
func (r *EvidenceRedactor) redactID(id string) string {
	if id == "" || !r.policy.RedactSecrets {
		return id
	}

	// For now, just redact IDs that look like they might be sensitive
	// In a real implementation, this would check against known secret patterns
	if strings.Contains(id, "key_") || strings.Contains(id, "token_") || strings.Contains(id, "secret_") {
		return "[REDACTED_ID]"
	}

	return id
}

// GenerateContentHMAC generates an HMAC for content verification.
// This allows verification that redacted content corresponds to the original
// without exposing the original content.
func (r *EvidenceRedactor) GenerateContentHMAC(content string) string {
	if r == nil || content == "" {
		return ""
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	h := hmac.New(sha256.New, r.hmacKey)
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyContentHMAC verifies that content matches the expected HMAC.
func (r *EvidenceRedactor) VerifyContentHMAC(content, expectedHMAC string) bool {
	if r == nil || content == "" || expectedHMAC == "" {
		return false
	}

	actualHMAC := r.GenerateContentHMAC(content)
	return hmac.Equal([]byte(actualHMAC), []byte(expectedHMAC))
}

// RedactionMiddleware wraps an EvidenceRegistry and applies redaction to all evidence.
type RedactionMiddleware struct {
	registry *EvidenceRegistry
	redactor *EvidenceRedactor
}

// NewRedactionMiddleware creates a new redaction middleware.
func NewRedactionMiddleware(registry *EvidenceRegistry, policy RedactionPolicy) *RedactionMiddleware {
	return &RedactionMiddleware{
		registry: registry,
		redactor: NewEvidenceRedactor(policy),
	}
}

// SetPolicy updates the redaction policy.
func (m *RedactionMiddleware) SetPolicy(policy RedactionPolicy) {
	if m.redactor != nil {
		m.redactor.SetPolicy(policy)
	}
}

// GetPromptEvidence retrieves and redacts prompt evidence.
func (m *RedactionMiddleware) GetPromptEvidence(ctx context.Context, promptID string, kind EvidenceKind) (*PromptEvidence, error) {
	ev, err := m.registry.GetPromptEvidence(ctx, promptID, kind)
	if err != nil {
		return nil, err
	}
	return m.redactor.RedactPromptEvidence(ev), nil
}

// GetModelAttemptEvidence retrieves and redacts model attempt evidence.
func (m *RedactionMiddleware) GetModelAttemptEvidence(ctx context.Context, callID string) (*ModelAttemptEvidence, error) {
	ev, err := m.registry.GetModelAttemptEvidence(ctx, callID)
	if err != nil {
		return nil, err
	}
	return m.redactor.RedactModelAttemptEvidence(ev), nil
}

// GetKnowledgeFlowEvidence retrieves and redacts knowledge flow evidence.
func (m *RedactionMiddleware) GetKnowledgeFlowEvidence(ctx context.Context, operationID string) (*KnowledgeFlowEvidence, error) {
	ev, err := m.registry.GetKnowledgeFlowEvidence(ctx, operationID)
	if err != nil {
		return nil, err
	}
	return m.redactor.RedactKnowledgeFlowEvidence(ev), nil
}

// GetEvidenceByCallID retrieves and redacts all evidence for a call.
func (m *RedactionMiddleware) GetEvidenceByCallID(ctx context.Context, callID string) (*CallEvidenceBundle, error) {
	bundle, err := m.registry.GetEvidenceByCallID(ctx, callID)
	if err != nil {
		return nil, err
	}
	return m.redactor.RedactCallEvidenceBundle(bundle), nil
}

// GetAllEvidence retrieves and redacts all evidence.
func (m *RedactionMiddleware) GetAllEvidence(ctx context.Context) (*CompleteEvidence, error) {
	ev, err := m.registry.GetAllEvidence(ctx)
	if err != nil {
		return nil, err
	}

	redacted := &CompleteEvidence{
		Prompts:     make([]*PromptEvidence, 0, len(ev.Prompts)),
		Calls:       make([]*ModelAttemptEvidence, 0, len(ev.Calls)),
		Knowledge:   make([]*KnowledgeFlowEvidence, 0, len(ev.Knowledge)),
		CollectedAt: ev.CollectedAt,
		StartedAt:   ev.StartedAt,
	}

	for _, p := range ev.Prompts {
		redacted.Prompts = append(redacted.Prompts, m.redactor.RedactPromptEvidence(p))
	}

	for _, c := range ev.Calls {
		redacted.Calls = append(redacted.Calls, m.redactor.RedactModelAttemptEvidence(c))
	}

	for _, k := range ev.Knowledge {
		redacted.Knowledge = append(redacted.Knowledge, m.redactor.RedactKnowledgeFlowEvidence(k))
	}

	return redacted, nil
}

// Subscribe subscribes to evidence events with redaction applied.
func (m *RedactionMiddleware) Subscribe(bufferSize int) <-chan EvidenceEvent {
	ch := m.registry.Subscribe(bufferSize)

	// Create a new channel that applies redaction
	redactedCh := make(chan EvidenceEvent, bufferSize)

	go func() {
		defer close(redactedCh)
		for ev := range ch {
			redactedEv := EvidenceEvent{
				Kind:      ev.Kind,
				Timestamp: ev.Timestamp,
			}

			if ev.PromptEvidence != nil {
				redactedEv.PromptEvidence = m.redactor.RedactPromptEvidence(ev.PromptEvidence)
			}
			if ev.ModelAttemptEvidence != nil {
				redactedEv.ModelAttemptEvidence = m.redactor.RedactModelAttemptEvidence(ev.ModelAttemptEvidence)
			}
			if ev.KnowledgeFlowEvidence != nil {
				redactedEv.KnowledgeFlowEvidence = m.redactor.RedactKnowledgeFlowEvidence(ev.KnowledgeFlowEvidence)
			}

			select {
			case redactedCh <- redactedEv:
			default:
				// Drop if channel is full
			}
		}
	}()

	return redactedCh
}

// Close closes the middleware.
func (m *RedactionMiddleware) Close() error {
	if m.registry != nil {
		return m.registry.Close()
	}
	return nil
}

// RegisterWithTelemetry registers a prompt telemetry bridge that forwards
// prompt resolution events to the main telemetry system.
// This ensures prompt events are captured in the evidence registry.
type PromptTelemetryBridge struct {
	sink telemetry.Telemetry
}

// NewPromptTelemetryBridge creates a new bridge that forwards prompt events.
func NewPromptTelemetryBridge(sink telemetry.Telemetry) *PromptTelemetryBridge {
	return &PromptTelemetryBridge{sink: sink}
}

// EmitPromptResolved implements prompt.PromptTelemetry.
func (b *PromptTelemetryBridge) EmitPromptResolved(ctx context.Context, e prompt.ResolvedEvent) {
	if b == nil || b.sink == nil {
		return
	}

	// Convert prompt resolved event to telemetry event
	metadata := map[string]any{
		metadataPromptID:  e.ID,
		"paradigm":        e.Paradigm,
		"output_length":   e.OutputLength,
		"blocks_included": e.BlocksIncluded,
		"blocks_excluded": e.BlocksExcluded,
		"providers_used":  e.ProvidersUsed,
		"duration_ms":     e.DurationMs,
		"cache_hit":       e.CacheHit,
	}

	ev := telemetry.Event{
		Type:      telemetry.EventLLMPrompt,
		Timestamp: time.Now().UTC(),
		Message:   "prompt resolved",
		Metadata:  metadata,
	}

	telemetry.StampCorrelation(ctx, &ev)
	b.sink.Emit(ev)
}

// EmitPromptResolveFailed implements prompt.PromptTelemetry.
func (b *PromptTelemetryBridge) EmitPromptResolveFailed(ctx context.Context, e prompt.ResolveFailedEvent) {
	if b == nil || b.sink == nil {
		return
	}

	metadata := map[string]any{
		metadataPromptID: e.ID,
		"paradigm":       e.Paradigm,
		"error":          e.Error,
		"duration_ms":    e.DurationMs,
	}

	ev := telemetry.Event{
		Type:      telemetry.EventLLMResponse,
		Timestamp: time.Now().UTC(),
		Message:   "prompt resolve failed",
		Metadata:  metadata,
	}

	telemetry.StampCorrelation(ctx, &ev)
	b.sink.Emit(ev)
}

// EmitPromptContextMissing implements prompt.PromptTelemetry.
func (b *PromptTelemetryBridge) EmitPromptContextMissing(ctx context.Context, e prompt.ContextMissingEvent) {
	if b == nil || b.sink == nil {
		return
	}

	metadata := map[string]any{
		metadataPromptID: e.PromptID,
		metadataBlockID:  e.BlockID,
		"key":            e.Key,
		"message":        e.Message,
	}

	ev := telemetry.Event{
		Type:      telemetry.EventCompilerWarning,
		Timestamp: time.Now().UTC(),
		Message:   "prompt context missing",
		Metadata:  metadata,
	}

	telemetry.StampCorrelation(ctx, &ev)
	b.sink.Emit(ev)
}

// EmitPromptValidationIssue implements prompt.PromptTelemetry.
func (b *PromptTelemetryBridge) EmitPromptValidationIssue(ctx context.Context, e prompt.ValidationIssueEvent) {
	if b == nil || b.sink == nil {
		return
	}

	metadata := map[string]any{
		metadataPromptID: e.Issue.PromptID,
		metadataBlockID:  e.Issue.BlockID,
		"issue_message":  e.Issue.Message,
		"issue_severity": e.Issue.Severity.String(),
	}

	ev := telemetry.Event{
		Type:      telemetry.EventCompilerWarning,
		Timestamp: time.Now().UTC(),
		Message:   "prompt validation issue",
		Metadata:  metadata,
	}

	telemetry.StampCorrelation(ctx, &ev)
	b.sink.Emit(ev)
}

// EmitPromptProviderFailed implements prompt.PromptTelemetry.
func (b *PromptTelemetryBridge) EmitPromptProviderFailed(ctx context.Context, e prompt.ProviderFailedEvent) {
	if b == nil || b.sink == nil {
		return
	}

	metadata := map[string]any{
		metadataPromptID: e.PromptID,
		metadataBlockID:  e.BlockID,
		"provider_name":  e.ProviderName,
		"error":          e.Error,
	}

	ev := telemetry.Event{
		Type:      telemetry.EventInferenceError,
		Timestamp: time.Now().UTC(),
		Message:   "prompt provider failed",
		Metadata:  metadata,
	}

	telemetry.StampCorrelation(ctx, &ev)
	b.sink.Emit(ev)
}
