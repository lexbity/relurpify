package cfgload

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadConfigFileWithinWorkspace(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("schema: relurpify/workspace/v1\n"), 0o644))

	data, err := ReadConfigFile(workspace, path)
	require.NoError(t, err)
	require.Contains(t, string(data), "relurpify/workspace/v1")
}

func TestReadConfigFileRejectsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "workspace.yaml")

	_, err := ReadConfigFile(workspace, outside)
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside workspace root")
}

func TestReadConfigFileRejectsStateDir(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".relurpify_state", "events.db")

	_, err := ReadConfigFile(workspace, path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "runtime state dir")
}
