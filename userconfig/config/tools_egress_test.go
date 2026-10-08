package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoadToolManifestDecodesEgressAllowlists proves the additive
// allow_private_hosts field decodes under the strict schema decoder.
func TestLoadToolManifestDecodesEgressAllowlists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "egress.tool.yaml")
	content := `schema: relurpify/tool/v1
name: cli_egress
version: "1"
family: network
description: egress tool
execution:
  backend: subprocess
  command:
    base: [curl]
  sandbox:
    network_access: true
    allow_hosts: [api.example.com]
    allow_private_hosts: [internal.example]
capability:
  trust_class: builtin_trusted
  risk_class: [execute, network]
  effect_class: [process_spawn, network_egress, external_state]
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644)) //nolint:gosec // test fixture

	m, err := LoadToolManifest(path)
	require.NoError(t, err)
	require.NotNil(t, m.Execution.Sandbox)
	require.Equal(t, []string{"api.example.com"}, m.Execution.Sandbox.AllowHosts)
	require.Equal(t, []string{"internal.example"}, m.Execution.Sandbox.AllowPrivateHosts)
}
