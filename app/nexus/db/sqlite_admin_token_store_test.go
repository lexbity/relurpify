package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestSQLiteAdminTokenStoreCreateListAndRevoke(t *testing.T) {
	store, err := NewSQLiteAdminTokenStore(filepath.Join(t.TempDir(), "admin_tokens.db"))
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.CreateToken(context.Background(), core.AdminTokenRecord{
		ID:          "tok-1",
		Name:        "subject-a",
		TenantID:    "tenant-1",
		SubjectKind: core.SubjectKindServiceAccount,
		SubjectID:   "subject-a",
		TokenHash:   "hash-1",
		Scopes:      []string{"nexus:admin"},
		IssuedAt:    now,
	}))

	records, err := store.ListTokens(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "tok-1", records[0].ID)
	require.Equal(t, "tenant-1", records[0].TenantID)
	require.Equal(t, core.SubjectKindServiceAccount, records[0].SubjectKind)
	require.Equal(t, []string{"nexus:admin"}, records[0].Scopes)

	recordByHash, err := store.GetTokenByHash(context.Background(), "hash-1")
	require.NoError(t, err)
	require.NotNil(t, recordByHash)
	require.Equal(t, "tok-1", recordByHash.ID)

	require.NoError(t, store.RevokeToken(context.Background(), "tok-1", now.Add(time.Hour)))
	record, err := store.GetToken(context.Background(), "tok-1")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.NotNil(t, record.RevokedAt)
}

func TestSQLiteAdminTokenStoreCreatesTokenHashIndex(t *testing.T) {
	store, err := NewSQLiteAdminTokenStore(filepath.Join(t.TempDir(), "admin_tokens.db"))
	require.NoError(t, err)
	defer store.Close()

	row := store.db.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, "idx_admin_tokens_token_hash")
	var name string
	err = row.Scan(&name)
	require.NoError(t, err)
	require.Equal(t, "idx_admin_tokens_token_hash", name)
}
