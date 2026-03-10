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
		ID:        "tok-1",
		Name:      "subject-a",
		SubjectID: "subject-a",
		TokenHash: "hash-1",
		Scopes:    []string{"nexus:admin"},
		IssuedAt:  now,
	}))

	records, err := store.ListTokens(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "tok-1", records[0].ID)
	require.Equal(t, []string{"nexus:admin"}, records[0].Scopes)

	require.NoError(t, store.RevokeToken(context.Background(), "tok-1", now.Add(time.Hour)))
	record, err := store.GetToken(context.Background(), "tok-1")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.NotNil(t, record.RevokedAt)
}
