package ayenitd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/jobs"
)

func newTestStateDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func openTestJobsStore(t *testing.T, dir string) jobs.Store {
	t.Helper()
	s, err := jobsStoreOpen(dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closer, ok := s.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	return s
}

func pendingFiles(dirs SpoolDirs) []string {
	entries, err := os.ReadDir(dirs.Pending)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func startTestWatcher(t *testing.T, dirs SpoolDirs, store jobs.Store) {
	t.Helper()
	w := newSpoolWatcher(dirs, store, nil)
	w.pollEvery = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() { _ = w.Stop() })
}

func TestSpoolWatcher_SubmitIngest(t *testing.T) {
	stateDir := newTestStateDir(t)
	dirs, err := OpenSpoolDirs(stateDir)
	require.NoError(t, err)
	store := openTestJobsStore(t, stateDir+"/jobs/store")
	startTestWatcher(t, dirs, store)

	require.NoError(t, WriteSpoolFile(dirs, spoolFile{
		CorrelateID: "turn-test-1",
		SubmittedAt: time.Now().UTC(),
		Producer:    "relurpish",
		Spec: jobs.Spec{
			Kind: "knowledge.bootstrap", Payload: map[string]any{"workspace_root": stateDir}, Queue: "knowledge",
		},
	}))

	require.Eventually(t, func() bool {
		list, err := store.List(context.Background(), jobs.Query{Queue: "knowledge"})
		return err == nil && len(list) == 1
	}, 2*time.Second, 10*time.Millisecond, "spool file must materialize as a job")

	// The claimed file is deleted on success and the pending dir drains.
	require.Eventually(t, func() bool { return len(pendingFiles(dirs)) == 0 }, time.Second, 10*time.Millisecond)
	claimed, err := os.ReadDir(dirs.Claimed)
	require.NoError(t, err)
	require.Empty(t, claimed, "claimed file must be removed after successful ingestion")
}

func TestSpoolWatcher_MalformedGoesToFailed(t *testing.T) {
	stateDir := newTestStateDir(t)
	dirs, err := OpenSpoolDirs(stateDir)
	require.NoError(t, err)
	store := openTestJobsStore(t, stateDir+"/jobs/store")
	startTestWatcher(t, dirs, store)

	// Not JSON at all.
	require.NoError(t, os.WriteFile(filepath.Join(dirs.Pending, "bad.json"), []byte("{not json"), 0o600))
	// Valid JSON, invalid spec (missing payload).
	require.NoError(t, WriteSpoolFile(dirs, spoolFile{
		CorrelateID: "turn-bad-spec",
		SubmittedAt: time.Now().UTC(),
		Spec:        jobs.Spec{Kind: "k", Queue: "knowledge"},
	}))

	require.Eventually(t, func() bool {
		failed, err := os.ReadDir(dirs.Failed)
		return err == nil && len(failed) == 2
	}, 2*time.Second, 10*time.Millisecond, "malformed files land in failed/, never retried")

	list, _ := store.List(context.Background(), jobs.Query{})
	require.Empty(t, list, "malformed submissions materialize nothing")
}

func TestSpoolWatcher_DuplicateCorrelateCounted(t *testing.T) {
	stateDir := newTestStateDir(t)
	dirs, err := OpenSpoolDirs(stateDir)
	require.NoError(t, err)
	store := openTestJobsStore(t, stateDir+"/jobs/store")
	startTestWatcher(t, dirs, store)

	sub := spoolFile{
		CorrelateID: "turn-dup",
		SubmittedAt: time.Now().UTC(),
		Spec:        jobs.Spec{Kind: "knowledge.bootstrap", Payload: map[string]any{"w": 1}, Queue: "knowledge"},
	}
	require.NoError(t, WriteSpoolFile(dirs, sub))
	require.NoError(t, WriteSpoolFile(dirs, sub))

	require.Eventually(t, func() bool {
		list, _ := store.List(context.Background(), jobs.Query{})
		return len(list) == 1 && len(pendingFiles(dirs)) == 0
	}, 2*time.Second, 10*time.Millisecond, "duplicate correlate id is counted, not errored: exactly one job")
}

func TestSpoolWatcher_BootReingestOfClaimed(t *testing.T) {
	// The crash-between-rename-and-create window, simulated deterministically:
	// a claimed/ file whose job never reached the store (the app crashed after
	// rename, before Create) is re-ingested at watcher start, idempotently by
	// CorrelateID (Q13 boot recovery).
	stateDir := newTestStateDir(t)
	dirs, err := OpenSpoolDirs(stateDir)
	require.NoError(t, err)

	// Seed the crash state by hand: the file lives in claimed/, the store is
	// empty.
	sub := spoolFile{
		CorrelateID: "turn-boot",
		SubmittedAt: time.Now().UTC(),
		Spec:        jobs.Spec{Kind: "knowledge.bootstrap", Payload: map[string]any{"w": 1}, Queue: "knowledge"},
	}
	data, err := json.Marshal(sub)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dirs.Claimed, "boot.job.json"), data, 0o600))

	store := openTestJobsStore(t, stateDir+"/jobs/store")
	startTestWatcher(t, dirs, store)

	require.Eventually(t, func() bool {
		list, err := store.List(context.Background(), jobs.Query{})
		return err == nil && len(list) == 1 && list[0].ID == "job-turn-boot"
	}, 3*time.Second, 20*time.Millisecond, "boot recovery re-ingests the claimed file")

	// The claimed file is consumed once the job exists.
	require.Eventually(t, func() bool {
		files, _ := os.ReadDir(dirs.Claimed)
		return len(files) == 0
	}, time.Second, 10*time.Millisecond)
}

func TestWriteSpoolFile_CrashAtomic(t *testing.T) {
	// Injected write failure: a temp file never appears under its final
	// name (NFR-6) — no reader observes a partial file.
	stateDir := newTestStateDir(t)
	dirs, err := OpenSpoolDirs(stateDir)
	require.NoError(t, err)

	err = WriteSpoolFile(dirs, spoolFile{
		CorrelateID: "boom",
		Spec:        jobs.Spec{Payload: make(chan int), Kind: "k", Queue: "q"},
	})
	require.Error(t, err)
	files, err := os.ReadDir(dirs.Pending)
	require.NoError(t, err)
	require.Empty(t, files, "no final name and no leftover temp file after a failed write")
}

func TestSpoolWatcher_PollIntervalDrainsRepeatably(t *testing.T) {
	stateDir := newTestStateDir(t)
	dirs, err := OpenSpoolDirs(stateDir)
	require.NoError(t, err)
	store := openTestJobsStore(t, stateDir+"/jobs/store")
	startTestWatcher(t, dirs, store)

	for i := 0; i < 5; i++ {
		require.NoError(t, WriteSpoolFile(dirs, spoolFile{
			CorrelateID: spoolCorrelate(i),
			SubmittedAt: time.Now().UTC(),
			Spec:        jobs.Spec{Kind: "k", Payload: map[string]any{}, Queue: "knowledge"},
		}))
	}
	require.Eventually(t, func() bool {
		list, _ := store.List(context.Background(), jobs.Query{})
		return len(list) == 5
	}, 3*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return len(pendingFiles(dirs)) == 0 }, time.Second, 10*time.Millisecond)
}

func spoolCorrelate(i int) string {
	return "turn-" + string(rune('a'+i))
}
