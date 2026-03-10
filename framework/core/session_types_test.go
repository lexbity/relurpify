package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionBoundaryKeyMain(t *testing.T) {
	require.Equal(t, "local", SessionBoundaryKey(SessionScopeMain, "local", "telegram", "peer-1", "thread-1"))
}

func TestSessionBoundaryKeyPerChannelPeer(t *testing.T) {
	require.Equal(t, "local:telegram:peer-1", SessionBoundaryKey(SessionScopePerChannelPeer, "local", "telegram", "peer-1", "thread-1"))
}

func TestSessionBoundaryKeyPerThread(t *testing.T) {
	require.Equal(t, "local:telegram:peer-1:thread-1", SessionBoundaryKey(SessionScopePerThread, "local", "telegram", "peer-1", "thread-1"))
}

func TestSessionBoundaryKeyUnknownScopeDefaultsToChannelPeer(t *testing.T) {
	require.Equal(t, "local:telegram:peer-1", SessionBoundaryKey(SessionScope("other"), "local", "telegram", "peer-1", "thread-1"))
}
