package runtime

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJournalAdapterAdmitCommand tests durable idempotent admission per D-08.
func TestJournalAdapterAdmitCommand(t *testing.T) {
	t.Run("new request gets accepted", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		db, err := graphdb.Open(ctx, graphdb.DefaultOptions(tmpDir))
		require.NoError(t, err)
		defer func() {
			_ = db.Close(ctx)
		}()

		workspace := WorkspaceID("test_workspace")
		ja := NewJournalAdapter(db, workspace)

		// Submit a new request
		result, err := ja.AdmitCommand(ctx, "", "digest123", "target1")
		require.NoError(t, err)
		assert.Equal(t, ReceiptStatusAccepted, result.Status)
		assert.NotEmpty(t, result.Receipt.RequestID)
		assert.Equal(t, "digest123", result.Receipt.Digest)
		assert.Equal(t, "target1", result.Receipt.Target)
		assert.False(t, result.Conflict)
	})

	t.Run("same ID and digest returns existing receipt", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		db, err := graphdb.Open(ctx, graphdb.DefaultOptions(tmpDir))
		require.NoError(t, err)
		defer func() {
			_ = db.Close(ctx)
		}()

		workspace := WorkspaceID("test_workspace")
		ja := NewJournalAdapter(db, workspace)

		// Submit first request
		result1, err := ja.AdmitCommand(ctx, "", "digest123", "target1")
		require.NoError(t, err)
		requestID := result1.Receipt.RequestID

		// Submit same request with same ID
		result2, err := ja.AdmitCommand(ctx, requestID, "digest123", "target1")
		require.NoError(t, err)
		assert.Equal(t, requestID, result2.Receipt.RequestID)
		assert.Equal(t, "digest123", result2.Receipt.Digest)
	})

	t.Run("same ID different digest returns conflict", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		db, err := graphdb.Open(ctx, graphdb.DefaultOptions(tmpDir))
		require.NoError(t, err)
		defer func() {
			_ = db.Close(ctx)
		}()

		workspace := WorkspaceID("test_workspace")
		ja := NewJournalAdapter(db, workspace)

		// Submit first request
		result1, err := ja.AdmitCommand(ctx, "", "digest123", "target1")
		require.NoError(t, err)
		requestID := result1.Receipt.RequestID

		// Submit same ID with different digest
		result2, err := ja.AdmitCommand(ctx, requestID, "different_digest", "target1")
		assert.True(t, result2.Conflict)
		assert.Equal(t, ErrReceiptConflict, err)
	})

	t.Run("receipt survives close and reopen", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		db, err := graphdb.Open(ctx, graphdb.DefaultOptions(tmpDir))
		require.NoError(t, err)

		workspace := WorkspaceID("test_workspace")
		ja := NewJournalAdapter(db, workspace)

		// Submit a request
		result1, err := ja.AdmitCommand(ctx, "", "digest123", "target1")
		require.NoError(t, err)
		requestID := result1.Receipt.RequestID

		// Close and reopen
		err = db.Close(ctx)
		require.NoError(t, err)

		db2, err := graphdb.Open(ctx, graphdb.DefaultOptions(tmpDir))
		require.NoError(t, err)
		defer func() {
			_ = db2.Close(ctx)
		}()

		ja2 := NewJournalAdapter(db2, workspace)

		// Same request should return existing receipt
		result2, err := ja2.AdmitCommand(ctx, requestID, "digest123", "target1")
		require.NoError(t, err)
		assert.Equal(t, requestID, result2.Receipt.RequestID)
		assert.Equal(t, "digest123", result2.Receipt.Digest)
	})
}

