package security

import (
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/securefile"
	"github.com/stretchr/testify/require"
)

func TestLoadLocalToolPolicy(t *testing.T) {
	workspace := t.TempDir()
	path := LocalToolPolicyPath(workspace)
	require.NoError(t, securefile.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, securefile.WriteFileSecure(path, []byte(`schema: relurpify/policy/localtool/v1
tools:
  git:
    execute: ask
`)))

	policy, err := LoadLocalToolPolicy(path, workspace, testDecode)
	require.NoError(t, err)
	require.Equal(t, "ask", policy["git"].Execute)
}

func TestLoadLocalToolPolicyRejectsInvalidExecute(t *testing.T) {
	workspace := t.TempDir()
	path := LocalToolPolicyPath(workspace)
	require.NoError(t, securefile.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, securefile.WriteFileSecure(path, []byte(`schema: relurpify/policy/localtool/v1
tools:
  git:
    execute: maybe
`)))

	_, err := LoadLocalToolPolicy(path, workspace, testDecode)
	require.Error(t, err)
}
