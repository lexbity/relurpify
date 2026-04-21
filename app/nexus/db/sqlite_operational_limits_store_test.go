package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"github.com/stretchr/testify/require"
)

func TestSQLiteOperationalLimiterPersistsWindowAndSlots(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fmp_limits.db")
	limits := fwfmp.OperationalLimits{
		Window:                time.Minute,
		MaxActiveResumeSlots:  1,
		MaxResumeBytesWindow:  512,
		MaxForwardBytesWindow: 1024,
		MaxFederatedForwards:  1,
	}
	store, err := NewSQLiteOperationalLimiter(path, limits)
	require.NoError(t, err)

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	refusal, err := store.AcquireResume(context.Background(), "slot-1", 256, now)
	require.NoError(t, err)
	require.Nil(t, refusal)
	snapshot, err := store.Snapshot(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, 1, snapshot.ActiveResumeSlots)
	require.Equal(t, int64(256), snapshot.ResumeBytesInWindow)
	require.NoError(t, store.Close())

	reopened, err := NewSQLiteOperationalLimiter(path, limits)
	require.NoError(t, err)
	defer reopened.Close()
	snapshot, err = reopened.Snapshot(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, 1, snapshot.ActiveResumeSlots)
	require.Equal(t, int64(256), snapshot.ResumeBytesInWindow)

	refusal, err = reopened.AcquireResume(context.Background(), "slot-2", 256, now)
	require.NoError(t, err)
	require.NotNil(t, refusal)
	require.Equal(t, core.RefusalDestinationBusy, refusal.Code)
}

func TestSQLiteOperationalLimiterResetsExpiredWindow(t *testing.T) {
	t.Parallel()

	store, err := NewSQLiteOperationalLimiter(filepath.Join(t.TempDir(), "fmp_limits.db"), fwfmp.OperationalLimits{
		Window:                time.Minute,
		MaxForwardBytesWindow: 512,
		MaxFederatedForwards:  1,
	})
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	refusal, err := store.AllowForward(context.Background(), "tx-1", 256, now)
	require.NoError(t, err)
	require.Nil(t, refusal)
	refusal, err = store.AllowForward(context.Background(), "tx-2", 256, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.Nil(t, refusal)
	snapshot, err := store.Snapshot(context.Background(), now.Add(2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, int64(256), snapshot.ForwardBytesInWindow)
	require.Equal(t, 1, snapshot.FederatedForwards)
}