func TestV2RunAdmissionIsAtomicIdempotentAndReopens(t *testing.T) {
	ctx := context.Background()
	opts := graphdb.DefaultOptions(t.TempDir())
	db, err := graphdb.Open(ctx, opts)
	require.NoError(t, err)
	id := generateRequestID()
	const count = 24
	runs := make([]string, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store := NewRunAdmissionStore("workspace", db)
			run, err := store.AdmitRun(ctx, id, "intent")
			errs[i] = err
			if run != nil {
				runs[i] = run.RunID
			}
		}()
	}
	wg.Wait()
	for i := range count {
		require.NoError(t, errs[i])
		require.NotEmpty(t, runs[i])
		require.Equal(t, runs[0], runs[i])
	}
	require.Len(t, db.ListNodes("run"), 1)
	require.Len(t, db.ListNodes("receipt"), 1)
	require.NoError(t, db.Close(ctx))
	db, err = graphdb.Open(ctx, opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close(ctx)) })
	store := NewRunAdmissionStore("workspace", db)
	run, err := store.AdmitRun(ctx, id, "intent")
	require.NoError(t, err)
	require.Equal(t, runs[0], run.RunID)
	run.Status = "tampered local copy"
	run, err = store.AdmitRun(ctx, id, "intent")
	require.NoError(t, err)
	require.Equal(t, "queued", run.Status)
	_, err = store.AdmitRun(ctx, id, "different")
	require.ErrorIs(t, err, ErrReceiptConflict)
	require.Len(t, db.ListNodes("run"), 1)
}

func TestV2AdmissionFreshnessAndClockDoNotBypassConflict(t *testing.T) {
	ctx := context.Background()
	db, err := graphdb.Open(ctx, graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close(ctx)) }()
	ja := NewJournalAdapter(db, "workspace")
	base := time.Now().UTC()
	ja.now = func() time.Time { return base }
	original, err := ja.AdmitCommand(ctx, "", "digest", "target")
	require.NoError(t, err)
	require.Equal(t, original.OperationID, original.Receipt.OperationID)
	ja.now = func() time.Time { return base.Add(6 * time.Minute) }
	_, err = ja.AdmitCommand(ctx, original.Receipt.RequestID, "other", "target")
	require.ErrorIs(t, err, ErrReceiptConflict)
	_, err = ja.AdmitCommand(ctx, original.Receipt.RequestID, "digest", "other")
	require.ErrorIs(t, err, ErrReceiptConflict)
	retry, err := ja.AdmitCommand(ctx, original.Receipt.RequestID, "digest", "target")
	require.NoError(t, err)
	require.Equal(t, original, retry)
	_, err = ja.AdmitCommand(ctx, generateRequestID(), "new", "target")
	require.ErrorIs(t, err, ErrReceiptExpired)
	ja.now = func() time.Time { return base.Add(-31 * time.Second) }
	// This UUID still meets the future-skew allowance at -30s; use a previously
	// issued ID to isolate the clock fence from freshness validation.
	id := requestIDAt(t, base.Add(-time.Minute))
	_, err = ja.AdmitCommand(ctx, id, "new", "target")
	require.ErrorContains(t, err, "clock moved backward")
	_, err = ja.AdmitCommand(ctx, original.Receipt.RequestID, "digest", "target")
	require.NoError(t, err)
	require.Len(t, db.ListNodes("receipt"), 1)
}

func requestIDAt(t *testing.T, at time.Time) RequestID {
	t.Helper()
	raw, err := hex.DecodeString(string(generateRequestID())[4:])
	require.NoError(t, err)
	var timestamp [8]byte
	binary.BigEndian.PutUint64(timestamp[:], uint64(at.UnixMilli()))
	copy(raw[:6], timestamp[2:])
	return RequestID("req_" + hex.EncodeToString(raw))
}

func TestV2AdmissionWorkspaceIsolationAndCancellation(t *testing.T) {
	ctx := context.Background()
	db, err := graphdb.Open(ctx, graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close(ctx)) }()
	a, b := NewJournalAdapter(db, "a"), NewJournalAdapter(db, "b")
	id := generateRequestID()
	_, err = a.AdmitCommand(ctx, id, "one", "target")
	require.NoError(t, err)
	_, err = b.AdmitCommand(ctx, id, "two", "target")
	require.NoError(t, err)
	require.Len(t, db.ListNodes("receipt"), 2)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = a.AdmitCommand(canceled, "", "new", "target")
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, db.ListNodes("receipt"), 2)
}
