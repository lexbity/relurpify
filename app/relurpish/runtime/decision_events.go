package runtime

import (
	"context"
	"encoding/json"
	"time"

	"codeburg.org/lexbit/relurpify/platform/observability"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/event"
)

// eventLogDecisionSink mirrors decision forensics into the framework event
// log as typed .v1 events with full actor identity. It is the causal-record
// half of the runtime's decision wiring; the operational half is
// telemetry.TelemetryDecisionSink. Both are composed via
// telemetry.MultiplexDecisionSink in buildRuntime.
type eventLogDecisionSink struct {
	log       event.Log
	partition string
	actor     observability.Actor
}

func (s eventLogDecisionSink) append(ctx context.Context, et, payload string) {
	_, _ = s.log.Append(ctx, s.partition, []event.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      et,
		Payload:   json.RawMessage(payload),
		Actor:     s.actor,
		Partition: s.partition,
	}})
}

// PolicyEvaluated implements telemetry.DecisionSink.
func (s eventLogDecisionSink) PolicyEvaluated(ctx context.Context, decision telemetry.PolicyDecision) {
	data, err := json.Marshal(map[string]any{
		"rule":   decision.Rule,
		"effect": decision.Effect,
		"reason": decision.Reason,
		"actor":  decision.Actor,
		"target": decision.Target,
		"fields": decision.Fields,
	})
	if err != nil {
		return
	}
	s.append(ctx, event.EventPolicyEvaluated, string(data))
}

// HITLRequested implements telemetry.DecisionSink.
func (s eventLogDecisionSink) HITLRequested(ctx context.Context, request telemetry.HITLRequest) {
	data, err := json.Marshal(map[string]any{
		"request_id":    request.RequestID,
		"action":        request.Action,
		"actor":         request.Actor,
		"justification": request.Justification,
		"requires_hitl": request.RequiresHITL,
	})
	if err != nil {
		return
	}
	s.append(ctx, event.EventHITLRequested, string(data))
}

// HITLResolved implements telemetry.DecisionSink.
func (s eventLogDecisionSink) HITLResolved(ctx context.Context, resolution telemetry.HITLResolution) {
	data, err := json.Marshal(map[string]any{
		"request_id":  resolution.RequestID,
		"outcome":     resolution.Outcome,
		"approved_by": resolution.ApprovedBy,
		"reason":      resolution.Reason,
	})
	if err != nil {
		return
	}
	et := event.EventHITLResolved
	if resolution.Outcome == "expired" {
		et = event.EventHITLExpired
	}
	s.append(ctx, et, string(data))
}

// DoomLoopDetected implements telemetry.DecisionSink.
func (s eventLogDecisionSink) DoomLoopDetected(ctx context.Context, signal telemetry.DoomLoopSignal) {
	data, err := json.Marshal(map[string]any{
		"kind":     signal.Kind,
		"target":   signal.CapabilityID,
		"calls":    signal.CallCount,
		"evidence": signal.Evidence,
		"guidance": signal.Guidance,
	})
	if err != nil {
		return
	}
	s.append(ctx, event.EventDoomLoopDetected, string(data))
}
