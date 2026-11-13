// Package runtime provides evidence extraction from existing telemetry data.
// This implements Phase 6 (P06) requirements: actual prompt, model-attempt,
// and knowledge-flow evidence using ONLY existing OpenTelemetry sources.
//
// The evidence service consumes telemetry events and provides structured
// access to prompt provenance, model attempt details, and knowledge flow
// observations without adding new instrumentation.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// EvidenceError types for P06 evidence operations.
var (
	// ErrEvidenceNotFound is returned when requested evidence doesn't exist.
	ErrEvidenceNotFound = errors.New("evidence: not found")

	// ErrEvidenceUnavailable is returned when evidence is not yet available.
	ErrEvidenceUnavailable = errors.New("evidence: unavailable")

	// ErrNoActualCall is returned when only preview data exists, no actual call.
	ErrNoActualCall = errors.New("evidence: no actual call recorded")

	// ErrPreviewOnly is returned when evidence is only from deterministic preview.
	ErrPreviewOnly = errors.New("evidence: preview only, no runtime data")
)

// EvidenceKind identifies the type of evidence.
type EvidenceKind string

const (
	// EvidenceKindPreview indicates deterministic preview resolution.
	// This consumes zero model requests and is based on draft context.
	EvidenceKindPreview EvidenceKind = "preview"

	// EvidenceKindRuntime indicates actual runtime resolution.
	// This is the real resolution that occurred during execution.
	EvidenceKindRuntime EvidenceKind = "runtime"

	// EvidenceKindActualCall indicates final transport-level evidence.
	// This is captured at the transport request boundary with redaction.
	EvidenceKindActualCall EvidenceKind = "actual_call"
)

// PromptEvidence contains evidence about prompt resolution and usage.
type PromptEvidence struct {
	// ID is the prompt identifier.
	ID string

	// Kind distinguishes preview, runtime resolution, or actual call.
	Kind EvidenceKind

	// SourceReference identifies the source prompt file or registry entry.
	SourceReference string

	// SourceRevision is the revision/digest of the source at resolution time.
	SourceRevision string

	// ResolvedAt is when resolution occurred.
	ResolvedAt time.Time

	// IncludedBlocks lists blocks that were included in resolution.
	IncludedBlocks []BlockEvidence

	// ExcludedBlocks lists blocks that were excluded.
	ExcludedBlocks []BlockEvidence

	// OutputLength is the length of the resolved prompt output.
	OutputLength int

	// DurationMS is the resolution duration in milliseconds.
	DurationMS int64

	// Error contains any resolution error.
	Error string

	// CallID links to the logical call that used this prompt, if any.
	CallID string

	// AttemptID links to the physical attempt, if any.
	AttemptID string

	// ProvidersUsed lists the providers used in resolution.
	ProvidersUsed []string

	// CacheHit indicates whether the resolution used a cache.
	CacheHit bool

	// ResolveDurationMS is the total resolution time.
	ResolveDurationMS int64
}

// BlockEvidence contains evidence about a prompt block's inclusion/exclusion.
type BlockEvidence struct {
	// BlockID is the block identifier.
	BlockID string

	// Source is the source location of the block.
	Source string

	// Order is the block's position in the source.
	Order int

	// Reason explains why the block was included or excluded.
	Reason string
}

// ModelAttemptEvidence contains evidence about a model call attempt.
type ModelAttemptEvidence struct {
	// CallID is the logical call identifier.
	CallID string

	// AttemptID is the physical attempt identifier.
	AttemptID string

	// Kind is always EvidenceKindActualCall for this type.
	Kind EvidenceKind

	// Model identifies the model used.
	Model string

	// Provider identifies the provider.
	Provider string

	// ProfileRevision is the profile revision used.
	ProfileRevision string

	// PromptID links to the resolved prompt evidence.
	PromptID string

	// InputTokens is the estimated input token count.
	InputTokens int

	// RequestArtifactID references the redacted request artifact.
	RequestArtifactID string

	// ResponseArtifactID references the response artifact.
	ResponseArtifactID string

	// FinishReason is the model's finish reason.
	FinishReason string

	// ToolCallingMode indicates how tools were configured.
	ToolCallingMode string

	// OutputTokens is the estimated output token count.
	OutputTokens int

	// DurationMS is the attempt duration in milliseconds.
	DurationMS int64

	// StartedAt is when the attempt started.
	StartedAt time.Time

	// CompletedAt is when the attempt completed.
	CompletedAt time.Time

	// Error contains any attempt error.
	Error string

	// TransportReclaimed indicates whether the transport was properly reclaimed.
	TransportReclaimed bool

	// LanePosition is the queue position when this attempt was made.
	LanePosition int64

	// PromptPreview is a truncated preview of the prompt (if available and allowed).
	PromptPreview string

	// MessageCount is the number of messages in the chat (for chat-based calls).
	MessageCount int

	// Roles lists the message roles (for chat-based calls).
	Roles []string
}

// KnowledgeFlowEvidence contains evidence about knowledge operations.
type KnowledgeFlowEvidence struct {
	// ID identifies this knowledge flow.
	ID string

	// Kind identifies the flow type (retrieve, compile, apply, capture, ground, persist).
	Kind string

	// Direction is "input" or "output".
	Direction string

	// Operation identifies the specific operation (e.g., "retrieve", "ground").
	Operation string

	// SourceIdentities lists the source knowledge identities.
	SourceIdentities []string

	// TargetIdentities lists the target knowledge identities.
	TargetIdentities []string

	// BudgetUsed is the knowledge budget consumed.
	BudgetUsed int

	// ItemsSelected is the number of items selected/retrieved.
	ItemsSelected int

	// CompilerRevision is the compiler revision used.
	CompilerRevision string

	// Lineage contains the provenance chain.
	Lineage []LineageEntry

	// DurationMS is the operation duration.
	DurationMS int64

	// StartedAt is when the operation started.
	StartedAt time.Time

	// CompletedAt is when the operation completed.
	CompletedAt time.Time

	// Error contains any operation error.
	Error string

	// ChunkID references the chunk if this is a chunk-related operation.
	ChunkID string

	// ContentHash is the content hash for the operation.
	ContentHash string

	// TokenEstimate is the token estimate for the operation.
	TokenEstimate int

	// SessionID is the session ID for the operation.
	SessionID string

	// WorkflowID is the workflow ID for the operation.
	WorkflowID string

	// NodeID is the node ID for the operation.
	NodeID string
}

