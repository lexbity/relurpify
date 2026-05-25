package contracts

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewFileScopePolicyProtectsWorkspaceMetadata(t *testing.T) {
	workspace := t.TempDir()
	policy := NewFileScopePolicy(workspace, nil)

	require.Contains(t, policy.ProtectedPaths, filepath.ToSlash(filepath.Join(workspace, "relurpify_cfg")))
	require.Contains(t, policy.ProtectedPaths, filepath.ToSlash(filepath.Join(workspace, ".git")))
	require.ErrorIs(t, policy.Check(FileSystemRead, filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")), ErrFileScopeProtectedPath)
	require.ErrorIs(t, policy.Check(FileSystemRead, filepath.Join(workspace, ".git", "config")), ErrFileScopeProtectedPath)
	require.NoError(t, policy.Check(FileSystemRead, filepath.Join(workspace, "src", "main.go")))
}
