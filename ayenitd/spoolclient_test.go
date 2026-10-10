package ayenitd

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/jobs"
)

func TestSpoolClient_SubmitValidity(t *testing.T) {
	stateDir := newTestStateDir(t)
	client, err := NewSpoolClient(stateDir, "relurpish", "ws-root")
	require.NoError(t, err)

	// An invalid spec is rejected before anything is written.
	_, err = client.Submit(context.Background(), jobs.Spec{Kind: "k", Queue: "q"})
	require.Error(t, err)
	dirs, err := OpenSpoolDirs(stateDir)
	require.NoError(t, err)
	require.Empty(t, pendingFiles(dirs), "a rejected spec writes nothing")

	// A valid spec returns an accepted handle with the provisional ID and
	// state queued.
	accepted, err := client.Submit(context.Background(), jobs.Spec{
		Kind: "knowledge.bootstrap", Payload: map[string]any{"workspace_root": "w"}, Queue: "knowledge",
	})
	require.NoError(t, err)
	require.Equal(t, jobs.StateQueued, accepted.State)
	require.Equal(t, "job-"+accepted.Spec.CorrelateID, accepted.ID)
	require.NotEmpty(t, accepted.Spec.CorrelateID)
}

func TestSpoolClient_CorrelateIDUniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id, err := NewCorrelateID()
		require.NoError(t, err)
		require.False(t, seen[id], "duplicate correlate id at iteration %d", i)
		seen[id] = true
	}
}

func TestSpoolClient_CrashAtomicWrite(t *testing.T) {
	// An injected marshal failure leaves only a temp file, never a final
	// name (NFR-6) — the Submit path reuses WriteSpoolFile's ordering.
	stateDir := newTestStateDir(t)
	client, err := NewSpoolClient(stateDir, "relurpish", "ws-root")
	require.NoError(t, err)
	_, err = client.Submit(context.Background(), jobs.Spec{
		Kind: "k", Queue: "q", Payload: make(chan int),
	})
	require.Error(t, err)
	dirs, err := OpenSpoolDirs(stateDir)
	require.NoError(t, err)
	require.Empty(t, pendingFiles(dirs))
}

func TestSpoolClient_DegradedModeSubmissionDrains(t *testing.T) {
	// FR-18: submission succeeds with no runner running, and a runner
	// started afterward drains the spooled file.
	stateDir := newTestStateDir(t)
	client, err := NewSpoolClient(stateDir, "relurpish", "ws-root")
	require.NoError(t, err)
	accepted, err := client.Submit(context.Background(), jobs.Spec{
		Kind: "knowledge.bootstrap", Payload: map[string]any{"workspace_root": stateDir}, Queue: "knowledge",
	})
	require.NoError(t, err, "submission must not require the runner, the store, or any lock beyond the filesystem")

	// Now start the runner against this state dir; it drains the file.
	store := openTestJobsStore(t, filepath.Join(stateDir, "jobs", "store"))
	dirs, err := OpenSpoolDirs(stateDir)
	require.NoError(t, err)
	startTestWatcher(t, dirs, store)

	require.Eventually(t, func() bool {
		list, err := store.List(context.Background(), jobs.Query{Queue: "knowledge"})
		return err == nil && len(list) == 1 && list[0].Spec.CorrelateID == accepted.Spec.CorrelateID
	}, 3*time.Second, 10*time.Millisecond, "the spooled submission is drained when a runner starts")
}
