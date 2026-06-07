package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadToolManifestsLoadsWorkspaceCorpus(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	manifests, err := LoadToolManifests(DefaultToolManifestDir(repoRoot))
	require.NoError(t, err)
	require.NotEmpty(t, manifests)
	require.GreaterOrEqual(t, len(manifests), 20)

	seen := make(map[string]struct{}, len(manifests))
	for _, manifest := range manifests {
		require.NotNil(t, manifest)
		require.NotEmpty(t, manifest.Name)
		require.NotEmpty(t, manifest.SourcePath)
		seen[manifest.Name] = struct{}{}
	}
	require.Contains(t, seen, "file_read")
	require.Contains(t, seen, "go_test")
	require.Contains(t, seen, "search_grep")
	require.Contains(t, seen, "shell_tool_discover")
}

func TestLoadToolManifestRejectsUnknownSchemaBody(t *testing.T) {
	_, err := LoadToolManifest(filepath.Join(t.TempDir(), "missing.tool.yaml"))
	require.Error(t, err)
}
