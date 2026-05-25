package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadLocalToolPolicy(t *testing.T) {
	workspace := t.TempDir()
	path := LocalToolPolicyPath(workspace)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/policy/localtool/v1
tools:
  git:
    execute: ask
`), 0o644))

	policy, err := LoadLocalToolPolicy(path, workspace)
	require.NoError(t, err)
	require.Equal(t, "ask", string(policy["git"].Execute))
}

func TestLoadLocalToolPolicyRejectsInvalidExecute(t *testing.T) {
	workspace := t.TempDir()
	path := LocalToolPolicyPath(workspace)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/policy/localtool/v1
tools:
  git:
    execute: maybe
`), 0o644))

	_, err := LoadLocalToolPolicy(path, workspace)
	require.Error(t, err)
}
