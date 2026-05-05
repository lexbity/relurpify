package reporting

import (
	"context"
	"errors"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// EucloTelemetry wraps core.Telemetry with typed Emit helpers.
type EucloTelemetry struct {
	sink core.Telemetry
}

// NewEucloTelemetry creates a new telemetry wrapper.
func NewEucloTelemetry(sink core.Telemetry) *EucloTelemetry {
	if sink == nil {
		sink = noopTelemetry{}
	}
	return &EucloTelemetry{sink: sink}
}

func (t *EucloTelemetry) emit(ctx context.Context, eventType EventType, payload any) {
	if t == nil || t.sink == nil {
		return
	}
	meta := eventPayloadMap(payload)
	taskID, _ := meta["task_id"].(string)
	t.sink.Emit(core.Event{
		Type:      core.EventType(eventType),
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Metadata:  meta,
	})
}

func (t *EucloTelemetry) EmitIntakeComplete(ctx context.Context, ev EventIntakeComplete) {
	t.emit(ctx, EventTypeIntakeComplete, ev)
}

func (t *EucloTelemetry) EmitFamilySelected(ctx context.Context, ev EventFamilySelected) {
	t.emit(ctx, EventTypeFamilySelected, ev)
}

func (t *EucloTelemetry) EmitIngestionComplete(ctx context.Context, ev EventIngestionComplete) {
	t.emit(ctx, EventTypeIngestionComplete, ev)
}

func (t *EucloTelemetry) EmitStreamRequested(ctx context.Context, ev EventStreamRequested) {
	t.emit(ctx, EventTypeStreamRequested, ev)
}

func (t *EucloTelemetry) EmitCapabilityClassified(ctx context.Context, ev EventCapabilityClassified) {
	t.emit(ctx, EventTypeCapabilityClassified, ev)
}

func (t *EucloTelemetry) EmitRouteSelected(ctx context.Context, ev EventRouteSelected) {
	t.emit(ctx, EventTypeRouteSelected, ev)
}

func (t *EucloTelemetry) EmitGateResult(ctx context.Context, ev EventGateResult) {
	t.emit(ctx, EventTypeGateResult, ev)
}

func (t *EucloTelemetry) EmitFrameEmitted(ctx context.Context, ev EventFrameEmitted) {
	t.emit(ctx, EventTypeFrameEmittedEuclo, ev)
}

func (t *EucloTelemetry) EmitFrameResolved(ctx context.Context, ev EventFrameResolved) {
	t.emit(ctx, EventTypeFrameResolvedEuclo, ev)
}

func (t *EucloTelemetry) EmitJobSubmitted(ctx context.Context, ev EventJobSubmitted) {
	t.emit(ctx, EventTypeJobSubmitted, ev)
}

func (t *EucloTelemetry) EmitJobCompleted(ctx context.Context, ev EventJobCompleted) {
	t.emit(ctx, EventTypeJobCompleted, ev)
}

func (t *EucloTelemetry) EmitStepCompleted(ctx context.Context, ev EventStepCompleted) {
	t.emit(ctx, EventTypeStepCompletedEuclo, ev)
}

func (t *EucloTelemetry) EmitExecutionComplete(ctx context.Context, ev EventExecutionComplete) {
	t.emit(ctx, EventTypeExecutionComplete, ev)
}

type noopTelemetry struct{}

func (noopTelemetry) Emit(core.Event) {}

// TelemetryNode reports execution metrics and outcomes.
type TelemetryNode struct {
	id        string
	telemetry *EucloTelemetry
}

// NewTelemetryNode creates a new telemetry node.
func NewTelemetryNode(id string) *TelemetryNode {
	return &TelemetryNode{id: id}
}

// ID returns the node ID.
func (n *TelemetryNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *TelemetryNode) Type() string {
	return "telemetry"
}

// Execute collects and reports telemetry data.
func (n *TelemetryNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	if env == nil {
		return nil, errors.New("envelope is nil")
	}

	completedVal, _ := env.GetWorkingValue("euclo.execution.completed")
	completed, _ := completedVal.(bool)
	blockedVal, _ := env.GetWorkingValue("euclo.policy.blocked")
	blocked, _ := blockedVal.(bool)
	errorCount := 0
	if v, ok := env.GetWorkingValue("euclo.execution.error_count"); ok {
		switch value := v.(type) {
		case int:
			errorCount = value
		case int64:
			errorCount = int(value)
		case float64:
			errorCount = int(value)
		}
	}

	outcome := ClassifyOutcome(completed, errorCount, blocked)
	env.SetWorkingValue("euclo.outcome.category", string(outcome.Category), contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.outcome.reason", outcome.Reason, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.outcome.completed", outcome.Completed, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.outcome_telemetry", map[string]any{
		"category":    string(outcome.Category),
		"reason":      outcome.Reason,
		"completed":   outcome.Completed,
		"error_count": outcome.ErrorCount,
		"blocked":     blocked,
	}, contextdata.MemoryClassTask)

	tel := n.telemetry
	if tel == nil {
		tel = NewEucloTelemetry(core.TelemetryFromContext(ctx))
	}
	if tel != nil {
		tel.EmitExecutionComplete(ctx, EventExecutionComplete{
			EventHeader: EventHeader{
				TaskID:     env.TaskID,
				SessionID:  env.SessionID,
				Seq:        0,
				OccurredAt: time.Now().UTC(),
			},
			Outcome:     string(outcome.Category),
			OutcomeKind: outcome.Reason,
			StepCount:   0,
			LLMCalls:    0,
			TokenUsage:  0,
		})
	}

	return map[string]any{
		"outcome_category": string(outcome.Category),
		"outcome_reason":   outcome.Reason,
		"completed":        outcome.Completed,
	}, nil
}

