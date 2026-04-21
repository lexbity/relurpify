package server

import (
	"context"
	"encoding/json"
	"testing"

	nexusgateway "codeburg.org/lexbit/relurpify/app/nexus/gateway"
	"codeburg.org/lexbit/relurpify/framework/core"
	rexnexus "codeburg.org/lexbit/relurpify/named/rex/nexus"
	rexproof "codeburg.org/lexbit/relurpify/named/rex/proof"
	rexruntime "codeburg.org/lexbit/relurpify/named/rex/runtime"
	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
	"github.com/stretchr/testify/require"
)

func TestSnapshotForPrincipalFiltersTenantAdminView(t *testing.T) {
	state := nexusgateway.StateSnapshot{
		LastSeq: 42,
		ActiveSessions: map[string]nexusgateway.SessionState{
			"sess-a": {Role: "agent"},
			"sess-b": {Role: "agent"},
		},
		ChannelActivity: map[string]nexusgateway.ChannelState{
			"webchat": {Inbound: 3, Outbound: 2},
		},
		EventTypeCounts: map[string]uint64{
			core.FrameworkEventMessageInbound: 5,
		},
		Tenants: map[string]nexusgateway.TenantState{
			"tenant-a": {
				LastSeq: 21,
				ActiveSessions: map[string]nexusgateway.SessionState{
					"sess-a": {Role: "agent"},
				},
				ChannelActivity: map[string]nexusgateway.ChannelState{
					"webchat": {Inbound: 2, Outbound: 1},
				},
				EventTypeCounts: map[string]uint64{
					core.FrameworkEventMessageInbound: 2,
				},
			},
		},
	}
	materializer := nexusgateway.NewStateMaterializer()
	require.NoError(t, materializer.Restore(context.Background(), mustSnapshotJSON(t, state)))

	snapshot, err := snapshotForPrincipal(context.Background(), materializer, nil, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "admin", ID: "admin-a", TenantID: "tenant-a"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			TenantID:      "tenant-a",
			Scopes:        []string{"gateway:admin"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "tenant-a", snapshot["tenant_id"])
	activeSessions := snapshot["active_sessions"].(map[string]nexusgateway.SessionState)
	require.Len(t, activeSessions, 1)
	require.Contains(t, activeSessions, "sess-a")
	require.Equal(t, uint64(2), snapshot["channel_activity"].(map[string]nexusgateway.ChannelState)["webchat"].Inbound)
	require.Equal(t, uint64(2), snapshot["event_counts"].(map[string]uint64)[core.FrameworkEventMessageInbound])
}

func TestSnapshotForPrincipalAllowsGlobalAdminView(t *testing.T) {
	state := nexusgateway.StateSnapshot{
		LastSeq: 42,
		ActiveSessions: map[string]nexusgateway.SessionState{
			"sess-a": {Role: "agent"},
			"sess-b": {Role: "agent"},
		},
		ChannelActivity: map[string]nexusgateway.ChannelState{
			"webchat": {Inbound: 3, Outbound: 2},
		},
		EventTypeCounts: map[string]uint64{
			core.FrameworkEventMessageInbound: 5,
		},
	}
	materializer := nexusgateway.NewStateMaterializer()
	require.NoError(t, materializer.Restore(context.Background(), mustSnapshotJSON(t, state)))

	snapshot, err := snapshotForPrincipal(context.Background(), materializer, nil, fwgateway.ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "admin", ID: "admin-global", TenantID: "tenant-a"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			TenantID:      "tenant-a",
			Scopes:        []string{"gateway:admin", "gateway:admin:global"},
		},
	})
	require.NoError(t, err)
	activeSessions := snapshot["active_sessions"].(map[string]nexusgateway.SessionState)
	require.Len(t, activeSessions, 2)
	require.NotContains(t, snapshot, "tenant_id")
	require.Equal(t, uint64(5), snapshot["event_counts"].(map[string]uint64)[core.FrameworkEventMessageInbound])
}

func TestSnapshotForPrincipalIncludesRexSnapshot(t *testing.T) {
	materializer := nexusgateway.NewStateMaterializer()
	provider := &RexRuntimeProvider{
		Adapter: rexnexus.NewAdapter("rex", fakeRexRuntime{}, nil),
	}

	snapshot, err := snapshotForPrincipal(context.Background(), materializer, provider, fwgateway.ConnectionPrincipal{})
	require.NoError(t, err)
	rexSnapshot, ok := snapshot["rex"].(rexnexus.AdminSnapshot)
	require.True(t, ok)
	require.Equal(t, rexruntime.HealthHealthy, rexSnapshot.Runtime.Health)
}

type fakeRexRuntime struct{}

func (fakeRexRuntime) Execute(context.Context, *core.Task, *core.Context) (*core.Result, error) {
	return &core.Result{Success: true, Data: map[string]any{"ok": true}}, nil
}

func (fakeRexRuntime) RuntimeProjection() rexnexus.Projection {
	return rexnexus.Projection{Health: rexruntime.HealthHealthy, LastProof: rexproof.ProofSurface{VerificationStatus: "pass"}}
}

func (fakeRexRuntime) Capabilities() []core.Capability {
	return []core.Capability{core.CapabilityExecute}
}

func mustSnapshotJSON(t *testing.T, snapshot nexusgateway.StateSnapshot) []byte {
	t.Helper()
	payload, err := json.Marshal(snapshot)
	require.NoError(t, err)
	return payload
}
