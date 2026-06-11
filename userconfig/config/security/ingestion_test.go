package security

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestLoadWorkspaceIngestionPolicy(t *testing.T) {
	workspace := t.TempDir()
	path := WorkspaceIngestionPolicyPath(workspace)
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/policy/ingestion/v1
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
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/policy/ingestion/v1
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
