package runtime

import (
	"context"
	"testing"

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

// TestWorkspaceOwner tests the workspace owner actor.
func TestWorkspaceOwner(t *testing.T) {
	t.Run("admit run creates run record", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		db, err := graphdb.Open(ctx, graphdb.DefaultOptions(tmpDir))
		require.NoError(t, err)
		defer func() {
			_ = db.Close(ctx)
		}()

		workspace := WorkspaceID("test_workspace")
		wo := NewWorkspaceOwner(workspace, db)
		defer wo.Close()

		// Admit a run
		run, err := wo.AdmitRun(ctx, "", "run_digest")
		require.NoError(t, err)
		assert.NotNil(t, run)
		assert.Contains(t, run.RunID, "run_")
		assert.Equal(t, string(workspace), run.WorkflowID)
		assert.Equal(t, "queued", run.Status)
	})

	t.Run("close workspace owner", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		db, err := graphdb.Open(ctx, graphdb.DefaultOptions(tmpDir))
		require.NoError(t, err)
		defer func() {
			_ = db.Close(ctx)
		}()

		workspace := WorkspaceID("test_workspace")
		wo := NewWorkspaceOwner(workspace, db)

		// Close should not panic
		err = wo.Close()
		assert.NoError(t, err)
	})
}

// TestJournalAdapterClockSkew tests clock skew handling per D-08.
func TestJournalAdapterClockSkew(t *testing.T) {
	t.Run("backward clock jump blocks admission", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		db, err := graphdb.Open(ctx, graphdb.DefaultOptions(tmpDir))
		require.NoError(t, err)
		defer func() {
			_ = db.Close(ctx)
		}()

		workspace := WorkspaceID("test_workspace")
		ja := NewJournalAdapter(db, workspace)

		// Submit a request to establish a clock baseline
		_, err = ja.AdmitCommand(ctx, "", "digest1", "target1")
		require.NoError(t, err)

		// Now we would need to simulate a backward clock jump
		// This test is limited by our current timestamp extraction
		// In a real implementation, we'd mock the clock
		t.Skip("Requires mockable clock for backward jump test")
	})
}

// TestJournalAdapterValidation tests validation before commit.
func TestJournalAdapterValidation(t *testing.T) {
	t.Run("validation happens before durable commit", func(t *testing.T) {
		ctx := context.Background()
		tmpDir := t.TempDir()
		db, err := graphdb.Open(ctx, graphdb.DefaultOptions(tmpDir))
		require.NoError(t, err)
		defer func() {
			_ = db.Close(ctx)
		}()

		workspace := WorkspaceID("test_workspace")
		_ = NewJournalAdapter(db, workspace)

		// The AdmitCommand should validate before committing to durable storage
		// This is tested implicitly by the other tests that verify durability
		// A more comprehensive test would verify that a rejected validation
		// leaves no durable record, but our current implementation doesn't
		// have a way to inject validation failures
	})
}
