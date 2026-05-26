package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadShellPolicy(t *testing.T) {
	workspace := t.TempDir()
	path := ShellPolicyPath(workspace)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/policy/shell/v1
rules:
  - id: deny-git-reset-hard
    pattern: '(^|\s)git\s+reset\s+--hard(\s|$)'
    reason: "Destructive git reset is blocked"
    action: block
`), 0o644))

	blacklist, err := LoadShellPolicy(path, workspace, testDecode)
	require.NoError(t, err)
	require.NotNil(t, blacklist)
	require.NotNil(t, blacklist.Check("git reset --hard"))
}

func TestLoadShellPolicyRejectsInvalidRegex(t *testing.T) {
	workspace := t.TempDir()
	path := ShellPolicyPath(workspace)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/policy/shell/v1
rules:
  - id: invalid
    pattern: '('
    reason: bad
    action: block
`), 0o644))

	_, err := LoadShellPolicy(path, workspace, testDecode)
	require.Error(t, err)
}

func TestLoadShellPolicyRejectsInvalidAction(t *testing.T) {
	workspace := t.TempDir()
	path := ShellPolicyPath(workspace)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/policy/shell/v1
rules:
  - id: invalid
    pattern: '.*'
    reason: bad
    action: maybe
`), 0o644))

	_, err := LoadShellPolicy(path, workspace, testDecode)
	require.Error(t, err)
}