// LineageEntry represents one link in the knowledge provenance chain.
type LineageEntry struct {
	// Step is the step identifier.
	Step string

	// Operation is the operation performed.
	Operation string

	// Inputs are the input identities.
	Inputs []string

	// Outputs are the output identities.
	Outputs []string

	// Timestamp is when this step occurred.
	Timestamp time.Time
}

// EvidenceRegistry stores and indexes evidence from telemetry events.
// It consumes events from the existing OpenTelemetry infrastructure and
// provides structured access to prompt, model attempt, and knowledge flow evidence.
type EvidenceRegistry struct {
	mu sync.RWMutex

	// prompts stores prompt evidence by prompt ID and kind.
	prompts map[string]map[EvidenceKind]*PromptEvidence

	// calls stores model attempt evidence by call ID.
	calls map[string]*ModelAttemptEvidence

	// knowledge stores knowledge flow evidence by operation ID.
	knowledge map[string]*KnowledgeFlowEvidence

	// callByPrompt maps prompt IDs to their associated call IDs.
	callByPrompt map[string]string

	// promptByCall maps call IDs to their associated prompt IDs.
	promptByCall map[string]string

	// knowledgeByCall maps call IDs to their associated knowledge flow IDs.
	knowledgeByCall map[string][]string

	// telemetrySink is the source of telemetry events.
	telemetrySink telemetry.Telemetry

	// subscriptions for new evidence.
	subscriptions []chan EvidenceEvent

	// closed indicates if the registry is closed.
	closed bool

	// startTime tracks when the registry was started.
	startTime time.Time
}

// EvidenceEvent represents a new piece of evidence.
type EvidenceEvent struct {
	// Kind identifies the evidence type.
	Kind string

	// PromptEvidence is set if Kind is "prompt".
	PromptEvidence *PromptEvidence

	// ModelAttemptEvidence is set if Kind is "model_attempt".
	ModelAttemptEvidence *ModelAttemptEvidence

	// KnowledgeFlowEvidence is set if Kind is "knowledge_flow".
	KnowledgeFlowEvidence *KnowledgeFlowEvidence

	// Timestamp is when the evidence was recorded.
	Timestamp time.Time
}

// NewEvidenceRegistry creates a new evidence registry.
// The registry consumes events from the provided telemetry sink.
// If sink is nil, the registry operates in a no-op mode.
func NewEvidenceRegistry(sink telemetry.Telemetry) *EvidenceRegistry {
	return &EvidenceRegistry{
		prompts:            make(map[string]map[EvidenceKind]*PromptEvidence),
		calls:              make(map[string]*ModelAttemptEvidence),
		knowledge:          make(map[string]*KnowledgeFlowEvidence),
		callByPrompt:       make(map[string]string),
		promptByCall:       make(map[string]string),
		knowledgeByCall:    make(map[string][]string),
		telemetrySink:      sink,
		subscriptions:      make([]chan EvidenceEvent, 0),
		startTime:         time.Now().UTC(),
	}
}

// Start begins consuming telemetry events.
// This must be called to start receiving evidence.
// If the sink is a BroadcastSink, it subscribes to it and processes events.
func (r *EvidenceRegistry) Start(ctx context.Context) error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errors.New("evidence registry is closed")
	}
	r.mu.Unlock()

	// If the sink is a BroadcastSink, subscribe to it
	if bs, ok := r.telemetrySink.(*telemetry.BroadcastSink); ok {
		ch, cancel := bs.Subscribe(1024)
		go r.consumeEvents(ctx, ch, cancel)
	}
	return nil
}

// consumeEvents processes telemetry events and extracts evidence.
func (r *EvidenceRegistry) consumeEvents(ctx context.Context, ch <-chan telemetry.Event, cancel func()) {
	for {
		select {
		case <-ctx.Done():
			cancel()
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			r.processEvent(ev)
		}
	}
}

// processEvent extracts evidence from a telemetry event.
// This is the core of P06: it processes ALL existing OpenTelemetry events
// and extracts prompt, model attempt, and knowledge flow evidence.
func (r *EvidenceRegistry) processEvent(ev telemetry.Event) {
	if r == nil {
		return
	}

	switch ev.Type {
	// LLM/Model events
	case telemetry.EventLLMPrompt:
		r.processLLMPrompt(ev)
	case telemetry.EventLLMResponse:
		r.processLLMResponse(ev)

	// Node/Pipeline events
	case telemetry.EventNodeStart:
		r.processNodeStart(ev)
	case telemetry.EventNodeFinish:
		r.processNodeFinish(ev)
	case telemetry.EventNodeError:
		r.processNodeError(ev)

	// Compiler/Knowledge events
	case telemetry.EventCompilerStarted:
		r.processCompilerEvent(ev)
	case telemetry.EventCompilerCompleted:
		r.processCompilerEvent(ev)
	case telemetry.EventCompilerCacheHit:
		r.processCompilerCacheHit(ev)
	case telemetry.EventCompilerCacheMiss:
		r.processCompilerCacheMiss(ev)
	case telemetry.EventCompilerBudgetExceeded:
		r.processCompilerBudgetExceeded(ev)

	// Capture/Grounding events
	case telemetry.EventCaptureGrounded:
		r.processCaptureEvent(ev)
	case telemetry.EventCaptureGroundFailed:
		r.processCaptureEvent(ev)
	case telemetry.EventCaptureQuarantined:
		r.processCaptureQuarantined(ev)
	case telemetry.EventCaptureEpistemicsDowngraded:
		r.processCaptureDowngraded(ev)

	// Chunk events
	case telemetry.EventChunkCommitted:
		r.processChunkEvent(ev)
	case telemetry.EventChunkStaled:
		r.processChunkStaled(ev)
	case telemetry.EventChunkInvalidated:
		r.processChunkInvalidated(ev)
	case telemetry.EventSummaryCommitted:
		r.processSummaryEvent(ev)

	// Tombstone events
	case telemetry.EventTombstonePreserved:
		r.processTombstoneEvent(ev)

	// Delegation events
	case telemetry.EventDelegationStart:
		r.processDelegationStart(ev)
	case telemetry.EventDelegationFinish:
		r.processDelegationFinish(ev)

	// Capability/Tool events
	case telemetry.EventCapabilityCall:
		r.processCapabilityCall(ev)
	case telemetry.EventCapabilityResult:
		r.processCapabilityResult(ev)
	case telemetry.EventToolCall:
		r.processToolCall(ev)
	case telemetry.EventToolResult:
		r.processToolResult(ev)

	// Sandbox events
	case telemetry.EventSandboxCommandExecuted:
		r.processSandboxEvent(ev)
	case telemetry.EventSandboxFailure:
		r.processSandboxEvent(ev)

	// Scheduler events
	case telemetry.EventSchedulerJobStarted:
		r.processSchedulerEvent(ev)
	case telemetry.EventSchedulerJobCompleted:
		r.processSchedulerEvent(ev)
	}
}

