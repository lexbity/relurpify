package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/telemetry/event"
)

func testContext(t *testing.T) context.Context {
	return context.Background()
}

// TestOpenRuntimeEventLogFactory_WiredInCompositionRoot verifies the FR-4
// composition assignment produces a live durable event.Log (Append/Read
// round-trip), so "EventLogFactory is assigned" is a behavioral claim, not a
// field name.
func TestOpenRuntimeEventLogFactory_WiredInCompositionRoot(t *testing.T) {
	require.NotNil(t, openRuntimeEventLogFactory)

	dir := filepath.Join(t.TempDir(), "events.db")
	log, err := openRuntimeEventLogFactory(dir)
	require.NoError(t, err)
	require.NotNil(t, log)
	bounded, ok := log.(*event.BadgerLog)
	require.True(t, ok, "the runtime event log must be the badger implementation")

	seqs, err := log.Append(testContext(t), "local", []event.FrameworkEvent{
		{Type: event.EventAgentRunStarted},
	})
	require.NoError(t, err)
	require.Equal(t, []uint64{1}, seqs)

	out, err := log.Read(testContext(t), "local", 0, 0, false)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, event.EventAgentRunStarted, out[0].Type)
	require.NoError(t, bounded.Close())
}
