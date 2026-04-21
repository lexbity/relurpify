package identity

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestResolverResolvesStaticToken(t *testing.T) {
	resolver := NewResolver([]StaticTokenBinding{{
		Token:       "static-token",
		TenantID:    "tenant-1",
		Role:        "agent",
		SubjectKind: core.SubjectKindServiceAccount,
		SubjectID:   "svc-1",
		Scopes:      []string{"nexus:admin"},
	}}, nil, nil)

	principal, err := resolver.ResolvePrincipal(context.Background(), "static-token")
	require.NoError(t, err)
	require.True(t, principal.Authenticated)
	require.Equal(t, "tenant-1", principal.Principal.TenantID)
	require.Equal(t, core.SubjectKindServiceAccount, principal.Principal.Subject.Kind)
	require.Equal(t, "svc-1", principal.Principal.Subject.ID)
	require.Equal(t, "agent", principal.Role)
}

func TestResolverResolvesIssuedTokenByHash(t *testing.T) {
	dir := t.TempDir()
	tokenStore, err := db.NewSQLiteAdminTokenStore(filepath.Join(dir, "admin_tokens.db"))
	require.NoError(t, err)
	defer tokenStore.Close()

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(dir, "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	now := time.Now().UTC()
	require.NoError(t, identityStore.UpsertTenant(context.Background(), core.TenantRecord{ID: "tenant-issued", CreatedAt: now}))
	require.NoError(t, identityStore.UpsertSubject(context.Background(), core.SubjectRecord{TenantID: "tenant-issued", Kind: core.SubjectKindUser, ID: "subject-1", CreatedAt: now}))
	require.NoError(t, tokenStore.CreateToken(context.Background(), core.AdminTokenRecord{
		ID:          "tok-1",
		TenantID:    "tenant-issued",
		SubjectKind: core.SubjectKindUser,
		SubjectID:   "subject-1",
		TokenHash:   hashToken("issued-token"),
		Scopes:      []string{"nexus:admin"},
		IssuedAt:    now,
	}))

	resolver := NewResolver(nil, tokenStore, identityStore)
	principal, err := resolver.ResolvePrincipal(context.Background(), "issued-token")
	require.NoError(t, err)
	require.True(t, principal.Authenticated)
	require.Equal(t, "tenant-issued", principal.Principal.TenantID)
	require.Equal(t, core.SubjectKindUser, principal.Principal.Subject.Kind)
	require.Equal(t, "subject-1", principal.Principal.Subject.ID)
	require.Equal(t, "admin", principal.Role)
}

func TestResolverRejectsMissingToken(t *testing.T) {
	resolver := NewResolver(nil, nil, nil)
	_, err := resolver.ResolvePrincipal(context.Background(), "")
	require.Error(t, err)
	var resErr ResolutionError
	require.ErrorAs(t, err, &resErr)
	require.Equal(t, ResolutionErrorBearerTokenRequired, resErr.Code)
}

func TestResolverRejectsUnknownToken(t *testing.T) {
	resolver := NewResolver(nil, &stubTokenStore{}, nil)
	_, err := resolver.ResolvePrincipal(context.Background(), "missing")
	require.Error(t, err)
	var resErr ResolutionError
	require.ErrorAs(t, err, &resErr)
	require.Equal(t, ResolutionErrorUnknownToken, resErr.Code)
}

func TestResolverRejectsExpiredToken(t *testing.T) {
	store := &stubTokenStore{
		token: &core.AdminTokenRecord{
			ID:          "tok-1",
			TenantID:    "tenant-1",
			SubjectKind: core.SubjectKindUser,
			SubjectID:   "subject-1",
			TokenHash:   hashToken("expired"),
			ExpiresAt:   ptrTime(time.Now().Add(-time.Minute)),
		},
	}
	resolver := NewResolver(nil, store, &stubSubjectStore{})
	_, err := resolver.ResolvePrincipal(context.Background(), "expired")
	var resErr ResolutionError
	require.ErrorAs(t, err, &resErr)
	require.Equal(t, ResolutionErrorTokenExpired, resErr.Code)
}

func TestResolverRejectsRevokedToken(t *testing.T) {
	store := &stubTokenStore{
		token: &core.AdminTokenRecord{
			ID:          "tok-1",
			TenantID:    "tenant-1",
			SubjectKind: core.SubjectKindUser,
			SubjectID:   "subject-1",
			TokenHash:   hashToken("revoked"),
			RevokedAt:   ptrTime(time.Now().UTC()),
		},
	}
	resolver := NewResolver(nil, store, &stubSubjectStore{})
	_, err := resolver.ResolvePrincipal(context.Background(), "revoked")
	var resErr ResolutionError
	require.ErrorAs(t, err, &resErr)
	require.Equal(t, ResolutionErrorTokenRevoked, resErr.Code)
}

func TestResolverRejectsMissingSubjectKind(t *testing.T) {
	store := &stubTokenStore{
		token: &core.AdminTokenRecord{
			ID:        "tok-1",
			TenantID:  "tenant-1",
			SubjectID: "subject-1",
			TokenHash: hashToken("missing-kind"),
		},
	}
	resolver := NewResolver(nil, store, &stubSubjectStore{})
	_, err := resolver.ResolvePrincipal(context.Background(), "missing-kind")
	var resErr ResolutionError
	require.ErrorAs(t, err, &resErr)
	require.Equal(t, ResolutionErrorAmbiguousSubject, resErr.Code)
}

func TestResolverRejectsStaticTokenMissingSubjectKind(t *testing.T) {
	resolver := NewResolver([]StaticTokenBinding{{
		Token:     "static-token",
		TenantID:  "tenant-1",
		Role:      "agent",
		SubjectID: "svc-1",
	}}, nil, nil)
	_, err := resolver.ResolvePrincipal(context.Background(), "static-token")
	var resErr ResolutionError
	require.ErrorAs(t, err, &resErr)
	require.Equal(t, ResolutionErrorAmbiguousSubject, resErr.Code)
}

type stubTokenStore struct {
	token *core.AdminTokenRecord
	err   error
}

func (s *stubTokenStore) GetTokenByHash(context.Context, string) (*core.AdminTokenRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.token, nil
}

type stubSubjectStore struct{}

func (s *stubSubjectStore) GetTenant(context.Context, string) (*core.TenantRecord, error) {
	return &core.TenantRecord{ID: "tenant-1"}, nil
}

func (s *stubSubjectStore) GetSubject(context.Context, string, core.SubjectKind, string) (*core.SubjectRecord, error) {
	return &core.SubjectRecord{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "subject-1"}, nil
}

func ptrTime(t time.Time) *time.Time { return &t }
