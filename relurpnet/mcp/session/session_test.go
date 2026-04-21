package session

import (
	"testing"

	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
	"github.com/stretchr/testify/require"
)

func TestSessionLifecycleTransitions(t *testing.T) {
	s, err := NewClientSession(Config{
		ProviderID:       "remote-mcp",
		SessionID:        "remote-mcp:primary",
		TransportKind:    "stdio",
		RequestedVersion: protocol.Revision20250618,
		LocalPeer:        protocol.PeerInfo{Name: "relurpify", Version: "dev"},
		LocalCapabilities: map[string]any{
			"roots": map[string]any{"listChanged": true},
		},
		Recoverable: true,
	})
	require.NoError(t, err)
	require.Equal(t, StateConnecting, s.Snapshot().State)

	require.NoError(t, s.MarkTransportEstablished())
	require.Equal(t, StateInitializing, s.Snapshot().State)

	require.NoError(t, s.ApplyInitializeResult(protocol.InitializeResult{
		ProtocolVersion: protocol.Revision20250618,
		ServerInfo:      protocol.PeerInfo{Name: "fixture-server", Version: "1.0.0"},
		Capabilities: map[string]any{
			"tools": map[string]any{"listChanged": true},
		},
	}))
	snap := s.Snapshot()
	require.Equal(t, StateInitialized, snap.State)
	require.Equal(t, protocol.Revision20250618, snap.NegotiatedVersion)
	require.Equal(t, "fixture-server", snap.RemotePeer.Name)
	require.NotNil(t, snap.RemoteCapabilities)

	require.NoError(t, s.UpdateRequestCount(2))
	require.Equal(t, 2, s.Snapshot().ActiveRequests)
	require.NoError(t, s.UpdateRequestCount(-1))
	require.Equal(t, 1, s.Snapshot().ActiveRequests)

	require.NoError(t, s.BeginClose())
	require.Equal(t, StateClosing, s.Snapshot().State)
	s.MarkClosed()
	require.Equal(t, StateClosed, s.Snapshot().State)
	require.Zero(t, s.Snapshot().ActiveRequests)
}

func TestSessionRejectsInvalidTransitionOrder(t *testing.T) {
	s, err := NewClientSession(Config{
		ProviderID:       "remote-mcp",
		SessionID:        "remote-mcp:primary",
		TransportKind:    "stdio",
		RequestedVersion: protocol.Revision20250618,
	})
	require.NoError(t, err)

	err = s.ApplyInitializeResult(protocol.InitializeResult{ProtocolVersion: protocol.Revision20250618})
	require.ErrorContains(t, err, "invalid from state connecting")
}

func TestSessionDegradeAndFailCaptureReason(t *testing.T) {
	s, err := NewClientSession(Config{
		ProviderID:       "remote-mcp",
		SessionID:        "remote-mcp:primary",
		TransportKind:    "stdio",
		RequestedVersion: protocol.Revision20250618,
	})
	require.NoError(t, err)
	require.NoError(t, s.MarkTransportEstablished())
	require.NoError(t, s.Degrade("transport stalled"))
	require.Equal(t, StateDegraded, s.Snapshot().State)
	require.Equal(t, "transport stalled", s.Snapshot().FailureReason)

	require.NoError(t, s.Fail("peer disconnected"))
	require.Equal(t, StateFailed, s.Snapshot().State)
	require.Equal(t, "peer disconnected", s.Snapshot().FailureReason)
}

func TestSessionRejectsNegativeActiveRequests(t *testing.T) {
	s, err := NewClientSession(Config{
		ProviderID:       "remote-mcp",
		SessionID:        "remote-mcp:primary",
		TransportKind:    "stdio",
		RequestedVersion: protocol.Revision20250618,
	})
	require.NoError(t, err)

	err = s.UpdateRequestCount(-1)
	require.ErrorContains(t, err, "cannot be negative")
}

func TestSessionTracksActiveSubscriptions(t *testing.T) {
	s, err := NewClientSession(Config{
		ProviderID:       "remote-mcp",
		SessionID:        "remote-mcp:primary",
		TransportKind:    "stdio",
		RequestedVersion: protocol.Revision20250618,
	})
	require.NoError(t, err)

	require.NoError(t, s.SetSubscription("file:///docs/guide.md", true))
	require.ElementsMatch(t, []string{"file:///docs/guide.md"}, s.Snapshot().ActiveSubscriptions)

	require.NoError(t, s.SetSubscription("file:///docs/guide.md", false))
	require.Empty(t, s.Snapshot().ActiveSubscriptions)
}
