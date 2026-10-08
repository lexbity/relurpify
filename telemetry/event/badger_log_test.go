package event

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestBadgerLog(t *testing.T) *BadgerLog {
	t.Helper()
	log, err := NewBadgerLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	return log
}

func TestBadgerLog_AppendAssignsPerPartitionMonotonicSeqs(t *testing.T) {
	log := newTestBadgerLog(t)
	ctx := context.Background()

	seqs, err := log.Append(ctx, "local", []FrameworkEvent{{Type: "a.v1"}, {Type: "b.v1"}})
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 2}, seqs)

	seqs, err = log.Append(ctx, "local", []FrameworkEvent{{Type: "c.v1"}})
	require.NoError(t, err)
	require.Equal(t, []uint64{3}, seqs)

	// Partitions sequence independently.
	seqs, err = log.Append(ctx, "other", []FrameworkEvent{{Type: "a.v1"}})
	require.NoError(t, err)
	require.Equal(t, []uint64{1}, seqs)

	last, err := log.LastSeq(ctx, "local")
	require.NoError(t, err)
	require.Equal(t, uint64(3), last)
	last, err = log.LastSeq(ctx, "other")
	require.NoError(t, err)
	require.Equal(t, uint64(1), last)
}

func TestBadgerLog_ReadAfterSeqAndLimit(t *testing.T) {
	log := newTestBadgerLog(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, err := log.Append(ctx, "local", []FrameworkEvent{{Type: "e.v1"}})
		require.NoError(t, err)
	}

	out, err := log.Read(ctx, "local", 2, 0, false)
	require.NoError(t, err)
	require.Len(t, out, 3)
	for _, ev := range out {
		require.Greater(t, ev.Seq, uint64(2))
		require.Equal(t, "local", ev.Partition)
	}

	out, err = log.Read(ctx, "local", 0, 2, false)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, uint64(1), out[0].Seq)
	require.Equal(t, uint64(2), out[1].Seq)
}

func TestBadgerLog_ReadFollowReturnsNewEvents(t *testing.T) {
	log := newTestBadgerLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = log.Append(context.Background(), "local", []FrameworkEvent{{Type: "late.v1"}})
	}()

	out, err := log.Read(ctx, "local", 0, 1, true)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "late.v1", out[0].Type)
}

func TestBadgerLog_ReadFollowTerminatesOnContextCancel(t *testing.T) {
	log := newTestBadgerLog(t)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	start := time.Now()
	out, err := log.Read(ctx, "local", 0, 5, true)
	require.Error(t, err, "follow must terminate on context cancellation")
	require.Empty(t, out)
	require.Less(t, time.Since(start), 2*time.Second)
}

func TestBadgerLog_ReadByTypePrefix(t *testing.T) {
	log := newTestBadgerLog(t)
	ctx := context.Background()
	_, err := log.Append(ctx, "local", []FrameworkEvent{
		{Type: "policy.evaluated.v1"},
		{Type: "agent.run.started.v1"},
		{Type: "policy.evaluated.v1"},
	})
	require.NoError(t, err)

	out, err := log.ReadByType(ctx, "local", "policy.evaluated", 0, 0)
	require.NoError(t, err)
	require.Len(t, out, 2)
	for _, ev := range out {
		require.Equal(t, "policy.evaluated.v1", ev.Type)
	}
}

func TestBadgerLog_SnapshotRoundTrip(t *testing.T) {
	log := newTestBadgerLog(t)
	ctx := context.Background()

	seq, data, err := log.LoadSnapshot(ctx, "local")
	require.NoError(t, err)
	require.Zero(t, seq)
	require.Nil(t, data)

	require.NoError(t, log.TakeSnapshot(ctx, "local", 7, []byte("state-1")))
	loadedSeq, loadedData, err := log.LoadSnapshot(ctx, "local")
	require.NoError(t, err)
	require.Equal(t, uint64(7), loadedSeq)
	require.Equal(t, []byte("state-1"), loadedData)
}

func TestBadgerLog_SurvivesCloseAndReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "events.db")
	log, err := NewBadgerLog(dir)
	require.NoError(t, err)
	_, err = log.Append(context.Background(), "local", []FrameworkEvent{{
		Type:    "durable.v1",
		Payload: []byte(`{"k":1}`),
	}})
	require.NoError(t, err)
	require.NoError(t, log.Close())
	require.NoError(t, log.Close(), "double close is a no-op")

	reopened, err := NewBadgerLog(dir)
	require.NoError(t, err)
	out, err := reopened.Read(context.Background(), "local", 0, 0, false)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, uint64(1), out[0].Seq)
	require.Equal(t, "durable.v1", out[0].Type)
	require.NoError(t, reopened.Close())
}

// TestBadgerLog_FailClosedOnUnwritablePath verifies a log that cannot open
// surfaces an error instead of a usable-looking instance; the composition
// root downgrades to JSONL-only telemetry (NFR-4).
func TestBadgerLog_FailClosedOnUnwritablePath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	_, err := NewBadgerLog(file)
	require.Error(t, err)
}
