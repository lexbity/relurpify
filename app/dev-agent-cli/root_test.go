package main

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"github.com/stretchr/testify/require"
)

func TestNewRootCmdUsesDevAgentName(t *testing.T) {
	root := NewRootCmd()
	if got := root.Use; got != "dev-agent" {
		t.Fatalf("root.Use = %q", got)
	}
	if got := root.Short; got != "Development and integration CLI for Relurpify" {
		t.Fatalf("root.Short = %q", got)
	}
	require.NotNil(t, root.PersistentFlags().Lookup("workspace"))
	require.NotNil(t, root.PersistentFlags().Lookup("config"))
}

func TestNewRootCmdPersistentPreRunLoadsDefaultConfig(t *testing.T) {
	originalWorkspace := workspace
	originalCfgFile := cfgFile
	originalWorkspaceCfg := workspaceCfg
	t.Cleanup(func() {
		workspace = originalWorkspace
		cfgFile = originalCfgFile
		workspaceCfg = originalWorkspaceCfg
	})

	root := NewRootCmd()
	workspace = t.TempDir()
	configPath := cfgload.DefaultWorkspaceConfigPath(workspace)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	yamlContent := `schema: relurpify/workspace/v1
model:
  default_name: test-model
sandbox:
  backend: gvisor
`
	require.NoError(t, os.WriteFile(configPath, []byte(yamlContent), 0o644))

	cfgFile = ""
	workspaceCfg = nil

	require.NoError(t, root.PersistentPreRunE(root, nil))
	require.Equal(t, cfgload.DefaultWorkspaceConfigPath(workspace), cfgFile)
	require.NotNil(t, workspaceCfg)
	require.Equal(t, "test-model", *workspaceCfg.Model.DefaultName)
}

func TestNewRootCmdPersistentPreRunLoadsExplicitConfig(t *testing.T) {
	originalWorkspace := workspace
	originalCfgFile := cfgFile
	originalWorkspaceCfg := workspaceCfg
	t.Cleanup(func() {
		workspace = originalWorkspace
		cfgFile = originalCfgFile
		workspaceCfg = originalWorkspaceCfg
	})

	root := NewRootCmd()
	workspace = t.TempDir()
	explicitPath := filepath.Join(workspace, "custom-workspace.yaml")
	yamlContent := `schema: relurpify/workspace/v1
model:
  default_name: custom-model
sandbox:
  backend: gvisor
`
	require.NoError(t, os.WriteFile(explicitPath, []byte(yamlContent), 0o644))
	workspaceCfg = nil

	cfgFile = explicitPath
	require.NoError(t, root.PersistentPreRunE(root, nil))
	require.NotNil(t, workspaceCfg)
	require.Equal(t, "custom-model", *workspaceCfg.Model.DefaultName)
}
