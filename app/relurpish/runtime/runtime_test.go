package runtime

import (
	"context"
	"os"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/workspacecfg"
)

// TestProbeEnvironmentHandlesMissingRunsc surfaces a helpful error message.
func TestProbeEnvironmentHandlesMissingRunsc(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Workspace = dir
	cfg.ManifestPath = filepath.Join(dir, "agent.manifest.yaml")
	cfg.ConfigPath = workspacecfg.New(dir).ConfigFile()
	cfg.Sandbox.RunscPath = "runsc-missing"
	report := ProbeEnvironment(context.Background(), cfg)
	require.Contains(t, strings.Join(report.Sandbox.Errors, " "), "runsc not found")
}

func TestSummarizeManifestMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.manifest.yaml")
	summary := summarizeManifest(path)
	require.Equal(t, path, summary.Path)
	require.False(t, summary.Exists)
	require.Empty(t, summary.Error)
}

func TestInitializeWorkspaceFromTemplatesCreatesWorkspaceFiles(t *testing.T) {
	shared := t.TempDir()
	t.Setenv("RELURPIFY_SHARED_DIR", shared)
	configTemplate := filepath.Join(shared, "templates", "workspace", "config.yaml")
	manifestTemplate := filepath.Join(shared, "templates", "workspace", "agent.manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configTemplate), 0o755))
	require.NoError(t, os.WriteFile(configTemplate, []byte("model: test-model\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestTemplate, []byte("path: ${workspace}\n"), 0o644))

	dir := t.TempDir()
	cfg := Config{Workspace: dir}
	require.NoError(t, cfg.Normalize())

	require.NoError(t, InitializeWorkspaceFromTemplates(cfg, false))
	data, err := os.ReadFile(cfg.ConfigPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "test-model")
	manifestData, err := os.ReadFile(cfg.ManifestPath)
	require.NoError(t, err)
	require.Contains(t, string(manifestData), filepath.ToSlash(dir))
	for _, path := range []string{
		workspacecfg.New(dir).AgentsDir(),
		workspacecfg.New(dir).SkillsDir(),
		workspacecfg.New(dir).LogsDir(),
		workspacecfg.New(dir).TelemetryDir(),
		workspacecfg.New(dir).MemoryDir(),
		workspacecfg.New(dir).SessionsDir(),
		workspacecfg.New(dir).TestRunsDir(),
	} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, info.IsDir())
	}
}

func TestInitializeWorkspaceFromTemplatesDoesNotOverwriteWithoutFix(t *testing.T) {
	shared := t.TempDir()
	t.Setenv("RELURPIFY_SHARED_DIR", shared)
	configTemplate := filepath.Join(shared, "templates", "workspace", "config.yaml")
	manifestTemplate := filepath.Join(shared, "templates", "workspace", "agent.manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configTemplate), 0o755))
	require.NoError(t, os.WriteFile(configTemplate, []byte("model: replacement\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestTemplate, []byte("name: replacement\n"), 0o644))

	dir := t.TempDir()
	cfg := Config{Workspace: dir}
	require.NoError(t, cfg.Normalize())
	require.NoError(t, os.MkdirAll(filepath.Dir(cfg.ConfigPath), 0o755))
	require.NoError(t, os.WriteFile(cfg.ConfigPath, []byte("model: keep\n"), 0o644))
	require.NoError(t, os.WriteFile(cfg.ManifestPath, []byte("name: keep\n"), 0o644))

	require.NoError(t, InitializeWorkspaceFromTemplates(cfg, false))
	configData, err := os.ReadFile(cfg.ConfigPath)
	require.NoError(t, err)
	require.Contains(t, string(configData), "keep")
	manifestData, err := os.ReadFile(cfg.ManifestPath)
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "keep")
}

func TestDetectChromiumStatusMissingIsWarningOnly(t *testing.T) {
	orig := execLookPath
	execLookPath = func(file string) (string, error) {
		return "", os.ErrNotExist
	}
	defer func() { execLookPath = orig }()

	status := detectChromiumStatus(context.Background())
	require.Equal(t, "chromium", status.Name)
	require.False(t, status.Required)
	require.False(t, status.Available)
	require.False(t, status.Blocking)
	require.Equal(t, "not found", status.Details)
}
