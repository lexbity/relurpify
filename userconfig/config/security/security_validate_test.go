package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadBundleCollectsAllErrors(t *testing.T) {
	workspace := t.TempDir()
	secDir := filepath.Join(workspace, "relurpify_cfg", "security")
	require.NoError(t, os.MkdirAll(secDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(secDir, "sandbox.policy.yaml"), []byte(`schema: relurpify/policy/sandbox/v1
read_only_root: false
protected_paths:
  - /tmp/custom
no_new_privileges: true
allowed_env_keys: []
denied_env_keys: []
network_rules:
  - host: ""
    port: 0
    protocol: ""
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(secDir, "shell.policy.yaml"), []byte(`schema: relurpify/policy/shell/v1
rules:
  - id: bad
    pattern: "("
    reason: broken
    action: block
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(secDir, "localtool.policy.yaml"), []byte(`schema: relurpify/policy/localtool/v1
tools:
  cli_git:
    execute: maybe
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(secDir, "workspaceingestion.policy.yaml"), []byte(`schema: relurpify/policy/ingestion/v1
rules:
  - id: ""
    name: broken
    priority: 1
    enabled: true
    effect:
      action: allow
      reason: broken
`), 0o644))

	bundle, err := LoadBundle(workspace, testDecode)
	require.Error(t, err)
	require.NotNil(t, bundle)
	errText := err.Error()
	for _, want := range []string{
		"load sandbox policy",
		"load shell policy",
		"load local tool policy",
		"load workspace ingestion policy",
	} {
		require.True(t, strings.Contains(errText, want), "missing %s in %q", want, errText)
	}
}
