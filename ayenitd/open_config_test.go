package ayenitd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveWorkspaceConfigOverridesLoadsSandboxBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`provider: ollama
model: test-model
sandbox_backend: docker
agent: coding
`), 0o644))

	cfg := resolveWorkspaceConfigOverrides(WorkspaceConfig{
		ConfigPath: path,
	})

	require.Equal(t, "docker", cfg.SandboxBackend)
	require.Equal(t, "test-model", cfg.InferenceModel)
	require.Equal(t, "coding", cfg.AgentName)
}

func TestResolveWorkspaceConfigOverridesUsesDefaultModelAndAgents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`default_model:
  name: fallback-model
agents:
  - planner
`), 0o644))

	cfg := resolveWorkspaceConfigOverrides(WorkspaceConfig{
		ConfigPath: path,
	})

	require.Equal(t, "fallback-model", cfg.InferenceModel)
	require.Equal(t, "planner", cfg.AgentName)
}

func TestResolveWorkspaceConfigOverridesPreservesExplicitValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`provider: ignored
model: ignored-model
sandbox_backend: ignored-backend
agent: ignored-agent
`), 0o644))

	cfg := resolveWorkspaceConfigOverrides(WorkspaceConfig{
		ConfigPath:        path,
		InferenceProvider: "explicit-provider",
		InferenceModel:    "explicit-model",
		SandboxBackend:    "explicit-backend",
		AgentName:         "explicit-agent",
	})

	require.Equal(t, "explicit-provider", cfg.InferenceProvider)
	require.Equal(t, "explicit-model", cfg.InferenceModel)
	require.Equal(t, "explicit-backend", cfg.SandboxBackend)
	require.Equal(t, "explicit-agent", cfg.AgentName)
}

func TestValidateConfigMissingFields(t *testing.T) {
	require.Error(t, validateConfig(WorkspaceConfig{}))
	require.Error(t, validateConfig(WorkspaceConfig{Workspace: "w"}))
	require.Error(t, validateConfig(WorkspaceConfig{Workspace: "w", ManifestPath: "m"}))
	require.NoError(t, validateConfig(WorkspaceConfig{Workspace: "w", ManifestPath: "m", InferenceEndpoint: "endpoint"}))
}

func TestSetupTelemetryRejectsInvalidLogDir(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("file"), 0o644))
	_, _, _, err := setupTelemetry(WorkspaceConfig{
		Workspace:         dir,
		ManifestPath:      filepath.Join(dir, "agent.manifest.yaml"),
		InferenceEndpoint: "http://localhost:11434",
		LogPath:           filepath.Join(blocked, "ayenitd.log"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "create log directory")
}
