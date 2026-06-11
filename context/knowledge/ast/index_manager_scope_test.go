package ast

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestIndexManagerRespectsFileScope(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "relurpify_cfg", "agent.yaml")
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(protected)))
	require.NoError(t, fs.WriteFileSecure(protected, []byte("secret")))

	store, err := NewTestStore(filepath.Join(workspace, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: workspace})
	manager.SetFileScope(sandbox.NewFileScopePolicy(workspace, []string{protected}))

	require.NoError(t, manager.IndexFile(context.Background(), protected))

	file, err := manager.Store().GetFileByPath(protected)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Nil(t, file)
}
