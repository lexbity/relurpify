package ast

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

func TestIndexManagerRespectsFileScope(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "relurpify_cfg", "agent.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(protected), 0o755))
	require.NoError(t, os.WriteFile(protected, []byte("secret"), 0o644))

	store, err := NewSQLiteStore(filepath.Join(workspace, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: workspace})
	manager.SetFileScope(sandbox.NewFileScopePolicy(workspace, []string{protected}))

	require.NoError(t, manager.IndexFile(protected))

	file, err := manager.Store().GetFileByPath(protected)
	require.Error(t, err)
	require.Nil(t, file)
}
