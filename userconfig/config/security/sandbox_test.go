package security

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestLoadSandboxPolicyInjectsProtectedRoot(t *testing.T) {
	workspace := t.TempDir()
	path := SandboxPolicyPath(workspace)
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/policy/sandbox/v1
read_only_root: false
protected_paths:
  - /tmp/custom
no_new_privileges: true
allowed_env_keys: []
denied_env_keys: []
network_rules: []
`)))

	policy, err := LoadSandboxPolicy(path, workspace, testDecode)
	require.NoError(t, err)
	require.Contains(t, policy.ProtectedPaths, filepath.Clean(filepath.Join(workspace, "relurpify_cfg")))
	require.Contains(t, policy.ProtectedPaths, filepath.Clean("/tmp/custom"))
}

func TestLoadSandboxPolicyRejectsMissingFile(t *testing.T) {
	_, err := LoadSandboxPolicy(SandboxPolicyPath(t.TempDir()), t.TempDir(), testDecode)
	require.Error(t, err)
}
