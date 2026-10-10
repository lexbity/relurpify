package authorization

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/governance/policy"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/telemetry"
)

func newDecisionSinkPermissionManager(t *testing.T) (*PermissionManager, *fwtelemetry.SnapshotDecisionSink) {
	t.Helper()
	declared := &ucperms.PermissionSet{
		Capabilities: []ucperms.CapabilityPermission{
			{Capability: "test-cap"},
		},
	}
	pm, err := NewPermissionManager("/tmp", declared, newTestAuditLogger(t), nil)
	require.NoError(t, err)
	sink := &fwtelemetry.SnapshotDecisionSink{}
	pm.SetDecisionSink(sink)
	return pm, sink
}

// TestPermissionManager_EmitsPolicyDenial verifies a capability denial emits
// a structured decision event carrying the reason and actor (FR-5, AC-2).
func TestPermissionManager_EmitsPolicyDenial(t *testing.T) {
	pm, sink := newDecisionSinkPermissionManager(t)

	err := pm.CheckCapability(context.Background(), "agent:euclo", "undeclared-cap")
	require.Error(t, err)

	policies, _, _, _ := sink.Snapshots()
	require.Len(t, policies, 1)
	decision := policies[0]
	require.Equal(t, fwtelemetry.PolicyEffectDeny, decision.Effect)
	require.Equal(t, "capability not declared", decision.Reason)
	require.Equal(t, "agent:euclo", decision.Actor)
	require.Equal(t, "cap:undeclared-cap", decision.Target)
}

// TestPermissionManager_EmitsPolicyAllow verifies every evaluation — not just
// denials — is emitted (FR-5).
func TestPermissionManager_EmitsPolicyAllow(t *testing.T) {
	pm, sink := newDecisionSinkPermissionManager(t)

	require.NoError(t, pm.CheckCapability(context.Background(), "agent:euclo", "test-cap"))

	policies, _, _, _ := sink.Snapshots()
	require.Len(t, policies, 1)
	decision := policies[0]
	require.Equal(t, fwtelemetry.PolicyEffectAllow, decision.Effect)
	require.Equal(t, "cap:test-cap", decision.Target)
}

// TestPermissionManager_TelemetryDecisionSinkCarriesCorrelation verifies the
// full chain: manager → TelemetryDecisionSink → stamped JSONL event (FR-2).
func TestPermissionManager_TelemetryDecisionSinkCarriesCorrelation(t *testing.T) {
	declared := &ucperms.PermissionSet{
		Capabilities: []ucperms.CapabilityPermission{
			{Capability: "test-cap"},
		},
	}
	pm, err := NewPermissionManager("/tmp", declared, nil, nil)
	require.NoError(t, err)

	events := &correlatingEventSink{}
	pm.SetDecisionSink(fwtelemetry.TelemetryDecisionSink{Telemetry: events})

	rc := telemetry.RunContext{SessionID: "sess-9", RunID: "run-9", TraceID: "trace-9", AgentID: "agent-9"}
	ctx := telemetry.WithRunContext(context.Background(), rc)

	require.NoError(t, pm.CheckCapability(ctx, "agent:euclo", "test-cap"))

	require.Len(t, events.events, 1)
	ev := events.events[0]
	require.Equal(t, fwtelemetry.EventPolicyEvaluated, ev.Type)
	require.Equal(t, "sess-9", ev.SessionID)
	require.Equal(t, "run-9", ev.RunID)
	require.Equal(t, "trace-9", ev.TraceID)
	require.Equal(t, fwtelemetry.PolicyEffectAllow, ev.Metadata["effect"])
}

type correlatingEventSink struct {
	events []fwtelemetry.Event
}

func (s *correlatingEventSink) Emit(ev fwtelemetry.Event) {
	s.events = append(s.events, ev)
}

