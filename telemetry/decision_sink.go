// decision_sink.go defines the governance → telemetry port for decision
// forensics (Decision 6). Governance is a leaf in the domain DAG: it may
// depend on telemetry, never the reverse. The DecisionSink interface keeps
// that dependency abstract so governance never depends on concrete sink
// implementations.
package telemetry

import (
	"context"
	"sync"
	"time"
)

// Decision telemetry event types. These are the JSONL-specific spellings of
// the causal record; the framework event log keeps its own typed .v1 events
// with the same semantics.
const (
	EventPolicyEvaluated        EventType = "policy.evaluated"
	EventPolicyConflictShadowed EventType = "policy.conflict_shadowed"
	EventHITLRequested          EventType = "hitl.requested"
	EventHITLResolved           EventType = "hitl.resolved"
	EventHITLExpired            EventType = "hitl.expired"
	EventDoomLoopDetected       EventType = "doom_loop.detected"

	EventCommandWrapperUnwrapped  EventType = "command.wrapper_unwrapped"
	EventCommandParseFailed       EventType = "command.parse_failed"
	EventCommandOpaqueConstructor EventType = "command.opaque_constructor"
)

// Command-event kinds, matching the EventCommand* event-type suffixes.
const (
	CommandEventWrapperUnwrapped  = "wrapper_unwrapped"
	CommandEventParseFailed       = "parse_failed"
	CommandEventOpaqueConstructor = "opaque_constructor"
)

// Supported values for PolicyDecision.Effect.
const (
	PolicyEffectAllow           = "allow"
	PolicyEffectDeny            = "deny"
	PolicyEffectRequireApproval = "require_approval"
)

// PolicyDecision carries the forensic payload of one policy evaluation.
// Domain-specific fragments travel in Fields and land in Metadata;
// correlation (session/run/trace) is stamped from ctx by the adapter.
type PolicyDecision struct {
	Rule   string // matched rule identifier ("" when no manifest rule matched — fallback policy)
	Effect string // PolicyEffectAllow | PolicyEffectDeny | PolicyEffectRequireApproval
	Reason string
	Actor  string
	Target string
	Fields map[string]any
}

// HITLRequest identifies a human-approval request.
type HITLRequest struct {
	RequestID     string
	Action        string
	Actor         string
	Justification string
	RequiresHITL  bool
}

// HITLResolution identifies how a request was resolved.
type HITLResolution struct {
	RequestID  string
	Outcome    string // "approved" | "denied" | "expired"
	ApprovedBy string
	Reason     string
}

// DoomLoopSignal describes a detected doom-loop pattern.
type DoomLoopSignal struct {
	Kind         string
	CapabilityID string
	CallCount    int
	Evidence     []string
	Guidance     string
}

// PolicyConflict describes a matched allow rule that was shadowed by a stronger
// effect in the deny-wins policy lattice. It is emitted so operators can find
// policies whose effective meaning changed when deny-wins landed.
type PolicyConflict struct {
	Winner   string // winning (strongest) rule ID
	Shadowed string // shadowed allow rule ID
	Effect   string // the shadowed rule's effect (always "allow")
	Actor    string
}

// CommandEvent carries a command-authorization forensic signal (wrapper
// unwrapping, parse failure, opaque constructor).
type CommandEvent struct {
	Kind    string // CommandEvent* constant
	Command string
	Wrapper string
	Depth   int
	Reason  string
	Actor   string
}

// DecisionSink is the port governance components receive at construction to
// emit decision forensics. Implementations must not panic and must not block
// the caller beyond NFR-5 (< 1ms per event).
type DecisionSink interface {
	PolicyEvaluated(ctx context.Context, decision PolicyDecision)
	PolicyConflictShadowed(ctx context.Context, conflict PolicyConflict)
	CommandEvent(ctx context.Context, event CommandEvent)
	HITLRequested(ctx context.Context, request HITLRequest)
	HITLResolved(ctx context.Context, resolution HITLResolution)
	DoomLoopDetected(ctx context.Context, signal DoomLoopSignal)
}

// TelemetryDecisionSink emits decision events as structured telemetry events,
// carrying the caller's correlation context into every event (FR-1/FR-2).
type TelemetryDecisionSink struct {
	Telemetry Telemetry
}

