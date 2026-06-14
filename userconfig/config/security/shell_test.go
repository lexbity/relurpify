package security

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestLoadShellPolicy(t *testing.T) {
	workspace := t.TempDir()
	path := ShellPolicyPath(workspace)
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/policy/shell/v1
rules:
  - id: deny-git-reset-hard
    pattern: '(^|\s)git\s+reset\s+--hard(\s|$)'
    reason: "Destructive git reset is blocked"
    action: block
`)))

	blacklist, err := LoadShellPolicy(path, workspace, testDecode)
	require.NoError(t, err)
	require.NotNil(t, blacklist)
	require.Len(t, blacklist.Rules, 1)
	require.Equal(t, "deny-git-reset-hard", blacklist.Rules[0].ID)
	require.Equal(t, "block", blacklist.Rules[0].Action)
}

func TestLoadShellPolicyRejectsInvalidRegex(t *testing.T) {
	workspace := t.TempDir()
	path := ShellPolicyPath(workspace)
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/policy/shell/v1
rules:
  - id: invalid
    pattern: '('
    reason: bad
    action: block
`)))

	_, err := LoadShellPolicy(path, workspace, testDecode)
	require.Error(t, err)
}

func TestLoadShellPolicyRejectsInvalidAction(t *testing.T) {
	workspace := t.TempDir()
	path := ShellPolicyPath(workspace)
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/policy/shell/v1
rules:
  - id: invalid
    pattern: '.*'
    reason: bad
    action: maybe
`)))

	_, err := LoadShellPolicy(path, workspace, testDecode)
	require.Error(t, err)
}
