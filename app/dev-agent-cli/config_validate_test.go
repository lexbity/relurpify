package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateConfigTreeAcceptsRepositoryWorkspace(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	report := validateConfigTree(repoRoot)
	require.False(t, report.HasErrors(), report.Error())
}

func TestValidateConfigTreeCollectsMultipleIssues(t *testing.T) {
	workspace := t.TempDir()
	mustWrite := func(relPath, body string) {
		path := filepath.Join(workspace, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}

	mustWrite("relurpify_cfg/workspace.yaml", `schema: relurpify/workspace/v1
paths:
  state_dir: /tmp/relurpify
model:
  provider: ""
  name: ""
sandbox:
  backend: invalid
logging:
  level: verbose
  format: yaml
audit:
  retention_days: 0
`)

	report := validateConfigTree(workspace)
	require.True(t, report.HasErrors(), report.Error())
	require.GreaterOrEqual(t, len(report.Issues), 1)

	got := report.Error()
	require.Contains(t, got, "relurpify_cfg/workspace.yaml")
	require.Contains(t, got, "config validation error:")
}

func TestConfigValidateCmdPrintsReport(t *testing.T) {
	workspaceDir := t.TempDir()
	write := func(relPath, body string) {
		path := filepath.Join(workspaceDir, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}

	write("relurpify_cfg/workspace.yaml", `schema: relurpify/workspace/v1
paths:
  state_dir: /tmp/relurpify
model:
  provider: ""
  name: ""
sandbox:
  backend: invalid
logging:
  level: verbose
  format: yaml
audit:
  retention_days: 0
`)

	originalWorkspace := workspace
	t.Cleanup(func() { workspace = originalWorkspace })
	workspace = workspaceDir

	cmd := newConfigValidateCmd()
	var out strings.Builder
	var errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	require.Empty(t, out.String())
	require.Contains(t, errOut.String(), "config validation error:")
	require.Contains(t, errOut.String(), "relurpify_cfg/workspace.yaml")
}
