package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadSandboxPolicyInjectsProtectedRoot(t *testing.T) {
	workspace := t.TempDir()
	path := SandboxPolicyPath(workspace)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/policy/sandbox/v1
read_only_root: false
protected_paths:
  - /tmp/custom
no_new_privileges: true
allowed_env_keys: []
denied_env_keys: []
network_rules: []
`), 0o644))

	policy, err := LoadSandboxPolicy(path, workspace)
	require.NoError(t, err)
	require.Contains(t, policy.ProtectedPaths, filepath.Clean(filepath.Join(workspace, "relurpify_cfg")))
	require.Contains(t, policy.ProtectedPaths, filepath.Clean("/tmp/custom"))
}

func TestLoadSandboxPolicyRejectsMissingFile(t *testing.T) {
	_, err := LoadSandboxPolicy(SandboxPolicyPath(t.TempDir()), t.TempDir())
	require.Error(t, err)
}
