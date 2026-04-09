package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigPathHelpers(t *testing.T) {
	workspace := filepath.Join("/tmp", "workspace")

	require.Equal(t, filepath.Join(workspace, DirName, "config.yaml"), DefaultConfigPath(workspace))
	require.Equal(t, []string{
		filepath.Join(workspace, DirName, "agents"),
		filepath.Join(workspace, DirName, "agent.manifest.yaml"),
	}, DefaultAgentPaths(workspace))
}

func TestLoadGlobalConfigDefaultsWhenMissing(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg, err := LoadGlobalConfig(filepath.Join(t.TempDir(), "missing", "config.yaml"), workspace)
	require.NoError(t, err)
	require.Equal(t, "1.0.0", cfg.Version)
	require.Equal(t, DefaultAgentPaths(workspace), cfg.AgentPaths)
	require.Equal(t, map[string]string{
		"file_write":  "ask",
		"file_edit":   "ask",
		"file_delete": "deny",
	}, cfg.Permissions)
}

func TestLoadGlobalConfigFromFileAndAgentFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify_cfg", "config.yaml")
	data := []byte(`version: "2.0.0"
permissions:
  file_write: allow
features:
  auto_save: true
`)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	cfg, err := LoadGlobalConfig(path, "/workspace")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", cfg.Version)
	require.True(t, cfg.Features.AutoSave)
	require.Equal(t, DefaultAgentPaths("/workspace"), cfg.AgentPaths)
	require.Equal(t, "allow", cfg.Permissions["file_write"])
}

func TestLoadGlobalConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: [broken"), 0o644))

	cfg, err := LoadGlobalConfig(path, "/workspace")
	require.Error(t, err)
	require.Nil(t, cfg)
}

func TestSaveGlobalConfig(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		err := SaveGlobalConfig(filepath.Join(t.TempDir(), "config.yaml"), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "config missing")
	})

	t.Run("writes file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "config.yaml")
		cfg := &GlobalConfig{
			Version: "3.1.4",
			Models: []ModelRef{{
				Name:        "test-model",
				Provider:    "local",
				Temperature: 0.2,
				MaxTokens:   1024,
			}},
			Features: FeatureFlags{AutoSave: true, ParallelAgents: true},
		}

		require.NoError(t, SaveGlobalConfig(path, cfg))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Contains(t, string(data), `version: 3.1.4`)
		require.Contains(t, string(data), `name: test-model`)
		require.Contains(t, string(data), `auto_save: true`)
	})

	t.Run("mkdir failure", func(t *testing.T) {
		dir := t.TempDir()
		parent := filepath.Join(dir, "blocked")
		require.NoError(t, os.WriteFile(parent, []byte("file"), 0o644))

		err := SaveGlobalConfig(filepath.Join(parent, "config.yaml"), &GlobalConfig{Version: "1"})
		require.Error(t, err)
	})
}

func TestAgentSearchPaths(t *testing.T) {
	workspace := "/workspace"

	var nilCfg *GlobalConfig
	require.Equal(t, DefaultAgentPaths(workspace), nilCfg.AgentSearchPaths(workspace))

	require.Equal(t, DefaultAgentPaths(workspace), (&GlobalConfig{}).AgentSearchPaths(workspace))

	cfg := &GlobalConfig{AgentPaths: []string{
		"~/agents/custom.yaml",
		"./local.yaml",
		"/abs/agent.yaml",
	}}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join(home, "/agents/custom.yaml"),
		filepath.Join(workspace, "./local.yaml"),
		"/abs/agent.yaml",
	}, cfg.AgentSearchPaths(workspace))
}

func TestExpandPath(t *testing.T) {
	workspace := "/workspace"
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	require.Empty(t, ExpandPath("", workspace))
	require.Equal(t, filepath.Join(home, ".relurpify"), ExpandPath("~/.relurpify", workspace))
	require.Equal(t, filepath.Join(workspace, "./agent.yaml"), ExpandPath("./agent.yaml", workspace))
	require.Equal(t, "/abs/agent.yaml", ExpandPath("/abs/agent.yaml", workspace))
}
