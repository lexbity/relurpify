package main

import (
	"path/filepath"
	"testing"

	frameworkconfig "codeburg.org/lexbit/relurpify/framework/config"
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
	originalGlobalCfg := globalCfg
	t.Cleanup(func() {
		workspace = originalWorkspace
		cfgFile = originalCfgFile
		globalCfg = originalGlobalCfg
	})

	workspace = t.TempDir()
	cfgFile = ""
	globalCfg = nil

	root := NewRootCmd()
	require.NoError(t, root.PersistentPreRunE(root, nil))
	require.Equal(t, frameworkconfig.DefaultConfigPath(workspace), cfgFile)
	require.NotNil(t, globalCfg)
	require.Equal(t, "1.0.0", globalCfg.Version)
	require.Equal(t, frameworkconfig.DefaultAgentPaths(workspace), globalCfg.AgentPaths)
}

func TestNewRootCmdPersistentPreRunLoadsExplicitConfig(t *testing.T) {
	originalWorkspace := workspace
	originalCfgFile := cfgFile
	originalGlobalCfg := globalCfg
	t.Cleanup(func() {
		workspace = originalWorkspace
		cfgFile = originalCfgFile
		globalCfg = originalGlobalCfg
	})

	workspace = t.TempDir()
	explicitPath := filepath.Join(workspace, "custom-config.yaml")
	if err := frameworkconfig.SaveGlobalConfig(explicitPath, &frameworkconfig.GlobalConfig{
		Version:    "2.0.0",
		AgentPaths: []string{"./custom-agents"},
	}); err != nil {
		t.Fatal(err)
	}
	globalCfg = nil

	root := NewRootCmd()
	cfgFile = explicitPath
	require.NoError(t, root.PersistentPreRunE(root, nil))
	require.NotNil(t, globalCfg)
	require.Equal(t, "2.0.0", globalCfg.Version)
	require.Equal(t, []string{"./custom-agents"}, globalCfg.AgentPaths)
}
