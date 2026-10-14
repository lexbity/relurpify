package testhelper

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestWriteCleanWorkspaceDefaultsProviderToOffline pins the empty-provider
// default: the fixture must never write an empty provider into workspace.yaml.
func TestWriteCleanWorkspaceDefaultsProviderToOffline(t *testing.T) {
	workspace := t.TempDir()
	WriteCleanWorkspace(t, workspace, WorkspaceOpts{})

	data, err := os.ReadFile(filepath.Join(workspace, "relurpify_cfg", "workspace.yaml"))
	require.NoError(t, err)

	var cfg struct {
		Model struct {
			Provider string `yaml:"provider"`
		} `yaml:"model"`
	}
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	assert.Equal(t, "offline", cfg.Model.Provider)
}

// TestWriteCleanWorkspaceSeedFilesAndRecipes covers the seed-file and recipe
// materialization branches, including nested paths.
func TestWriteCleanWorkspaceSeedFilesAndRecipes(t *testing.T) {
	workspace := t.TempDir()
	WriteCleanWorkspace(t, workspace, WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"src/main.go":        "package main\n",
			"docs/readme.md":     "# docs\n",
			"flat.txt":           "flat",
			"  spaced-name.txt ": "trimmed name",
		},
		Recipes: map[string]string{
			"audit.yaml":        "steps: []\n",
			"nested/extra.yaml": "steps: []\n",
		},
	})

	for _, rel := range []string{
		"src/main.go",
		"docs/readme.md",
		"flat.txt",
		"spaced-name.txt",
		filepath.Join("relurpify_cfg", "euclo", "audit.yaml"),
		filepath.Join("relurpify_cfg", "euclo", "nested", "extra.yaml"),
	} {
		_, err := os.Stat(filepath.Join(workspace, rel))
		require.NoError(t, err, "expected fixture file %s to exist", rel)
	}

	content, err := os.ReadFile(filepath.Join(workspace, "src", "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(content))
}

func TestWriteCleanWorkspaceWorkspaceYAMLRoundTrips(t *testing.T) {
	workspace := t.TempDir()
	WriteCleanWorkspace(t, workspace, WorkspaceOpts{Provider: "ollama"})

	data, err := os.ReadFile(filepath.Join(workspace, "relurpify_cfg", "workspace.yaml"))
	require.NoError(t, err)

	var cfg struct {
		Schema string `yaml:"schema"`
		Paths  struct {
			StateDir string `yaml:"state_dir"`
		} `yaml:"paths"`
		Model struct {
			Provider string `yaml:"provider"`
			Name     string `yaml:"name"`
		} `yaml:"model"`
	}
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	assert.Equal(t, "relurpify/workspace/v1", cfg.Schema)
	assert.Equal(t, ".relurpify_state", cfg.Paths.StateDir)
	assert.Equal(t, "ollama", cfg.Model.Provider)
	assert.Equal(t, "gemma4:e4b", cfg.Model.Name)
}
