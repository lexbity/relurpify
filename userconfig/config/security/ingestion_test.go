package security

import (
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/securefile"
	"github.com/stretchr/testify/require"
)

func TestLoadWorkspaceIngestionPolicy(t *testing.T) {
	workspace := t.TempDir()
	path := WorkspaceIngestionPolicyPath(workspace)
	require.NoError(t, securefile.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, securefile.WriteFileSecure(path, []byte(`schema: relurpify/policy/ingestion/v1
rules:
  - id: allow-workspace-ingestion
    name: Workspace ingestion
    priority: 100
    enabled: true
    effect:
      action: allow
      reason: Allow workspace ingestion for configured sources
`)))

	rules, err := LoadWorkspaceIngestionPolicy(path, workspace, testDecode)
	require.NoError(t, err)
	require.Len(t, rules, 1)
}

func TestLoadWorkspaceIngestionPolicyRejectsInvalidRule(t *testing.T) {
	workspace := t.TempDir()
	path := WorkspaceIngestionPolicyPath(workspace)
	require.NoError(t, securefile.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, securefile.WriteFileSecure(path, []byte(`schema: relurpify/policy/ingestion/v1
rules:
  - id: ""
    name: broken
    priority: 1
    enabled: true
    effect:
      action: allow
      reason: broken
`)))

	_, err := LoadWorkspaceIngestionPolicy(path, workspace, testDecode)
	require.Error(t, err)
}