// TestManifestPolicyEngine_EmitsShadowedConflict verifies the deny-wins lattice
// reports a shadowed allow through the decision sink.
func TestManifestPolicyEngine_EmitsShadowedConflict(t *testing.T) {
	pm, sink := newDecisionSinkPermissionManager(t)
	engine := &ManifestPolicyEngine{
		agentID: "agent:euclo",
		manager: pm,
		rules: []policy.PolicyRule{
			testRule("tool:allow", 300, "allow"),
			testRule("global:deny", 100, "deny"),
		},
	}
	decision, err := engine.Evaluate(context.Background(), policy.PolicyRequest{})
	require.NoError(t, err)
	require.Equal(t, "deny", decision.Effect)

	conflicts := sink.Conflicts()
	require.Len(t, conflicts, 1)
	require.Equal(t, "global:deny", conflicts[0].Winner)
	require.Equal(t, "tool:allow", conflicts[0].Shadowed)
	require.Equal(t, "allow", conflicts[0].Effect)
	require.Equal(t, "agent:euclo", conflicts[0].Actor)
}

// TestHITLBroker_EmitsLifecycleEvents verifies the full HITL lifecycle is
// emitted with matching request IDs (FR-6, AC-3).
func TestHITLBroker_EmitsLifecycleEvents(t *testing.T) {
	sink := &fwtelemetry.SnapshotDecisionSink{}
	broker := NewHITLBroker(50*time.Millisecond, sink)
	defer broker.Stop()

	go func() {
		// Wait for the pending request to be recorded, then approve it.
		require.Eventually(t, func() bool {
			return len(broker.PendingRequests()) == 1
		}, time.Second, 5*time.Millisecond)
		pending := broker.PendingRequests()[0]
		require.NoError(t, broker.Approve(PermissionDecision{
			RequestID:  pending.ID,
			Approved:   true,
			ApprovedBy: "approval-actor",
		}))
	}()

	_, err := broker.RequestPermission(context.Background(), PermissionRequest{
		Permission:    ucperms.PermissionDescriptor{Action: "fs:write:/tmp/x"},
		Justification: "test approval",
		Scope:         policy.GrantScopeOneTime,
	})
	require.NoError(t, err)

	_, requests, resolves, _ := sink.Snapshots()
	require.Len(t, requests, 1)
	require.Len(t, resolves, 1)
	require.Equal(t, requests[0].RequestID, resolves[0].RequestID, "matching request IDs")
	require.Equal(t, "approved", resolves[0].Outcome)
	require.Equal(t, "approval-actor", resolves[0].ApprovedBy)
}

// TestHITLBroker_EmitsExpiredEvent verifies the timeout path emits an
// expired resolution (FR-6).
func TestHITLBroker_EmitsExpiredEvent(t *testing.T) {
	sink := &fwtelemetry.SnapshotDecisionSink{}
	broker := NewHITLBroker(30*time.Millisecond, sink)
	defer broker.Stop()

	_, err := broker.RequestPermission(context.Background(), PermissionRequest{
		Permission: ucperms.PermissionDescriptor{Action: "fs:write:/tmp/x"},
	})
	require.Error(t, err)

	_, requests, resolves, _ := sink.Snapshots()
	require.Len(t, requests, 1)
	require.Len(t, resolves, 1)
	require.Equal(t, requests[0].RequestID, resolves[0].RequestID)
	require.Equal(t, "expired", resolves[0].Outcome)
}

// TestHITLBroker_EmitsAsyncResolution verifies an async request resolved via
// Approve emits exactly one resolution (FR-6).
func TestHITLBroker_EmitsAsyncResolution(t *testing.T) {
	sink := &fwtelemetry.SnapshotDecisionSink{}
	broker := NewHITLBroker(5*time.Minute, sink)
	defer broker.Stop()

	ctx := context.Background()
	requestID, err := broker.SubmitAsync(ctx, PermissionRequest{
		Permission:    ucperms.PermissionDescriptor{Action: "fs:write:/tmp/x"},
		Justification: "async ask",
	})
	require.NoError(t, err)

	require.NoError(t, broker.Approve(PermissionDecision{
		RequestID:  requestID,
		Approved:   true,
		ApprovedBy: "human:reviewer",
	}))

	_, requests, resolves, _ := sink.Snapshots()
	require.Len(t, requests, 1)
	require.Len(t, resolves, 1)
	require.Equal(t, requestID, resolves[0].RequestID)
	require.Equal(t, "approved", resolves[0].Outcome)
	require.Empty(t, broker.PendingRequests(), "resolved async request must be consumed")
}