// processLLMPrompt handles an LLM prompt event.
// This captures the prompt at the transport boundary.
func (r *EvidenceRegistry) processLLMPrompt(ev telemetry.Event) {
	if r == nil {
		return
	}

	callID := ev.NodeID
	if callID == "" {
		callID = extractString(ev.Metadata, "call_id")
	}
	if callID == "" {
		callID = ev.TraceID
	}
	if callID == "" {
		// Generate a synthetic call ID from the event sequence
		callID = fmt.Sprintf("call_%d", ev.Seq)
	}

	model := extractString(ev.Metadata, "model")
	provider := extractString(ev.Metadata, "provider")
	promptChars := int(extractInt64(ev.Metadata, "prompt_chars"))
	promptPreview := extractString(ev.Metadata, "prompt_preview")
	toolCallingMode := extractString(ev.Metadata, "tool_calling_mode")
	messageCount := int(extractInt64(ev.Metadata, "message_count"))

	// Extract roles if available
	var roles []string
	if raw, ok := ev.Metadata["roles"].([]any); ok {
		roles = make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				roles = append(roles, s)
			}
		}
	}

	// Extract prompt ID if available
	promptID := extractString(ev.Metadata, "prompt_id")

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create or update model attempt evidence
	attempt, ok := r.calls[callID]
	if !ok {
		attempt = &ModelAttemptEvidence{
			CallID:           callID,
			AttemptID:        generateEvidenceID("attempt"),
			Kind:             EvidenceKindActualCall,
			Model:            model,
			Provider:         provider,
			StartedAt:        ev.Timestamp,
			InputTokens:      promptChars / 4, // Rough estimate
			ToolCallingMode:  toolCallingMode,
			PromptPreview:    truncatePreview(promptPreview, 256),
			MessageCount:     messageCount,
			Roles:            roles,
			PromptID:         promptID,
		}
		r.calls[callID] = attempt
	} else {
		// Update existing attempt
		if attempt.Model == "" && model != "" {
			attempt.Model = model
		}
		if attempt.Provider == "" && provider != "" {
			attempt.Provider = provider
		}
		if attempt.StartedAt.IsZero() {
			attempt.StartedAt = ev.Timestamp
		}
		if attempt.PromptPreview == "" && promptPreview != "" {
			attempt.PromptPreview = truncatePreview(promptPreview, 256)
		}
		if attempt.ToolCallingMode == "" && toolCallingMode != "" {
			attempt.ToolCallingMode = toolCallingMode
		}
		if len(attempt.Roles) == 0 && len(roles) > 0 {
			attempt.Roles = roles
		}
		if attempt.MessageCount == 0 && messageCount > 0 {
			attempt.MessageCount = messageCount
		}
		// Update prompt ID if we have one
		if promptID != "" && attempt.PromptID == "" {
			attempt.PromptID = promptID
		}
	}

	// Link prompt to call if prompt_id is in metadata
	if promptID != "" {
		attempt.PromptID = promptID
		r.callByPrompt[promptID] = callID
		r.promptByCall[callID] = promptID
	}

	// Update input tokens estimate
	if promptChars > 0 {
		attempt.InputTokens = promptChars / 4
	}

	// Record the operation kind in attempt metadata if available
	if opKind := extractString(ev.Metadata, "kind"); opKind != "" {
		// This could be useful for distinguishing operation types
		attempt.AttemptID = fmt.Sprintf("%s_%s", opKind, attempt.AttemptID)
	}

	r.publishEvidence(EvidenceEvent{
		Kind:               "model_attempt",
		ModelAttemptEvidence: attempt,
		Timestamp:          ev.Timestamp,
	})
}

