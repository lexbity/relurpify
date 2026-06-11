package security

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestLoadLocalToolPolicy(t *testing.T) {
	workspace := t.TempDir()
	path := LocalToolPolicyPath(workspace)
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/policy/localtool/v1
tools:
  git:
    execute: ask
`)))

	policy, err := LoadLocalToolPolicy(path, workspace, testDecode)
	require.NoError(t, err)
	require.Equal(t, "ask", string(policy["git"].Execute))
}

func TestLoadLocalToolPolicyRejectsInvalidExecute(t *testing.T) {
	workspace := t.TempDir()
	path := LocalToolPolicyPath(workspace)
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/policy/localtool/v1
tools:
  git:
    execute: maybe
`)))

	_, err := LoadLocalToolPolicy(path, workspace, testDecode)
	require.Error(t, err)
}
