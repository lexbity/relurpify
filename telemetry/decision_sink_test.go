package telemetry

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type decisionEventSink struct {
	events []Event
}

func (s *decisionEventSink) Emit(ev Event) {
	s.events = append(s.events, ev)
}

// TestTelemetryDecisionSink_PolicyEvaluated verifies the policy.evaluated
// event shape (AC-2): type, metadata.rule, metadata.effect, metadata.reason,
// and correlation stamped from context onto the first-class fields.
func TestTelemetryDecisionSink_PolicyEvaluated(t *testing.T) {
	sink := &decisionEventSink{}
	ds := TelemetryDecisionSink{Telemetry: sink}

	rc := RunContext{SessionID: "sess-1", RunID: "run-1", TraceID: "trace-1", AgentID: "agent-1"}
	ctx := WithRunContext(context.Background(), rc)

	ds.PolicyEvaluated(ctx, PolicyDecision{
		Rule:   "file_write:deny",
		Effect: PolicyEffectDeny,
		Reason: "path outside workspace scope",
		Actor:  "agent:euclo",
		Target: "file_write",
	})

	require.Len(t, sink.events, 1)
	ev := sink.events[0]
	require.Equal(t, EventPolicyEvaluated, ev.Type)
	require.Equal(t, "deny", ev.Metadata["effect"])
	require.Equal(t, "file_write:deny", ev.Metadata["rule"])
	require.Equal(t, "path outside workspace scope", ev.Metadata["reason"])
	require.Equal(t, "file_write", ev.Metadata["target"])
	// FR-2: correlation comes from context onto first-class fields, not Metadata.
	require.Equal(t, "sess-1", ev.SessionID)
	require.Equal(t, "run-1", ev.RunID)
	require.Equal(t, "trace-1", ev.TraceID)
	require.GreaterOrEqual(t, ev.Timestamp.Unix(), int64(0))
}

// TestTelemetryDecisionSink_PolicyConflictShadowed verifies the
// policy.conflict_shadowed event shape.
func TestTelemetryDecisionSink_PolicyConflictShadowed(t *testing.T) {
	sink := &decisionEventSink{}
	ds := TelemetryDecisionSink{Telemetry: sink}

	ds.PolicyConflictShadowed(context.Background(), PolicyConflict{
		Winner:   "global:deny",
		Shadowed: "tool:allow",
		Effect:   "allow",
		Actor:    "agent:euclo",
	})

	require.Len(t, sink.events, 1)
	ev := sink.events[0]
	require.Equal(t, EventPolicyConflictShadowed, ev.Type)
	require.Equal(t, "global:deny", ev.Metadata["winner_rule"])
	require.Equal(t, "tool:allow", ev.Metadata["shadowed_rule"])
	require.Equal(t, "allow", ev.Metadata["shadowed_effect"])
	require.Equal(t, "agent:euclo", ev.Actor)
}

// TestTelemetryDecisionSink_HITLLifecycle verifies hitl.requested and
// hitl.resolved events with matching request IDs (AC-3).
func TestTelemetryDecisionSink_HITLLifecycle(t *testing.T) {
	sink := &decisionEventSink{}
	ds := TelemetryDecisionSink{Telemetry: sink}
	ctx := context.Background()

	ds.HITLRequested(ctx, HITLRequest{
		RequestID:     "hitl-1",
		Action:        "fs:write:/etc/hosts",
		Actor:         "agent:euclo",
		Justification: "needs write outside workspace",
		RequiresHITL:  true,
	})
	ds.HITLResolved(ctx, HITLResolution{
		RequestID:  "hitl-1",
		Outcome:    "approved",
		ApprovedBy: "human:reviewer",
	})

	require.Len(t, sink.events, 2)
	require.Equal(t, EventHITLRequested, sink.events[0].Type)
	require.Equal(t, "hitl-1", sink.events[0].Metadata["request_id"])
	require.Equal(t, "fs:write:/etc/hosts", sink.events[0].Metadata["action"])
	require.Equal(t, EventHITLResolved, sink.events[1].Type)
	require.Equal(t, "hitl-1", sink.events[1].Metadata["request_id"], "matching request IDs (AC-3)")
	require.Equal(t, "approved", sink.events[1].Metadata["outcome"])
}