// processLLMResponse handles an LLM response event.
func (r *EvidenceRegistry) processLLMResponse(ev telemetry.Event) {
	if r == nil {
		return
	}

	callID := ev.NodeID
	if callID == "" {
		callID = extractString(ev.Metadata, "call_id")
	}
	if callID == "" {
		callID = ev.TraceID
	}

	model := extractString(ev.Metadata, "model")
	finishReason := extractString(ev.Metadata, "finish_reason")
	durationMS := extractInt64(ev.Metadata, "duration_ms")
	outputTokens := int(extractInt64(ev.Metadata, "output_tokens"))

	r.mu.Lock()
	defer r.mu.Unlock()

	attempt, ok := r.calls[callID]
	if !ok {
		// Create attempt if we missed the prompt
		attempt = &ModelAttemptEvidence{
			CallID:           callID,
			AttemptID:        generateEvidenceID("attempt"),
			Kind:             EvidenceKindActualCall,
			Model:            model,
			StartedAt:        time.Time{}, // Will be set when we see prompt
			CompletedAt:      ev.Timestamp,
			FinishReason:     finishReason,
			DurationMS:       durationMS,
			OutputTokens:     outputTokens,
			TransportReclaimed: true,
		}
		r.calls[callID] = attempt
	} else {
		attempt.CompletedAt = ev.Timestamp
		attempt.FinishReason = finishReason
		if durationMS > 0 {
			attempt.DurationMS = durationMS
		}
		if outputTokens > 0 {
			attempt.OutputTokens = outputTokens
		}
		attempt.TransportReclaimed = true
	}

	// Calculate duration if we have both start and end times
	if !attempt.StartedAt.IsZero() && !attempt.CompletedAt.IsZero() && attempt.DurationMS == 0 {
		attempt.DurationMS = int64(attempt.CompletedAt.Sub(attempt.StartedAt).Milliseconds())
	}

	r.publishEvidence(EvidenceEvent{
		Kind:               "model_attempt",
		ModelAttemptEvidence: attempt,
		Timestamp:          ev.Timestamp,
	})
}

// processNodeStart handles node start events.
func (r *EvidenceRegistry) processNodeStart(ev telemetry.Event) {
	if r == nil {
		return
	}

	nodeID := ev.NodeID
	if nodeID == "" {
		return
	}

	// Check if this node has model-related metadata
	model := extractString(ev.Metadata, "model")
	provider := extractString(ev.Metadata, "provider")
	
	// Check for prompt ID
	promptID := extractString(ev.Metadata, "prompt_id")

	if promptID != "" {
		r.mu.Lock()
		defer r.mu.Unlock()

		// Create a runtime prompt evidence entry
		if _, ok := r.prompts[promptID]; !ok {
			r.prompts[promptID] = make(map[EvidenceKind]*PromptEvidence)
		}

		// For now, just record that this prompt was referenced at runtime
		if _, ok := r.prompts[promptID][EvidenceKindRuntime]; !ok {
			r.prompts[promptID][EvidenceKindRuntime] = &PromptEvidence{
				ID:           promptID,
				Kind:         EvidenceKindRuntime,
				ResolvedAt:   ev.Timestamp,
				CallID:       nodeID,
			}
		}
	}

	// If this is a model node, also create a model attempt
	if model != "" || provider != "" {
		r.processLLMPrompt(ev)
	}
}

// processNodeFinish handles node finish events.
func (r *EvidenceRegistry) processNodeFinish(ev telemetry.Event) {
	if r == nil {
		return
	}

	nodeID := ev.NodeID
	if nodeID == "" {
		return
	}

	// Update any ongoing model attempt with this node ID
	r.mu.Lock()
	defer r.mu.Unlock()

	if attempt, ok := r.calls[nodeID]; ok {
		attempt.CompletedAt = ev.Timestamp
		attempt.FinishReason = extractString(ev.Metadata, "finish_reason")
		attempt.DurationMS = extractInt64(ev.Metadata, "duration_ms")
		if attempt.DurationMS == 0 && !attempt.StartedAt.IsZero() {
			attempt.DurationMS = int64(attempt.CompletedAt.Sub(attempt.StartedAt).Milliseconds())
		}
		attempt.TransportReclaimed = extractBool(ev.Metadata, "transport_reclaimed")
	}
}

// processNodeError handles node error events.
func (r *EvidenceRegistry) processNodeError(ev telemetry.Event) {
	if r == nil {
		return
	}

	nodeID := ev.NodeID
	if nodeID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if attempt, ok := r.calls[nodeID]; ok {
		attempt.Error = extractString(ev.Metadata, "error")
		if attempt.Error == "" {
			attempt.Error = ev.Message
		}
		attempt.CompletedAt = ev.Timestamp
	}
}

