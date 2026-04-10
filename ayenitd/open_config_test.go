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