// EmitRouteSelected reports the selected route and candidate metadata.
func EmitRouteSelected(ctx context.Context, taskID, sessionID, family, routeKind, routeID string, candidateCount int, fallbackTaken bool) {
	emitRouteEvent(ctx, EventTypeRouteSelected, taskID, sessionID, map[string]any{
		"session_id":      sessionID,
		"family":          family,
		"route_kind":      routeKind,
		"route_id":        routeID,
		"candidate_count": candidateCount,
		"fallback_taken":  fallbackTaken,
	})
}

// EmitRouteCompleted reports route completion metadata.
func EmitRouteCompleted(ctx context.Context, taskID, sessionID, routeKind, routeID string, outcome RouteOutcome, artifactKinds []string, elapsed time.Duration) {
	emitRouteEvent(ctx, EventTypeRouteCompleted, taskID, sessionID, map[string]any{
		"session_id":     sessionID,
		"route_kind":     routeKind,
		"route_id":       routeID,
		"outcome":        string(outcome),
		"artifact_kinds": append([]string(nil), artifactKinds...),
		"elapsed_ms":     elapsed.Milliseconds(),
	})
}

// EmitRouteUnavailable reports an unavailable route and the reason.
func EmitRouteUnavailable(ctx context.Context, taskID, sessionID, routeID, availability, reason string) {
	emitRouteEvent(ctx, EventTypeRouteUnavailable, taskID, sessionID, map[string]any{
		"session_id":   sessionID,
		"route_id":     routeID,
		"availability": availability,
		"reason":       reason,
	})
}

// EmitRouteDryRun reports a dry-run payload.
func EmitRouteDryRun(ctx context.Context, taskID, sessionID string, report any) {
	emitRouteEvent(ctx, EventTypeRouteDryRun, taskID, sessionID, map[string]any{
		"session_id": sessionID,
		"report":     report,
	})
}

// EmitRouteFallback reports primary and fallback route IDs.
func EmitRouteFallback(ctx context.Context, taskID, sessionID, primaryID, fallbackID, reason string) {
	emitRouteEvent(ctx, EventTypeRouteFallback, taskID, sessionID, map[string]any{
		"session_id":  sessionID,
		"primary_id":  primaryID,
		"fallback_id": fallbackID,
		"reason":      reason,
	})
}

func emitRouteEvent(ctx context.Context, eventType EventType, taskID, sessionID string, data map[string]any) {
	telemetry := core.TelemetryFromContext(ctx)
	if telemetry == nil {
		return
	}
	telemetry.Emit(core.Event{
		Type:      core.EventType(string(eventType)),
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Metadata:  data,
	})
}
