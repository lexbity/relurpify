package reporting

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// TelemetryNode reports execution metrics and outcomes.
type TelemetryNode struct {
	id string
}

// NewTelemetryNode creates a new telemetry node.
func NewTelemetryNode(id string) *TelemetryNode {
	return &TelemetryNode{
		id: id,
	}
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
// Phase 13: Stub implementation - will integrate with framework telemetry.
func (n *TelemetryNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	// Get execution metrics from envelope
	completedVal, _ := env.GetWorkingValue("euclo.execution.completed")
	completed, _ := completedVal.(bool)

	// Classify outcome
	outcome := ClassifyOutcome(completed, 0, false)

	// Write outcome to envelope
	env.SetWorkingValue("euclo.outcome.category", string(outcome.Category), contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.outcome.reason", outcome.Reason, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.outcome.completed", outcome.Completed, contextdata.MemoryClassTask)

	// Phase 13: Stub telemetry emission - in production, this would emit to framework telemetry
	return map[string]any{
		"outcome_category": string(outcome.Category),
		"outcome_reason":   outcome.Reason,
		"completed":        outcome.Completed,
	}, nil
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
