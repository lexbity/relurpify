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

func TestSQLiteAuditChainStoreRoundTripAndVerify(t *testing.T) {
	t.Parallel()

	signer := fwfmp.NewEd25519SignerFromSeed([]byte("audit-chain-roundtrip"))
	store, err := NewSQLiteAuditChainStore(
		filepath.Join(t.TempDir(), "audit.db"),
		signer,
		&fwfmp.Ed25519Verifier{PublicKey: signer.PublicKey()},
	)
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC)
	require.NoError(t, store.Log(context.Background(), core.AuditRecord{
		Timestamp:   now,
		AgentID:     "runtime-a",
		Action:      "fmp",
		Type:        core.FrameworkEventFMPHandoffOffered,
		Permission:  "mesh",
		Result:      "ok",
		Correlation: "offer-1",
		Metadata: map[string]any{
			"lineage_id": "lineage-1",
			"offer_id":   "offer-1",
		},
	}))
	require.NoError(t, store.Log(context.Background(), core.AuditRecord{
		Timestamp:   now.Add(time.Second),
		AgentID:     "runtime-b",
		Action:      "fmp",
		Type:        core.FrameworkEventFMPResumeCommitted,
		Permission:  "mesh",
		Result:      "ok",
		Correlation: "offer-1",
		Metadata: map[string]any{
			"lineage_id":  "lineage-1",
			"new_attempt": "attempt-2",
		},
	}))

	entries, err := store.ReadChain(context.Background(), core.AuditChainFilter{LineageID: "lineage-1"})
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Empty(t, entries[0].PreviousHash)
	require.Equal(t, entries[0].RecordHash, entries[1].PreviousHash)
	require.Equal(t, "lineage-1", entries[0].Record.Metadata["lineage_id"])

	verification, err := store.VerifyChain(context.Background(), core.AuditChainFilter{LineageID: "lineage-1"})
	require.NoError(t, err)
	require.True(t, verification.Verified)
	require.Equal(t, 2, verification.EntryCount)
	require.Equal(t, entries[1].RecordHash, verification.LastHash)
}

func TestSQLiteAuditChainStoreDetectsTampering(t *testing.T) {
	t.Parallel()

	signer := fwfmp.NewEd25519SignerFromSeed([]byte("audit-chain-tamper"))
	store, err := NewSQLiteAuditChainStore(
		filepath.Join(t.TempDir(), "audit.db"),
		signer,
		&fwfmp.Ed25519Verifier{PublicKey: signer.PublicKey()},
	)
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.Log(context.Background(), core.AuditRecord{
		AgentID:    "runtime-a",
		Action:     "fmp",
		Type:       core.FrameworkEventFMPHandoffOffered,
		Permission: "mesh",
		Result:     "ok",
		Metadata: map[string]any{
			"lineage_id": "lineage-1",
			"offer_id":   "offer-1",
		},
	}))
	require.NoError(t, store.Log(context.Background(), core.AuditRecord{
		AgentID:    "runtime-a",
		Action:     "fmp",
		Type:       core.FrameworkEventFMPResumeCommitted,
		Permission: "mesh",
		Result:     "ok",
		Metadata: map[string]any{
			"lineage_id":  "lineage-1",
			"new_attempt": "attempt-2",
		},
	}))

	_, err = store.db.Exec(`UPDATE fmp_audit_chain SET metadata_json = '{"lineage_id":"lineage-1","offer_id":"mutated"}' WHERE seq = 1`)
	require.NoError(t, err)

	verification, err := store.VerifyChain(context.Background(), core.AuditChainFilter{LineageID: "lineage-1"})
	require.NoError(t, err)
	require.False(t, verification.Verified)
	require.Contains(t, verification.Failure, "sequence 1")
}

func TestSQLiteAuditChainStoreNilVerifierAndInvalidPath(t *testing.T) {
	t.Parallel()

	_, err := NewSQLiteAuditChainStore("   ", nil, nil)
	require.Error(t, err)

	store, err := NewSQLiteAuditChainStore(filepath.Join(t.TempDir(), "audit.db"), fwfmp.NewEd25519SignerFromSeed([]byte("audit-chain-nil-verifier")), nil)
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.Log(context.Background(), core.AuditRecord{
		AgentID:    "runtime-a",
		Action:     "fmp",
		Type:       core.FrameworkEventFMPHandoffOffered,
		Permission: "mesh",
		Result:     "ok",
		Metadata: map[string]any{
			"lineage_id": "lineage-1",
		},
	}))

	verification, err := store.VerifyChain(context.Background(), core.AuditChainFilter{LineageID: "lineage-1"})
	require.NoError(t, err)
	require.True(t, verification.Verified)
	require.Equal(t, 1, verification.EntryCount)
}
