package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadWorkspaceIngestionPolicy(t *testing.T) {
	workspace := t.TempDir()
	path := WorkspaceIngestionPolicyPath(workspace)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/policy/ingestion/v1
rules:
  - id: allow-workspace-ingestion
    name: Workspace ingestion
    priority: 100
    enabled: true
    effect:
      action: allow
      reason: Allow workspace ingestion for configured sources
`), 0o644))

	rules, err := LoadWorkspaceIngestionPolicy(path, workspace, testDecode)
	require.NoError(t, err)
	require.Len(t, rules, 1)
}

func TestLoadWorkspaceIngestionPolicyRejectsInvalidRule(t *testing.T) {
	workspace := t.TempDir()
	path := WorkspaceIngestionPolicyPath(workspace)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/policy/ingestion/v1
rules:
  - id: ""
    name: broken
    priority: 1
    enabled: true
    effect:
      action: allow
      reason: broken
`), 0o644))

	_, err := LoadWorkspaceIngestionPolicy(path, workspace, testDecode)
	require.Error(t, err)
}
