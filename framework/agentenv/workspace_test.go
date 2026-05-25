package agentenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateConfigMissingFields(t *testing.T) {
	require.Error(t, validateConfig(WorkspaceConfig{}))
	require.Error(t, validateConfig(WorkspaceConfig{Workspace: "w"}))
	require.Error(t, validateConfig(WorkspaceConfig{Workspace: "w", ManifestPath: "m"}))
	require.NoError(t, validateConfig(WorkspaceConfig{Workspace: "w", ManifestPath: "m", InferenceEndpoint: "endpoint"}))
}

func TestSetupTelemetryDefaultsToStateDir(t *testing.T) {
	dir := t.TempDir()
	expected := filepath.Join(dir, ".relurpify_state", "logs", "agentenv.log")
	cfg := WorkspaceConfig{
		Workspace:         dir,
		ManifestPath:      filepath.Join(dir, "relurpify_cfg", "agent.yaml"),
		InferenceEndpoint: "http://localhost:11434",
		StateDir:          filepath.Join(dir, ".relurpify_state"),
	}
	logFile, _, _, err := setupTelemetry(cfg)
	require.NoError(t, err)
	require.NoError(t, logFile.Close())
	_, err = os.Stat(expected)
	require.NoError(t, err)
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