// emit builds and dispatches one decision event.
func (s TelemetryDecisionSink) emit(ctx context.Context, et EventType, message string, metadata map[string]any, actor string) {
	if s.Telemetry == nil {
		return
	}
	ev := Event{
		Type:      et,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	if actor != "" {
		ev.Actor = actor
	}
	StampCorrelation(ctx, &ev)
	s.Telemetry.Emit(ev)
}

// PolicyEvaluated implements DecisionSink.
func (s TelemetryDecisionSink) PolicyEvaluated(ctx context.Context, decision PolicyDecision) {
	metadata := policyDecisionMetadata(decision)
	s.emit(ctx, EventPolicyEvaluated, "policy evaluated: "+decision.Effect, metadata, decision.Actor)
}

// PolicyConflictShadowed implements DecisionSink.
func (s TelemetryDecisionSink) PolicyConflictShadowed(ctx context.Context, conflict PolicyConflict) {
	metadata := map[string]any{
		"winner_rule":     conflict.Winner,
		"shadowed_rule":   conflict.Shadowed,
		"shadowed_effect": conflict.Effect,
	}
	s.emit(ctx, EventPolicyConflictShadowed, "policy conflict: allow shadowed by stronger effect", metadata, conflict.Actor)
}

// commandEventType maps a CommandEvent kind to its telemetry event type.
func commandEventType(kind string) EventType {
	switch kind {
	case CommandEventWrapperUnwrapped:
		return EventCommandWrapperUnwrapped
	case CommandEventParseFailed:
		return EventCommandParseFailed
	case CommandEventOpaqueConstructor:
		return EventCommandOpaqueConstructor
	default:
		return EventType("command." + kind)
	}
}

// CommandEvent implements DecisionSink.
func (s TelemetryDecisionSink) CommandEvent(ctx context.Context, event CommandEvent) {
	metadata := map[string]any{"kind": event.Kind}
	if event.Command != "" {
		metadata["command"] = event.Command
	}
	if event.Wrapper != "" {
		metadata["wrapper"] = event.Wrapper
	}
	if event.Depth > 0 {
		metadata["depth"] = event.Depth
	}
	if event.Reason != "" {
		metadata["reason"] = event.Reason
	}
	s.emit(ctx, commandEventType(event.Kind), "command event: "+event.Kind, metadata, event.Actor)
}

// HITLRequested implements DecisionSink.
func (s TelemetryDecisionSink) HITLRequested(ctx context.Context, request HITLRequest) {
	metadata := map[string]any{
		"request_id": request.RequestID,
		"action":     request.Action,
	}
	if request.Actor != "" {
		metadata["actor"] = request.Actor
	}
	if request.Justification != "" {
		metadata["justification"] = request.Justification
	}
	if request.RequiresHITL {
		metadata["requires_hitl"] = true
	}
	s.emit(ctx, EventHITLRequested, "hitl permission requested", metadata, request.Actor)
}

// HITLResolved implements DecisionSink.
func (s TelemetryDecisionSink) HITLResolved(ctx context.Context, resolution HITLResolution) {
	metadata := map[string]any{
		"request_id": resolution.RequestID,
		"outcome":    resolution.Outcome,
	}
	if resolution.ApprovedBy != "" {
		metadata["approved_by"] = resolution.ApprovedBy
	}
	if resolution.Reason != "" {
		metadata["reason"] = resolution.Reason
	}
	s.emit(ctx, EventHITLResolved, "hitl permission resolved: "+resolution.Outcome, metadata, resolution.ApprovedBy)
}

// DoomLoopDetected implements DecisionSink.
func (s TelemetryDecisionSink) DoomLoopDetected(ctx context.Context, signal DoomLoopSignal) {
	metadata := map[string]any{
		"kind":   signal.Kind,
		"calls":  signal.CallCount,
		"target": signal.CapabilityID,
	}
	if len(signal.Evidence) > 0 {
		metadata["evidence"] = signal.Evidence
	}
	if signal.Guidance != "" {
		metadata["guidance"] = signal.Guidance
	}
	s.emit(ctx, EventDoomLoopDetected, "doom loop detected: "+signal.Kind, metadata, "")
}

func policyDecisionMetadata(decision PolicyDecision) map[string]any {
	metadata := map[string]any{
		"rule":   decision.Rule,
		"effect": decision.Effect,
		"reason": decision.Reason,
		"actor":  decision.Actor,
		"target": decision.Target,
	}
	for k, v := range decision.Fields {
		if _, exists := metadata[k]; exists {
			continue
		}
		metadata[k] = v
	}
	return metadata
}

// MultiplexDecisionSink fans decision events out to every registered sink,
// mirroring MultiplexTelemetry for the decision port.
type MultiplexDecisionSink struct {
	Sinks []DecisionSink
}

// PolicyEvaluated implements DecisionSink.
func (m MultiplexDecisionSink) PolicyEvaluated(ctx context.Context, decision PolicyDecision) {
	for _, s := range m.Sinks {
		s.PolicyEvaluated(ctx, decision)
	}
}

// PolicyConflictShadowed implements DecisionSink.
func (m MultiplexDecisionSink) PolicyConflictShadowed(ctx context.Context, conflict PolicyConflict) {
	for _, s := range m.Sinks {
		s.PolicyConflictShadowed(ctx, conflict)
	}
}

// CommandEvent implements DecisionSink.
func (m MultiplexDecisionSink) CommandEvent(ctx context.Context, event CommandEvent) {
	for _, s := range m.Sinks {
		s.CommandEvent(ctx, event)
	}
}

// HITLRequested implements DecisionSink.
func (m MultiplexDecisionSink) HITLRequested(ctx context.Context, request HITLRequest) {
	for _, s := range m.Sinks {
		s.HITLRequested(ctx, request)
	}
}

// HITLResolved implements DecisionSink.
func (m MultiplexDecisionSink) HITLResolved(ctx context.Context, resolution HITLResolution) {
	for _, s := range m.Sinks {
		s.HITLResolved(ctx, resolution)
	}
}

// DoomLoopDetected implements DecisionSink.
func (m MultiplexDecisionSink) DoomLoopDetected(ctx context.Context, signal DoomLoopSignal) {
	for _, s := range m.Sinks {
		s.DoomLoopDetected(ctx, signal)
	}
}

// SnapshotDecisionSink is the in-memory recording decision sink used by the
// e2e harness and tests to assert on emitted decisions (FR-8).
type SnapshotDecisionSink struct {
	mu            sync.Mutex
	policies      []PolicyDecision
	requests      []HITLRequest
	resolves      []HITLResolution
	dooms         []DoomLoopSignal
	conflicts     []PolicyConflict
	commandEvents []CommandEvent
}

// PolicyEvaluated implements DecisionSink.
func (s *SnapshotDecisionSink) PolicyEvaluated(_ context.Context, decision PolicyDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies = append(s.policies, decision)
}

// HITLRequested implements DecisionSink.
func (s *SnapshotDecisionSink) HITLRequested(_ context.Context, request HITLRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
}

// HITLResolved implements DecisionSink.
func (s *SnapshotDecisionSink) HITLResolved(_ context.Context, resolution HITLResolution) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolves = append(s.resolves, resolution)
}

