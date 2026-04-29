package admin

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestCreateSubjectThenIssueToken(t *testing.T) {
	t.Parallel()

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	tokenStore, err := db.NewSQLiteAdminTokenStore(filepath.Join(t.TempDir(), "tokens.db"))
	require.NoError(t, err)
	defer tokenStore.Close()

	svc := NewService(ServiceConfig{
		Identities: identityStore,
		Tokens:     tokenStore,
	}).(*service)

	createResult, err := svc.CreateSubject(context.Background(), CreateSubjectRequest{
		AdminRequest:    AdminRequest{TenantID: "tenant-1"},
		SubjectTenantID: "tenant-1",
		SubjectKind:     core.SubjectKindServiceAccount,
		SubjectID:       "svc-1",
		DisplayName:     "Service 1",
		Roles:           []string{"operator"},
	})
	require.NoError(t, err)
	require.Equal(t, "svc-1", createResult.Subject.ID)

	tokenResult, err := svc.IssueToken(context.Background(), IssueTokenRequest{
		AdminRequest:    AdminRequest{TenantID: "tenant-1"},
		SubjectTenantID: "tenant-1",
		SubjectKind:     core.SubjectKindServiceAccount,
		SubjectID:       "svc-1",
		Scopes:          []string{"nexus:operator"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, tokenResult.Token)

	record, err := tokenStore.GetToken(context.Background(), tokenResult.TokenID)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, "tenant-1", record.TenantID)
	require.Equal(t, core.SubjectKindServiceAccount, record.SubjectKind)
}

func TestBindExternalIdentity(t *testing.T) {
	t.Parallel()

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	svc := NewService(ServiceConfig{
		Identities: identityStore,
	}).(*service)

	_, err = svc.CreateSubject(context.Background(), CreateSubjectRequest{
		AdminRequest:    AdminRequest{TenantID: "tenant-1"},
		SubjectTenantID: "tenant-1",
		SubjectKind:     core.SubjectKindUser,
		SubjectID:       "user-1",
		DisplayName:     "User 1",
	})
	require.NoError(t, err)

	result, err := svc.BindExternalIdentity(context.Background(), BindExternalIdentityRequest{
		AdminRequest:    AdminRequest{TenantID: "tenant-1"},
		SubjectTenantID: "tenant-1",
		Provider:        identity.ExternalProviderDiscord,
		AccountID:       "guild-1",
		ExternalID:      "discord-user-1",
		SubjectKind:     core.SubjectKindUser,
		SubjectID:       "user-1",
		DisplayName:     "Discord User 1",
		ProviderLabel:   "Guild 1",
	})
	require.NoError(t, err)
	require.Equal(t, "tenant-1", result.Identity.TenantID)
	require.Equal(t, "user-1", result.Identity.Subject.ID)

	record, err := identityStore.GetExternalIdentity(context.Background(), "tenant-1", identity.ExternalProviderDiscord, "guild-1", "discord-user-1")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, "user-1", record.Subject.ID)
	require.Equal(t, "Discord User 1", record.DisplayName)
}
