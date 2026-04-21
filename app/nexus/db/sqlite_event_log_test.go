package db

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestSQLiteEventLogAppendReadAndSnapshot(t *testing.T) {
	log, err := NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()

	payload, err := json.Marshal(map[string]string{"hello": "world"})
	require.NoError(t, err)

	seqs, err := log.Append(context.Background(), "local", []core.FrameworkEvent{{
		Timestamp:      time.Now().UTC(),
		Type:           core.FrameworkEventSystemStarted,
		Payload:        payload,
		Actor:          core.EventActor{Kind: "system", ID: "relurpify"},
		IdempotencyKey: "start-1",
	}})
	require.NoError(t, err)
	require.Len(t, seqs, 1)

	seqs2, err := log.Append(context.Background(), "local", []core.FrameworkEvent{{
		Timestamp:      time.Now().UTC(),
		Type:           core.FrameworkEventSystemStarted,
		Payload:        payload,
		Actor:          core.EventActor{Kind: "system", ID: "relurpify"},
		IdempotencyKey: "start-1",
	}})
	require.NoError(t, err)
	require.Equal(t, seqs, seqs2)

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, core.FrameworkEventSystemStarted, events[0].Type)

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
		_, err := log.Append(context.Background(), partition, []core.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      eventType,
			Partition: partition,
			Actor:     core.EventActor{Kind: "system", ID: partition},
		}})
		require.NoError(t, err)
	}
	appendEvent("local", core.FrameworkEventSystemStarted)
	appendEvent("other", core.FrameworkEventMessageInbound)
	appendEvent("local", core.FrameworkEventMessageInbound)

	events, err := log.ReadByType(context.Background(), "local", "message.", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "local", events[0].Partition)
	require.Equal(t, core.FrameworkEventMessageInbound, events[0].Type)
}

func TestSQLiteEventLogCompactBefore(t *testing.T) {
	log, err := NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()

	_, err = log.Append(context.Background(), "local", []core.FrameworkEvent{
		{
			Timestamp: time.Now().UTC().Add(-48 * time.Hour),
			Type:      core.FrameworkEventSystemStarted,
			Partition: "local",
		},
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
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
	require.Equal(t, core.FrameworkEventMessageInbound, events[0].Type)
}