// DoomLoopDetected implements DecisionSink.
func (s *SnapshotDecisionSink) DoomLoopDetected(_ context.Context, signal DoomLoopSignal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dooms = append(s.dooms, signal)
}

// PolicyConflictShadowed implements DecisionSink.
func (s *SnapshotDecisionSink) PolicyConflictShadowed(_ context.Context, conflict PolicyConflict) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conflicts = append(s.conflicts, conflict)
}

// CommandEvent implements DecisionSink.
func (s *SnapshotDecisionSink) CommandEvent(_ context.Context, event CommandEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commandEvents = append(s.commandEvents, event)
}

// Conflicts returns copies of every recorded shadowed-allow conflict.
func (s *SnapshotDecisionSink) Conflicts() []PolicyConflict {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]PolicyConflict(nil), s.conflicts...)
}

// CommandEvents returns copies of every recorded command event.
func (s *SnapshotDecisionSink) CommandEvents() []CommandEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]CommandEvent(nil), s.commandEvents...)
}

// Snapshots returns copies of everything recorded so far.
func (s *SnapshotDecisionSink) Snapshots() (policies []PolicyDecision, requests []HITLRequest, resolves []HITLResolution, dooms []DoomLoopSignal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	policies = append([]PolicyDecision(nil), s.policies...)
	requests = append([]HITLRequest(nil), s.requests...)
	resolves = append([]HITLResolution(nil), s.resolves...)
	dooms = append([]DoomLoopSignal(nil), s.dooms...)
	return policies, requests, resolves, dooms
}
