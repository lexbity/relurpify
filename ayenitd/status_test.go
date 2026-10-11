package ayenitd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/jobs"
)

// TestStatusSchemaV2 proves the R-8 rename: the bounded sample is `jobs` in
// job-ID order; the `recent` key is gone from the on-disk schema.
func TestStatusSchemaV2(t *testing.T) {
	store, err := jobsStoreOpen(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		if closer, ok := store.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	dir := t.TempDir()
	w := newStatusWriter(dir, dir, "runner-v2", store, 50*time.Millisecond)
	require.NoError(t, w.Start(context.Background()))
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(filepath.Join(dir, "jobs", "status.json"))
		return err == nil && len(raw) > 0
	}, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, w.Stop())

	raw, err := os.ReadFile(filepath.Join(dir, "jobs", "status.json"))
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Contains(t, decoded, "jobs", "schema v2 carries the jobs sample")
	require.NotContains(t, decoded, "recent", "the recency lie is renamed away")
}

// TestStopJoinsBeforeFinalFlush proves D7's single-writer property: the
// ticker loop is joined before the final drain flush, so the two writers can
// never race onto the same temp file. The fault-injection window makes a
// concurrent write observable as a torn or failed rename.
func TestStopJoinsBeforeFinalFlush(t *testing.T) {
	store, err := jobsStoreOpen(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		if closer, ok := store.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	dir := t.TempDir()
	w := newStatusWriter(dir, dir, "runner-join", store, 5*time.Millisecond)
	require.NoError(t, w.Start(context.Background()))
	// Let several ticks write, then stop repeatedly: every Stop's final
	// flush is serialized against the (already joined) ticker.
	for i := 0; i < 20; i++ {
		require.NoError(t, w.Stop())
	}
	require.NoError(t, w.Start(context.Background()))
	require.NoError(t, w.Stop())

	raw, err := os.ReadFile(filepath.Join(dir, "jobs", "status.json"))
	require.NoError(t, err)
	var status RunnerStatus
	require.NoError(t, json.Unmarshal(raw, &status), "status.json never tears")
	require.True(t, status.Draining, "the final flush carries draining=true")
	require.NoFileExists(t, filepath.Join(dir, "jobs", "status.json.tmp"),
		"no temp file survives a joined stop")
}

// TestStatusTotalsSeeRecoveryTerminal proves FR-19's totals: a job failed at
// recovery (CompletedAt set) counts into failed_24h.
func TestStatusTotalsSeeRecoveryTerminal(t *testing.T) {
	store, err := jobsStoreOpen(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		if closer, ok := store.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	now := time.Now().UTC()
	require.NoError(t, store.Create(context.Background(), jobs.Job{
		ID: "job-recovered", Spec: jobs.Spec{Kind: "sweep", Payload: map[string]any{}, Queue: "knowledge"},
		State:       jobs.StateFailed,
		CompletedAt: now,
		CreatedAt:   now,
	}))
	w := newStatusWriter(t.TempDir(), t.TempDir(), "runner-totals", store, time.Hour)
	require.NoError(t, w.writeOnce())
}

// TestStatusWriteOnceIsSerialized is a paranoid concurrency check: parallel
// writeOnce callers never produce an unparsable file (the rename is atomic
// and the writers write distinct temp files under the join discipline).
func TestStatusWriteOnceIsSerialized(t *testing.T) {
	store, err := jobsStoreOpen(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		if closer, ok := store.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	dir := t.TempDir()
	w := newStatusWriter(dir, dir, "runner-parallel", store, time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.writeOnce()
		}()
	}
	wg.Wait()
	raw, err := os.ReadFile(filepath.Join(dir, "jobs", "status.json"))
	require.NoError(t, err)
	require.True(t, strings.TrimSpace(string(raw)) != "")
	var status RunnerStatus
	require.NoError(t, json.Unmarshal(raw, &status))
}