// processCompilerEvent handles compiler events for knowledge flow.
func (r *EvidenceRegistry) processCompilerEvent(ev telemetry.Event) {
	if r == nil {
		return
	}

	operationID := ev.NodeID
	if operationID == "" {
		operationID = ev.TraceID
	}
	if operationID == "" {
		operationID = generateEvidenceID("knowledge")
	}

	var direction, operation string
	var itemsSelected, budgetUsed int
	var lineage []LineageEntry
	var errorMsg string

	if ev.Type == telemetry.EventCompilerStarted {
		operation = "compile"
		direction = "input"
	} else if ev.Type == telemetry.EventCompilerCompleted {
		operation = "compile"
		direction = "output"
		budgetUsed = int(extractInt64(ev.Metadata, "budget_used"))
		itemsSelected = int(extractInt64(ev.Metadata, "items_selected"))
	}

	// Extract lineage from metadata if available
	if raw, ok := ev.Metadata["lineage"].([]any); ok {
		for _, entry := range raw {
			if m, ok := entry.(map[string]any); ok {
				lineage = append(lineage, LineageEntry{
					Step:      extractString(m, "step"),
					Operation: extractString(m, "operation"),
					Inputs:   extractStringSlice(m, "inputs"),
					Outputs:  extractStringSlice(m, "outputs"),
					Timestamp: ev.Timestamp,
				})
			}
		}
	}

	// Extract source identities
	sourceIDs := extractStringSlice(ev.Metadata, "source_identities")
	targetIDs := extractStringSlice(ev.Metadata, "target_identities")

	// Link to call if available
	callID := extractString(ev.Metadata, "call_id")

	flow := &KnowledgeFlowEvidence{
		ID:             operationID,
		Kind:           operation,
		Direction:      direction,
		Operation:      operation,
		ItemsSelected:  itemsSelected,
		BudgetUsed:     budgetUsed,
		Lineage:        lineage,
		DurationMS:     extractInt64(ev.Metadata, "duration_ms"),
		StartedAt:      ev.Timestamp,
		CompletedAt:    ev.Timestamp,
		Error:          errorMsg,
		SourceIdentities: sourceIDs,
		TargetIdentities: targetIDs,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.knowledge[operationID] = flow

	// Link knowledge flow to call
	if callID != "" {
		r.knowledgeByCall[callID] = append(r.knowledgeByCall[callID], operationID)
	}

	r.publishEvidence(EvidenceEvent{
		Kind:                  "knowledge_flow",
		KnowledgeFlowEvidence: flow,
		Timestamp:             ev.Timestamp,
	})
}

// processCompilerCacheHit handles compiler cache hit events.
func (r *EvidenceRegistry) processCompilerCacheHit(ev telemetry.Event) {
	if r == nil {
		return
	}

	promptID := extractString(ev.Metadata, "prompt_id")
	if promptID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.prompts[promptID]; !ok {
		r.prompts[promptID] = make(map[EvidenceKind]*PromptEvidence)
	}

	r.prompts[promptID][EvidenceKindPreview] = &PromptEvidence{
		ID:          promptID,
		Kind:        EvidenceKindPreview,
		ResolvedAt:  ev.Timestamp,
		CacheHit:    true,
		DurationMS:  extractInt64(ev.Metadata, "duration_ms"),
	}
}

// processCompilerCacheMiss handles compiler cache miss events.
func (r *EvidenceRegistry) processCompilerCacheMiss(ev telemetry.Event) {
	// Similar to cache hit but marks as cache miss
	promptID := extractString(ev.Metadata, "prompt_id")
	if promptID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.prompts[promptID]; !ok {
		r.prompts[promptID] = make(map[EvidenceKind]*PromptEvidence)
	}

	if _, ok := r.prompts[promptID][EvidenceKindPreview]; !ok {
		r.prompts[promptID][EvidenceKindPreview] = &PromptEvidence{
			ID:          promptID,
			Kind:        EvidenceKindPreview,
			ResolvedAt:  ev.Timestamp,
			CacheHit:    false,
			DurationMS:  extractInt64(ev.Metadata, "duration_ms"),
		}
	}
}

// processCompilerBudgetExceeded handles compiler budget exceeded events.
func (r *EvidenceRegistry) processCompilerBudgetExceeded(ev telemetry.Event) {
	operationID := ev.NodeID
	if operationID == "" {
		operationID = generateEvidenceID("knowledge")
	}

	budgetUsed := int(extractInt64(ev.Metadata, "budget_used"))

	r.mu.Lock()
	defer r.mu.Unlock()

	flow := &KnowledgeFlowEvidence{
		ID:          operationID,
		Kind:        "compile",
		Operation:   "compile",
		Direction:   "output",
		BudgetUsed:  budgetUsed,
		Error:       "budget exceeded",
		StartedAt:   ev.Timestamp,
		CompletedAt: ev.Timestamp,
	}

	r.knowledge[operationID] = flow
}

// processCaptureEvent handles capture and grounding events.
func (r *EvidenceRegistry) processCaptureEvent(ev telemetry.Event) {
	if r == nil {
		return
	}

	operationID := ev.NodeID
	if operationID == "" {
		operationID = ev.TraceID
	}
	if operationID == "" {
		operationID = generateEvidenceID("knowledge")
	}

	var operation, direction string
	var errorMsg string

	if ev.Type == telemetry.EventCaptureGrounded {
		operation = "ground"
		direction = "output"
	} else if ev.Type == telemetry.EventCaptureGroundFailed {
		operation = "ground"
		direction = "output"
		errorMsg = extractString(ev.Metadata, "error")
	}

	// Extract additional metadata
	chunkID := extractString(ev.Metadata, "chunk_id")
	contentHash := extractString(ev.Metadata, "content_hash")
	tokenEstimate := int(extractInt64(ev.Metadata, "token_estimate"))
	sessionID := extractString(ev.Metadata, "session_id")
	workflowID := extractString(ev.Metadata, "workflow_id")
	nodeID := extractString(ev.Metadata, "node_id")

	// Link to call if available
	callID := extractString(ev.Metadata, "call_id")

	flow := &KnowledgeFlowEvidence{
		ID:            operationID,
		Kind:          operation,
		Direction:     direction,
		Operation:     operation,
		Error:         errorMsg,
		DurationMS:    extractInt64(ev.Metadata, "duration_ms"),
		StartedAt:     ev.Timestamp,
		CompletedAt:   ev.Timestamp,
		ChunkID:       chunkID,
		ContentHash:   contentHash,
		TokenEstimate: tokenEstimate,
		SessionID:     sessionID,
		WorkflowID:    workflowID,
		NodeID:        nodeID,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.knowledge[operationID] = flow

	// Link knowledge flow to call
	if callID != "" {
		r.knowledgeByCall[callID] = append(r.knowledgeByCall[callID], operationID)
	}

	r.publishEvidence(EvidenceEvent{
		Kind:                  "knowledge_flow",
		KnowledgeFlowEvidence: flow,
		Timestamp:             ev.Timestamp,
	})
}

// processCaptureQuarantined handles capture quarantined events.
func (r *EvidenceRegistry) processCaptureQuarantined(ev telemetry.Event) {
	operationID := ev.NodeID
	if operationID == "" {
		operationID = generateEvidenceID("knowledge")
	}

	chunkID := extractString(ev.Metadata, "chunk_id")
	contentHash := extractString(ev.Metadata, "content_hash")
	kind := extractString(ev.Metadata, "kind")

	r.mu.Lock()
	defer r.mu.Unlock()

	flow := &KnowledgeFlowEvidence{
		ID:          operationID,
		Kind:        "quarantine",
		Operation:   "capture",
		Direction:   "output",
		ChunkID:     chunkID,
		ContentHash: contentHash,
		Error:       fmt.Sprintf("quarantined: %s", kind),
		StartedAt:   ev.Timestamp,
		CompletedAt: ev.Timestamp,
	}

	r.knowledge[operationID] = flow
}

// processCaptureDowngraded handles capture epistemics downgraded events.
func (r *EvidenceRegistry) processCaptureDowngraded(ev telemetry.Event) {
	operationID := ev.NodeID
	if operationID == "" {
		operationID = generateEvidenceID("knowledge")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	flow := &KnowledgeFlowEvidence{
		ID:          operationID,
		Kind:        "downgraded",
		Operation:   "capture",
		Direction:   "output",
		Error:       "epistemics downgraded",
		StartedAt:   ev.Timestamp,
		CompletedAt: ev.Timestamp,
	}

	r.knowledge[operationID] = flow
}

// processChunkEvent handles chunk commit events.
func (r *EvidenceRegistry) processChunkEvent(ev telemetry.Event) {
	if r == nil {
		return
	}

	operationID := ev.NodeID
	if operationID == "" {
		operationID = ev.TraceID
	}
	if operationID == "" {
		operationID = generateEvidenceID("knowledge")
	}

	var operation, direction string

	if ev.Type == telemetry.EventChunkCommitted {
		operation = "chunk_commit"
		direction = "output"
	} else if ev.Type == telemetry.EventSummaryCommitted {
		operation = "summary_commit"
		direction = "output"
	}

	// Extract metadata
	chunkID := extractString(ev.Metadata, "chunk_id")
	contentHash := extractString(ev.Metadata, "content_hash")
	tokenEstimate := int(extractInt64(ev.Metadata, "token_estimate"))
	sessionID := extractString(ev.Metadata, "session_id")
	workflowID := extractString(ev.Metadata, "workflow_id")
	nodeID := extractString(ev.Metadata, "node_id")

	// Link to call if available
	callID := extractString(ev.Metadata, "call_id")

	flow := &KnowledgeFlowEvidence{
		ID:            operationID,
		Kind:          operation,
		Direction:     direction,
		Operation:     operation,
		DurationMS:    extractInt64(ev.Metadata, "duration_ms"),
		StartedAt:     ev.Timestamp,
		CompletedAt:   ev.Timestamp,
		ChunkID:       chunkID,
		ContentHash:   contentHash,
		TokenEstimate: tokenEstimate,
		SessionID:     sessionID,
		WorkflowID:    workflowID,
		NodeID:        nodeID,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.knowledge[operationID] = flow

	// Link knowledge flow to call
	if callID != "" {
		r.knowledgeByCall[callID] = append(r.knowledgeByCall[callID], operationID)
	}

	r.publishEvidence(EvidenceEvent{
		Kind:                  "knowledge_flow",
		KnowledgeFlowEvidence: flow,
		Timestamp:             ev.Timestamp,
	})
}

// processChunkStaled handles chunk staled events.
func (r *EvidenceRegistry) processChunkStaled(ev telemetry.Event) {
	if r == nil {
		return
	}

	operationID := generateEvidenceID("knowledge")

	chunkIDs := extractStringSlice(ev.Metadata, "chunk_ids")
	affectedPaths := extractStringSlice(ev.Metadata, "affected_paths")
	reason := extractString(ev.Metadata, "reason")
	workflowID := extractString(ev.Metadata, "workflow_id")

	r.mu.Lock()
	defer r.mu.Unlock()

	flow := &KnowledgeFlowEvidence{
		ID:             operationID,
		Kind:           "staled",
		Operation:      "chunk",
		Direction:      "output",
		Error:          reason,
		SourceIdentities: chunkIDs,
		TargetIdentities: affectedPaths,
		StartedAt:      ev.Timestamp,
		CompletedAt:    ev.Timestamp,
		WorkflowID:     workflowID,
	}

	r.knowledge[operationID] = flow
}

// processChunkInvalidated handles chunk invalidated events.
func (r *EvidenceRegistry) processChunkInvalidated(ev telemetry.Event) {
	operationID := generateEvidenceID("knowledge")

	affectedPaths := extractStringSlice(ev.Metadata, "affected_paths")

	r.mu.Lock()
	defer r.mu.Unlock()

	flow := &KnowledgeFlowEvidence{
		ID:            operationID,
		Kind:          "invalidated",
		Operation:     "chunk",
		Direction:     "output",
		TargetIdentities: affectedPaths,
		StartedAt:     ev.Timestamp,
		CompletedAt:   ev.Timestamp,
	}

	r.knowledge[operationID] = flow
}

// processSummaryEvent handles summary commit events.
func (r *EvidenceRegistry) processSummaryEvent(ev telemetry.Event) {
	// Similar to chunk event but for summaries
	r.processChunkEvent(ev)
}

// processTombstoneEvent handles tombstone preserved events.
func (r *EvidenceRegistry) processTombstoneEvent(ev telemetry.Event) {
	operationID := generateEvidenceID("knowledge")

	chunkID := extractString(ev.Metadata, "chunk_id")
	contentHash := extractString(ev.Metadata, "content_hash")

	r.mu.Lock()
	defer r.mu.Unlock()

	flow := &KnowledgeFlowEvidence{
		ID:          operationID,
		Kind:        "tombstone",
		Operation:   "preserve",
		Direction:   "output",
		ChunkID:     chunkID,
		ContentHash: contentHash,
		StartedAt:   ev.Timestamp,
		CompletedAt: ev.Timestamp,
	}

	r.knowledge[operationID] = flow
}

// processDelegationStart handles delegation start events.
func (r *EvidenceRegistry) processDelegationStart(ev telemetry.Event) {
	// Delegation can involve model calls, track as potential call
	callID := extractString(ev.Metadata, "call_id")
	if callID == "" {
		callID = ev.NodeID
	}
	if callID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.calls[callID]; !ok {
		r.calls[callID] = &ModelAttemptEvidence{
			CallID:    callID,
			AttemptID: generateEvidenceID("attempt"),
			Kind:      EvidenceKindActualCall,
			StartedAt: ev.Timestamp,
		}
	}
}

// processDelegationFinish handles delegation finish events.
func (r *EvidenceRegistry) processDelegationFinish(ev telemetry.Event) {
	callID := extractString(ev.Metadata, "call_id")
	if callID == "" {
		callID = ev.NodeID
	}
	if callID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if attempt, ok := r.calls[callID]; ok {
		attempt.CompletedAt = ev.Timestamp
		attempt.FinishReason = extractString(ev.Metadata, "finish_reason")
	}
}

// processCapabilityCall handles capability call events.
func (r *EvidenceRegistry) processCapabilityCall(ev telemetry.Event) {
	// Capability calls can trigger model calls, track them
	callID := extractString(ev.Metadata, "call_id")
	if callID == "" {
		callID = ev.NodeID
	}
	if callID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.calls[callID]; !ok {
		r.calls[callID] = &ModelAttemptEvidence{
			CallID:    callID,
			AttemptID: generateEvidenceID("attempt"),
			Kind:      EvidenceKindActualCall,
			StartedAt: ev.Timestamp,
		}
	}
}

// processCapabilityResult handles capability result events.
func (r *EvidenceRegistry) processCapabilityResult(ev telemetry.Event) {
	callID := extractString(ev.Metadata, "call_id")
	if callID == "" {
		callID = ev.NodeID
	}
	if callID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if attempt, ok := r.calls[callID]; ok {
		attempt.CompletedAt = ev.Timestamp
		attempt.FinishReason = "capability_result"
	}
}

// processToolCall handles tool call events.
func (r *EvidenceRegistry) processToolCall(ev telemetry.Event) {
	// Tool calls are part of model interactions
	callID := extractString(ev.Metadata, "call_id")
	if callID == "" {
		callID = ev.NodeID
	}
	if callID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Link tool call to model attempt
	if attempt, ok := r.calls[callID]; ok {
		// Update tool calling mode if we have more info
		if toolName := extractString(ev.Metadata, "tool_name"); toolName != "" {
			if attempt.ToolCallingMode == "" {
				attempt.ToolCallingMode = toolName
			}
		}
	}
}

// processToolResult handles tool result events.
func (r *EvidenceRegistry) processToolResult(ev telemetry.Event) {
	callID := extractString(ev.Metadata, "call_id")
	if callID == "" {
		callID = ev.NodeID
	}
	if callID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if attempt, ok := r.calls[callID]; ok {
		attempt.CompletedAt = ev.Timestamp
	}
}

// processSandboxEvent handles sandbox events.
func (r *EvidenceRegistry) processSandboxEvent(ev telemetry.Event) {
	// Sandbox events don't directly relate to evidence, but we track them
	// for completeness
}

// processSchedulerEvent handles scheduler events.
func (r *EvidenceRegistry) processSchedulerEvent(ev telemetry.Event) {
	// Scheduler events don't directly relate to evidence
}

// GetPromptEvidence retrieves prompt evidence for a given ID and kind.
// It returns the best available evidence based on the requested kind:
// - For EvidenceKindActualCall: tries to find actual call evidence linked to the prompt
// - For EvidenceKindRuntime: returns runtime resolution evidence
// - For EvidenceKindPreview: returns preview evidence
// If the specific kind is not available, it returns the best available evidence.
func (r *EvidenceRegistry) GetPromptEvidence(ctx context.Context, promptID string, kind EvidenceKind) (*PromptEvidence, error) {
	if r == nil {
		return nil, ErrEvidenceNotFound
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if prompts, ok := r.prompts[promptID]; ok {
		// Try to get the specific kind first
		if evidence, ok := prompts[kind]; ok {
			return evidence, nil
		}

		// If specific kind not found, try fallbacks based on kind
		if kind == EvidenceKindActualCall {
			// Actual call evidence is linked via callByPrompt
			if callID, ok := r.callByPrompt[promptID]; ok {
				if attempt, ok := r.calls[callID]; ok {
					// Return a synthetic prompt evidence linked to the actual call
					return &PromptEvidence{
						ID:           promptID,
						Kind:         EvidenceKindActualCall,
						CallID:       callID,
						AttemptID:    attempt.AttemptID,
						ResolvedAt:   attempt.StartedAt,
					}, nil
				}
			}
			// If we have runtime evidence, return it with a note
			if evidence, ok := prompts[EvidenceKindRuntime]; ok {
				return evidence, ErrNoActualCall
			}
			// If we have preview evidence, return it with a note
			if evidence, ok := prompts[EvidenceKindPreview]; ok {
				return evidence, ErrPreviewOnly
			}
		}

		// For other kinds, try to return the best available
		if evidence, ok := prompts[EvidenceKindRuntime]; ok {
			return evidence, nil
		}
		if evidence, ok := prompts[EvidenceKindPreview]; ok {
			if kind == EvidenceKindActualCall {
				return evidence, ErrNoActualCall
			}
			return evidence, nil
		}
	}

	return nil, ErrEvidenceNotFound
}

// GetModelAttemptEvidence retrieves model attempt evidence for a given call ID.
func (r *EvidenceRegistry) GetModelAttemptEvidence(ctx context.Context, callID string) (*ModelAttemptEvidence, error) {
	if r == nil {
		return nil, ErrEvidenceNotFound
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if attempt, ok := r.calls[callID]; ok {
		return attempt, nil
	}

	return nil, ErrEvidenceNotFound
}

// GetKnowledgeFlowEvidence retrieves knowledge flow evidence for a given operation ID.
func (r *EvidenceRegistry) GetKnowledgeFlowEvidence(ctx context.Context, operationID string) (*KnowledgeFlowEvidence, error) {
	if r == nil {
		return nil, ErrEvidenceNotFound
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if flow, ok := r.knowledge[operationID]; ok {
		return flow, nil
	}

	return nil, ErrEvidenceNotFound
}

// GetEvidenceByCallID retrieves all evidence associated with a specific call ID.
// This includes the model attempt, linked prompt evidence, and linked knowledge flows.
func (r *EvidenceRegistry) GetEvidenceByCallID(ctx context.Context, callID string) (*CallEvidenceBundle, error) {
	if r == nil {
		return nil, ErrEvidenceNotFound
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	bundle := &CallEvidenceBundle{
		CallID: callID,
	}

	// Get model attempt
	if attempt, ok := r.calls[callID]; ok {
		bundle.ModelAttempt = attempt
	}

	// Get linked prompt
	if promptID, ok := r.promptByCall[callID]; ok {
		if prompts, ok := r.prompts[promptID]; ok {
			// Get all kinds of prompt evidence
			if ev, ok := prompts[EvidenceKindPreview]; ok {
				bundle.PromptPreview = ev
			}
			if ev, ok := prompts[EvidenceKindRuntime]; ok {
				bundle.PromptRuntime = ev
			}
		}
	}

	// Get linked knowledge flows
	if flowIDs, ok := r.knowledgeByCall[callID]; ok {
		bundle.KnowledgeFlows = make([]*KnowledgeFlowEvidence, 0, len(flowIDs))
		for _, flowID := range flowIDs {
			if flow, ok := r.knowledge[flowID]; ok {
				bundle.KnowledgeFlows = append(bundle.KnowledgeFlows, flow)
			}
		}
	}

	if bundle.ModelAttempt == nil && bundle.PromptPreview == nil && bundle.PromptRuntime == nil && len(bundle.KnowledgeFlows) == 0 {
		return nil, ErrEvidenceNotFound
	}

	return bundle, nil
}

// CallEvidenceBundle contains all evidence associated with a single call.
type CallEvidenceBundle struct {
	CallID string

	// ModelAttempt is the model attempt evidence.
	ModelAttempt *ModelAttemptEvidence

	// PromptPreview is the preview prompt evidence (if available).
	PromptPreview *PromptEvidence

	// PromptRuntime is the runtime prompt evidence (if available).
	PromptRuntime *PromptEvidence

	// KnowledgeFlows are the knowledge flow evidence associated with this call.
	KnowledgeFlows []*KnowledgeFlowEvidence
}

// ListPromptEvidence lists all prompt evidence IDs.
func (r *EvidenceRegistry) ListPromptEvidence(ctx context.Context) ([]string, error) {
	if r == nil {
		return nil, errors.New("registry is nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.prompts))
	for id := range r.prompts {
		ids = append(ids, id)
	}
	return ids, nil
}

// ListModelAttempts lists all model attempt IDs.
func (r *EvidenceRegistry) ListModelAttempts(ctx context.Context) ([]string, error) {
	if r == nil {
		return nil, errors.New("registry is nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.calls))
	for id := range r.calls {
		ids = append(ids, id)
	}
	return ids, nil
}

// ListKnowledgeFlows lists all knowledge flow IDs.
func (r *EvidenceRegistry) ListKnowledgeFlows(ctx context.Context) ([]string, error) {
	if r == nil {
		return nil, errors.New("registry is nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.knowledge))
	for id := range r.knowledge {
		ids = append(ids, id)
	}
	return ids, nil
}

// GetAllEvidence returns all evidence collected by the registry.
// This is useful for export and debugging.
func (r *EvidenceRegistry) GetAllEvidence(ctx context.Context) (*CompleteEvidence, error) {
	if r == nil {
		return nil, errors.New("registry is nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := &CompleteEvidence{
		Prompts:     make([]*PromptEvidence, 0),
		Calls:       make([]*ModelAttemptEvidence, 0),
		Knowledge:   make([]*KnowledgeFlowEvidence, 0),
		CollectedAt: time.Now().UTC(),
		StartedAt:   r.startTime,
	}

	// Collect all prompt evidence
	for _, kinds := range r.prompts {
		for _, ev := range kinds {
			result.Prompts = append(result.Prompts, ev)
		}
	}

	// Collect all model attempt evidence
	for _, ev := range r.calls {
		result.Calls = append(result.Calls, ev)
	}

	// Collect all knowledge flow evidence
	for _, ev := range r.knowledge {
		result.Knowledge = append(result.Knowledge, ev)
	}

	return result, nil
}

// CompleteEvidence contains all evidence collected by the registry.
type CompleteEvidence struct {
	Prompts   []*PromptEvidence
	Calls     []*ModelAttemptEvidence
	Knowledge []*KnowledgeFlowEvidence

	CollectedAt time.Time
	StartedAt   time.Time
}

// Subscribe registers a channel to receive new evidence events.
// The buffer size determines how many events can be buffered before drops occur.
func (r *EvidenceRegistry) Subscribe(bufferSize int) <-chan EvidenceEvent {
	if r == nil {
		return nil
	}

	if bufferSize <= 0 {
		bufferSize = 256
	}

	ch := make(chan EvidenceEvent, bufferSize)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		close(ch)
		return ch
	}

	r.subscriptions = append(r.subscriptions, ch)

	return ch
}

// publishEvidence sends evidence to all subscribers.
func (r *EvidenceRegistry) publishEvidence(ev EvidenceEvent) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, ch := range r.subscriptions {
		select {
		case ch <- ev:
		default:
			// Drop if channel is full
		}
	}
}

// Close closes the registry and releases resources.
func (r *EvidenceRegistry) Close() error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true

	for _, ch := range r.subscriptions {
		close(ch)
	}
	r.subscriptions = nil

	return nil
}

// truncatePreview truncates a preview string to maxLen characters.
func truncatePreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}

// Helper functions for extracting values from metadata.

func extractString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func extractStringSlice(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		if slice, ok := v.([]any); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
		if slice, ok := v.([]string); ok {
			return slice
		}
	}
	return nil
}

func extractInt64(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int64:
			return val
		case int:
			return int64(val)
		case int32:
			return int64(val)
		case int16:
			return int64(val)
		case int8:
			return int64(val)
		case uint:
			return int64(val)
		case uint8:
			return int64(val)
		case uint16:
			return int64(val)
		case uint32:
			return int64(val)
		case uint64:
			return int64(val)
		case float64:
			return int64(val)
		case float32:
			return int64(val)
		case json.Number:
			if i, err := val.Int64(); err == nil {
				return i
			}
		}
	}
	return 0
}

func extractBool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// generateEvidenceID generates a unique evidence identifier.
func generateEvidenceID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, telemetry.NewSpanID())
}
