package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceConfigBackendRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify.yaml")
	cfg := WorkspaceConfig{
		Model:          "test-model",
		Provider:       "ollama",
		SandboxBackend: "docker",
		Agents:         []string{"coding"},
	}

	require.NoError(t, SaveWorkspaceConfig(path, cfg))

	loaded, err := LoadWorkspaceConfig(path)
	require.NoError(t, err)
	require.Equal(t, "docker", loaded.SandboxBackend)
	require.Equal(t, "test-model", loaded.Model)
	require.Equal(t, "ollama", loaded.Provider)
	require.Equal(t, []string{"coding"}, loaded.Agents)
}

func TestLoadWorkspaceConfigParsesSandboxBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`model: test-model
sandbox_backend: gvisor
agents:
  - coding
`), 0o644))

	loaded, err := LoadWorkspaceConfig(path)
	require.NoError(t, err)
	require.Equal(t, "gvisor", loaded.SandboxBackend)
}
