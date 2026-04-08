package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	nexusdb "github.com/lexcodex/relurpify/app/nexus/db"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestLoadOrCreateFMPSignerAndPrincipalRole(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "seed")

	signer, err := loadOrCreateFMPSigner(seedPath)
	require.NoError(t, err)
	require.NotNil(t, signer)
	require.FileExists(t, seedPath)

	seedText, err := os.ReadFile(seedPath)
	require.NoError(t, err)
	rawSeed, err := base64.RawStdEncoding.DecodeString(string(seedText))
	require.NoError(t, err)
	require.Len(t, rawSeed, 32)

	loaded, err := loadOrCreateFMPSigner(seedPath)
	require.NoError(t, err)
	require.Equal(t, signer.PublicKey(), loaded.PublicKey())

	badSeedPath := filepath.Join(dir, "bad-seed")
	require.NoError(t, os.WriteFile(badSeedPath, []byte("not-base64"), 0o600))
	_, err = loadOrCreateFMPSigner(badSeedPath)
	require.Error(t, err)

	require.Equal(t, "admin", principalRole([]string{"node", "admin"}))
	require.Equal(t, "operator", principalRole([]string{"node", "operator"}))
	require.Equal(t, "node", principalRole([]string{"node"}))
	require.Equal(t, "agent", principalRole(nil))
}

func TestDynamicTokenPrincipalBranches(t *testing.T) {
	ctx := context.Background()

	_, err := dynamicTokenPrincipal(ctx, core.AdminTokenRecord{ID: "tok-1"}, nil)
	require.ErrorContains(t, err, "missing subject id")

	identityStore, err := nexusdb.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	require.NoError(t, identityStore.UpsertTenant(ctx, core.TenantRecord{
		ID:         "tenant-1",
		CreatedAt:  time.Now().UTC(),
		DisabledAt: func() *time.Time { ts := time.Now().UTC(); return &ts }(),
	}))
	_, err = dynamicTokenPrincipal(ctx, core.AdminTokenRecord{
		ID:        "tok-2",
		TenantID:  "tenant-1",
		SubjectID: "subject-1",
	}, identityStore)
	require.ErrorContains(t, err, "disabled")

	require.NoError(t, identityStore.UpsertTenant(ctx, core.TenantRecord{
		ID:        "tenant-2",
		CreatedAt: time.Now().UTC(),
	}))
	_, err = dynamicTokenPrincipal(ctx, core.AdminTokenRecord{
		ID:          "tok-3",
		TenantID:    "tenant-2",
		SubjectKind: core.SubjectKindUser,
		SubjectID:   "subject-1",
	}, identityStore)
	require.ErrorContains(t, err, "not found")

	require.NoError(t, identityStore.UpsertSubject(ctx, core.SubjectRecord{
		TenantID:   "tenant-2",
		Kind:       core.SubjectKindUser,
		ID:         "subject-1",
		CreatedAt:  time.Now().UTC(),
		DisabledAt: func() *time.Time { ts := time.Now().UTC(); return &ts }(),
	}))
	_, err = dynamicTokenPrincipal(ctx, core.AdminTokenRecord{
		ID:          "tok-4",
		TenantID:    "tenant-2",
		SubjectKind: core.SubjectKindUser,
		SubjectID:   "subject-1",
	}, identityStore)
	require.ErrorContains(t, err, "disabled")

	require.NoError(t, identityStore.UpsertSubject(ctx, core.SubjectRecord{
		TenantID:  "tenant-2",
		Kind:      core.SubjectKindUser,
		ID:        "subject-1",
		CreatedAt: time.Now().UTC(),
	}))
	principal, err := dynamicTokenPrincipal(ctx, core.AdminTokenRecord{
		ID:          "tok-5",
		TenantID:    "tenant-2",
		SubjectKind: core.SubjectKindUser,
		SubjectID:   "subject-1",
		Scopes:      []string{"nexus:admin"},
	}, identityStore)
	require.NoError(t, err)
	require.Equal(t, "tenant-2", principal.TenantID)
	require.Equal(t, core.SubjectKindUser, principal.Subject.Kind)
	require.Equal(t, []string{"nexus:admin"}, principal.Scopes)
}

func TestStdioAdminPrincipalFallback(t *testing.T) {
	cfg := nexuscfg.Config{}
	principal, err := stdioAdminPrincipal(cfg, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, "default", principal.TenantID)
	require.Equal(t, core.AuthMethodBootstrapAdmin, principal.AuthMethod)

	cfg.Gateway.Auth.Enabled = true
	cfg.Gateway.Auth.Tokens = []nexuscfg.GatewayTokenAuth{{
		Token:     "token-1",
		TenantID:  "tenant-1",
		Role:      "admin",
		SubjectID: "svc-1",
		Scopes:    []string{"nexus:admin"},
	}}
	principal, err = stdioAdminPrincipal(cfg, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, "tenant-1", principal.TenantID)
	require.Equal(t, core.SubjectKindServiceAccount, principal.Subject.Kind)
	require.Equal(t, []string{"nexus:admin"}, principal.Scopes)
}
