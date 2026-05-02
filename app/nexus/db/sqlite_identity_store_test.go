package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestSQLiteIdentityStoreExternalIdentityRoundTrip(t *testing.T) {
	store, err := NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	record := identity.ExternalIdentity{
		TenantID:   "tenant-1",
		Provider:   identity.ExternalProviderDiscord,
		AccountID:  "guild-1",
		ExternalID: "user-42",
		Subject: identity.SubjectRef{
			TenantID: "tenant-1",
			Kind:     identity.SubjectKindUser,
			ID:       "internal-user-1",
		},
		VerifiedAt:    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		LastSeenAt:    time.Date(2026, 3, 10, 12, 5, 0, 0, time.UTC),
		DisplayName:   "Lex",
		ProviderLabel: "discord-main",
	}
	require.NoError(t, store.UpsertExternalIdentity(context.Background(), record))

	got, err := store.GetExternalIdentity(context.Background(), "tenant-1", identity.ExternalProviderDiscord, "guild-1", "user-42")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, record.Subject.ID, got.Subject.ID)
	require.Equal(t, record.DisplayName, got.DisplayName)

	list, err := store.ListExternalIdentities(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "user-42", list[0].ExternalID)
}

func TestSQLiteIdentityStoreTenantAndSubjectRoundTrip(t *testing.T) {
	store, err := NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.UpsertTenant(context.Background(), identity.TenantRecord{
		ID:          "tenant-1",
		DisplayName: "Tenant 1",
		CreatedAt:   now,
	}))
	require.NoError(t, store.UpsertSubject(context.Background(), identity.SubjectRecord{
		TenantID:    "tenant-1",
		Kind:        identity.SubjectKindServiceAccount,
		ID:          "svc-1",
		DisplayName: "Service 1",
		Roles:       []string{"operator"},
		CreatedAt:   now,
	}))

	tenant, err := store.GetTenant(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.NotNil(t, tenant)
	require.Equal(t, "Tenant 1", tenant.DisplayName)

	subject, err := store.GetSubject(context.Background(), "tenant-1", identity.SubjectKindServiceAccount, "svc-1")
	require.NoError(t, err)
	require.NotNil(t, subject)
	require.Equal(t, []string{"operator"}, subject.Roles)

	tenants, err := store.ListTenants(context.Background())
	require.NoError(t, err)
	require.Len(t, tenants, 1)

	subjects, err := store.ListSubjects(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, subjects, 1)
}

func TestSQLiteIdentityStoreNodeEnrollmentRoundTrip(t *testing.T) {
	store, err := NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	enrollment := identity.NodeEnrollment{
		TenantID:   "tenant-1",
		NodeID:     "node-1",
		TrustClass: identity.TrustClassWorkspaceTrusted,
		Owner: identity.SubjectRef{
			TenantID: "tenant-1",
			Kind:     identity.SubjectKindNode,
			ID:       "node-1",
		},
		PublicKey:      []byte("public-key"),
		KeyID:          "k1",
		PairedAt:       time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		LastVerifiedAt: time.Date(2026, 3, 10, 11, 30, 0, 0, time.UTC),
		AuthMethod:     identity.AuthMethodNodeChallenge,
	}
	require.NoError(t, store.UpsertNodeEnrollment(context.Background(), enrollment))

	got, err := store.GetNodeEnrollment(context.Background(), "tenant-1", "node-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, enrollment.KeyID, got.KeyID)
	require.Equal(t, enrollment.AuthMethod, got.AuthMethod)

	list, err := store.ListNodeEnrollments(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "node-1", list[0].NodeID)
}
