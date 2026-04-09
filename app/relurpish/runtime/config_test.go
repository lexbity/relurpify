package runtime

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManifest_NoProvider_ResolvesToOllama(t *testing.T) {
	cfg := Config{
		Workspace:    t.TempDir(),
		ManifestPath: filepath.Join(t.TempDir(), "agent.manifest.yaml"),
	}
	require.NoError(t, cfg.Normalize())
	require.Equal(t, "ollama", cfg.InferenceProvider)
	require.Equal(t, "http://localhost:11434", cfg.InferenceEndpoint)
}
