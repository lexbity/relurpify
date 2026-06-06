package persistence

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/governance/identity"
	"github.com/stretchr/testify/require"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestSQLiteEventLogAppendReadAndSnapshot(t *testing.T) {
	log, err := NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()

	payload, err := json.Marshal(map[string]string{"hello": "world"})
	require.NoError(t, err)

	seqs, err := log.Append(context.Background(), "local", []telemetry.FrameworkEvent{{
		Timestamp:      time.Now().UTC(),
		Type:           telemetry.FrameworkEventSystemStarted,
		Payload:        payload,
		Actor:          identity.EventActor{Kind: "system", ID: "relurpify"},
		IdempotencyKey: "start-1",
	}})
	require.NoError(t, err)
	require.Len(t, seqs, 1)

	seqs2, err := log.Append(context.Background(), "local", []telemetry.FrameworkEvent{{
		Timestamp:      time.Now().UTC(),
		Type:           telemetry.FrameworkEventSystemStarted,
		Payload:        payload,
		Actor:          identity.EventActor{Kind: "system", ID: "relurpify"},
		IdempotencyKey: "start-1",
	}})
	require.NoError(t, err)
	require.Equal(t, seqs, seqs2)

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, telemetry.FrameworkEventSystemStarted, events[0].Type)

	require.NoError(t, log.TakeSnapshot(context.Background(), "local", events[0].Seq, []byte("snapshot")))
	seq, data, err := log.LoadSnapshot(context.Background(), "local")
	require.NoError(t, err)
	require.Equal(t, events[0].Seq, seq)
	require.Equal(t, []byte("snapshot"), data)
}

func TestSQLiteEventLogReadByTypeAndPartition(t *testing.T) {
	log, err := NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()

	appendEvent := func(partition, eventType string) {
		_, err := log.Append(context.Background(), partition, []telemetry.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      eventType,
			Partition: partition,
			Actor:     identity.EventActor{Kind: "system", ID: partition},
		}})
		require.NoError(t, err)
	}
	appendEvent("local", telemetry.FrameworkEventSystemStarted)
	appendEvent("other", telemetry.FrameworkEventMessageInbound)
	appendEvent("local", telemetry.FrameworkEventMessageInbound)

	events, err := log.ReadByType(context.Background(), "local", "message.", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "local", events[0].Partition)
	require.Equal(t, telemetry.FrameworkEventMessageInbound, events[0].Type)
}

func TestSQLiteEventLogCompactBefore(t *testing.T) {
	log, err := NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()

	_, err = log.Append(context.Background(), "local", []telemetry.FrameworkEvent{
		{
			Timestamp: time.Now().UTC().Add(-48 * time.Hour),
			Type:      telemetry.FrameworkEventSystemStarted,
			Partition: "local",
		},
		{
			Timestamp: time.Now().UTC(),
			Type:      telemetry.FrameworkEventMessageInbound,
			Partition: "local",
		},
	})
	require.NoError(t, err)

	deleted, err := log.CompactBefore(context.Background(), time.Now().UTC().Add(-24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, telemetry.FrameworkEventMessageInbound, events[0].Type)
}
