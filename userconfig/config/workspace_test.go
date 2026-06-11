package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestLoadWorkspaceConfigAppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify_cfg", "workspace.yaml")
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/workspace/v1
model:
  provider: ollama
  name: gemma4:e4b
sandbox:
  backend: gvisor
`)))

	cfg, err := LoadWorkspaceConfig(path, dir, WorkspaceLoadOptions{})
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(dir), filepath.Clean(cfg.WorkspaceAbs))
	require.Equal(t, filepath.Join(dir, ".relurpify_state"), cfg.StateDir())
	require.Equal(t, "ollama", cfg.Model.Provider)
	require.Equal(t, "gemma4:e4b", cfg.Model.Name)
	require.Equal(t, "gvisor", stringValue(cfg.Sandbox.Backend))
	require.Equal(t, "info", stringValue(cfg.Logging.Level))
	require.Equal(t, "json", stringValue(cfg.Logging.Format))
	require.Equal(t, 7, *cfg.Audit.RetentionDays)
	require.False(t, *cfg.Telemetry.Enabled)
	require.NotEmpty(t, cfg.DefaultsUsed)
}

func TestLoadWorkspaceConfigRejectsStrictDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify_cfg", "workspace.yaml")
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, fs.WriteFileSecure(path, []byte(`schema: relurpify/workspace/v1
model:
  provider: ollama
  name: gemma4:e4b
sandbox:
  backend: gvisor
`)))

	_, err := LoadWorkspaceConfig(path, dir, WorkspaceLoadOptions{Strict: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "strict mode rejects defaulted workspace values")
}

func TestLoadWorkspaceConfigRejectsMissingFile(t *testing.T) {
	_, err := LoadWorkspaceConfig(filepath.Join(t.TempDir(), "missing.yaml"), t.TempDir(), WorkspaceLoadOptions{})
	require.Error(t, err)
}
