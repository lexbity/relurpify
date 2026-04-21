package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	"codeburg.org/lexbit/relurpify/framework/core"
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
}

func TestStdioAdminPrincipalFallback(t *testing.T) {
	cfg := nexuscfg.Config{}
	principal, err := stdioAdminPrincipal(cfg, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, "default", principal.TenantID)
	require.Equal(t, core.AuthMethodBootstrapAdmin, principal.AuthMethod)

	cfg.Gateway.Auth.Enabled = true
	cfg.Gateway.Auth.Tokens = []nexuscfg.GatewayTokenAuth{{
		Token:       "token-1",
		TenantID:    "tenant-1",
		Role:        "admin",
		SubjectKind: string(core.SubjectKindServiceAccount),
		SubjectID:   "svc-1",
		Scopes:      []string{"nexus:admin"},
	}}
	principal, err = stdioAdminPrincipal(cfg, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, "tenant-1", principal.TenantID)
	require.Equal(t, core.SubjectKindServiceAccount, principal.Subject.Kind)
	require.Equal(t, []string{"nexus:admin"}, principal.Scopes)
}
