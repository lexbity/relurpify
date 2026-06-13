package artifactstore

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	Relurpify_state_gc_test = ".relurpify_state"
	Artifacts_gc_test = "artifacts"
	Old_gc_test = "old"
	Remove_gc_test = "remove"
	Session_gc_test = "session"
	Small_gc_test = "small"
	Test_gc_test = "test"
	Text_gc_test = "text"
)


func TestGCSessionRemovesOnlyThatSession(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Put artifacts in two sessions.
	ref1, err := store.Put(ctx, Text_gc_test, map[string]string{Session_gc_test: "keep"}, strings.NewReader("keep-data"))
	require.NoError(t, err)

	_, err = store.Put(ctx, Text_gc_test, map[string]string{Session_gc_test: Remove_gc_test}, strings.NewReader("remove-data"))
	require.NoError(t, err)

	// GC the "remove" session.
	err = store.GC(ctx, Remove_gc_test)
	require.NoError(t, err)

	// "keep" session artifacts should still exist.
	rc, _, err := store.Open(ctx, ref1)
	require.NoError(t, err)
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	require.Equal(t, "keep-data", string(data))

	// "remove" session directory should be gone.
	removeDir := filepath.Join(workspace, Relurpify_state_gc_test, Artifacts_gc_test, Remove_gc_test)
	_, err = os.Stat(removeDir)
	require.True(t, os.IsNotExist(err), "removed session dir should not exist")
}

func TestGCSessionIdempotent(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_, err = store.Put(ctx, Text_gc_test, map[string]string{Session_gc_test: Test_gc_test}, strings.NewReader("data"))
	require.NoError(t, err)

	// First GC should succeed.
	err = store.GC(ctx, Test_gc_test)
	require.NoError(t, err)

	// Second GC (idempotent) should not error.
	err = store.GC(ctx, Test_gc_test)
	require.NoError(t, err)
}

func TestGCAgeRemovesOldSessions(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Put artifacts in two sessions.
	_, err = store.Put(ctx, Text_gc_test, map[string]string{Session_gc_test: Old_gc_test}, strings.NewReader("old-data"))
	require.NoError(t, err)

	_, err = store.Put(ctx, Text_gc_test, map[string]string{Session_gc_test: "new"}, strings.NewReader("new-data"))
	require.NoError(t, err)

	// Manually set the "old" session mod time to be older.
	oldDir := filepath.Join(workspace, Relurpify_state_gc_test, Artifacts_gc_test, Old_gc_test)
	past := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(oldDir, past, past))

	store.mu.Lock()
	if oldSess, ok := store.sessions[Old_gc_test]; ok {
		oldSess.ModTime = past
	}
	store.mu.Unlock()

	// GC with 1 hour max age should remove "old".
	result, err := store.GCAge(ctx, 1*time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, result.SessionsRemoved)
	require.Positive(t, result.BytesFreed)

	// "new" session should remain.
	newDir := filepath.Join(workspace, Relurpify_state_gc_test, Artifacts_gc_test, "new")
	_, err = os.Stat(newDir)
	require.NoError(t, err, "new session should remain")
}

func TestEvictOldest(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 1024*1024) // 1 MiB cap
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Put a large artifact.
	largeData := strings.Repeat("A", 500*1024) // 500 KiB
	_, err = store.Put(ctx, Text_gc_test, map[string]string{Session_gc_test: "large"}, strings.NewReader(largeData))
	require.NoError(t, err)

	// Put a small artifact.
	_, err = store.Put(ctx, Text_gc_test, map[string]string{Session_gc_test: Small_gc_test}, strings.NewReader(Small_gc_test))
	require.NoError(t, err)

	// Ensure deterministic ordering for tests.
	store.mu.Lock()
	if largeSess, ok := store.sessions["large"]; ok {
		largeSess.ModTime = time.Now().Add(-1 * time.Hour)
	}
	if smallSess, ok := store.sessions[Small_gc_test]; ok {
		smallSess.ModTime = time.Now()
	}
	store.mu.Unlock()

	// Evict to 100 KiB target — "large" should be removed first (it's older).
	result, err := store.EvictOldest(ctx, 100*1024)
	require.NoError(t, err)
	require.Equal(t, 1, result.SessionsRemoved)

	// "small" session should remain.
	smallDir := filepath.Join(workspace, Relurpify_state_gc_test, Artifacts_gc_test, Small_gc_test)
	_, err = os.Stat(smallDir)
	require.NoError(t, err, "small session should remain")
}

func TestEvictOldestNoOpWhenUnderTarget(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_, err = store.Put(ctx, Text_gc_test, nil, strings.NewReader(Small_gc_test))
	require.NoError(t, err)

	// Target is much larger than stored data — no eviction needed.
	total := store.TotalBytes()
	result, err := store.EvictOldest(ctx, total+1)
	require.NoError(t, err)
	require.Equal(t, 0, result.SessionsRemoved)
}

func TestTotalBytesAfterPut(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	_, err = store.Put(context.Background(), Text_gc_test, nil, strings.NewReader("hello"))
	require.NoError(t, err)
	require.Equal(t, int64(5), store.TotalBytes())
}