// TestTelemetryDecisionSink_DoomLoopDetected verifies the doom_loop.detected
// event carries kind, target, and call count (FR-7).
func TestTelemetryDecisionSink_DoomLoopDetected(t *testing.T) {
	sink := &decisionEventSink{}
	ds := TelemetryDecisionSink{Telemetry: sink}

	ds.DoomLoopDetected(context.Background(), DoomLoopSignal{
		Kind:         "identical_call",
		CapabilityID: "test.cap",
		CallCount:    3,
	})

	require.Len(t, sink.events, 1)
	ev := sink.events[0]
	require.Equal(t, EventDoomLoopDetected, ev.Type)
	require.Equal(t, "identical_call", ev.Metadata["kind"])
	require.Equal(t, "test.cap", ev.Metadata["target"])
	require.Equal(t, 3, ev.Metadata["calls"])
}

// TestTelemetryDecisionSink_NilTelemetryNeverPanics verifies the NFR-3
// contract: emission into a nil sink is a silent drop.
func TestTelemetryDecisionSink_NilTelemetryNeverPanics(t *testing.T) {
	ds := TelemetryDecisionSink{Telemetry: nil}
	ds.PolicyEvaluated(context.Background(), PolicyDecision{Effect: PolicyEffectAllow})
	ds.HITLRequested(context.Background(), HITLRequest{})
	ds.HITLResolved(context.Background(), HITLResolution{})
	ds.DoomLoopDetected(context.Background(), DoomLoopSignal{})
}

// TestMultiplexDecisionSink_FansOut verifies decision events reach all sinks.
func TestMultiplexDecisionSink_FansOut(t *testing.T) {
	sink1 := &SnapshotDecisionSink{}
	sink2 := &SnapshotDecisionSink{}
	mux := MultiplexDecisionSink{Sinks: []DecisionSink{sink1, sink2}}

	mux.PolicyEvaluated(context.Background(), PolicyDecision{Effect: PolicyEffectDeny, Reason: "r"})
	mux.HITLRequested(context.Background(), HITLRequest{RequestID: "req-1"})
	mux.HITLResolved(context.Background(), HITLResolution{RequestID: "req-1", Outcome: "approved"})
	mux.DoomLoopDetected(context.Background(), DoomLoopSignal{Kind: "oscillating"})

	for _, sink := range []*SnapshotDecisionSink{sink1, sink2} {
		policies, requests, resolves, dooms := sink.Snapshots()
		require.Len(t, policies, 1)
		require.Equal(t, PolicyEffectDeny, policies[0].Effect)
		require.Len(t, requests, 1)
		require.Equal(t, "req-1", requests[0].RequestID)
		require.Len(t, resolves, 1)
		require.Equal(t, "approved", resolves[0].Outcome)
		require.Len(t, dooms, 1)
		require.Equal(t, "oscillating", dooms[0].Kind)
	}
}

// TestDecisionSinkEmissionUnderNFR5 verifies a decision emission adds well
// under NFR-5's budget (< 1ms per event into a in-memory sink).
func TestDecisionSinkEmissionUnderNFR5(t *testing.T) {
	sink := &decisionEventSink{}
	ds := TelemetryDecisionSink{Telemetry: sink}
	start := time.Now()
	for i := 0; i < 1000; i++ {
		ds.PolicyEvaluated(context.Background(), PolicyDecision{Effect: PolicyEffectAllow, Target: "t"})
	}
	require.Less(t, time.Since(start).Milliseconds()/1000, int64(1))
}
