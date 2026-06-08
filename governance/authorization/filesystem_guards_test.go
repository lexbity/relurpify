package authorization

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

func TestPermissionManagerBlocksWorkspaceMetadataAndStateByDefault(t *testing.T) {
	workspace := t.TempDir()
	stateDir := filepath.Join(workspace, ".relurpify_state")
	declared := &permissions.PermissionSet{
		FileSystem: []permissions.FileSystemPermission{
			{Action: permissions.FileSystemRead, Path: filepath.ToSlash(filepath.Join(workspace, "**"))},
			{Action: permissions.FileSystemWrite, Path: filepath.ToSlash(filepath.Join(workspace, "**"))},
		},
	}

	pm, err := NewPermissionManager(workspace, declared, nil, nil)
	require.NoError(t, err)
	pm.SetDefaultPolicy("deny")
	pm.SetFilesystemGuardRoots(
		[]string{
			filepath.Join(workspace, "relurpify_cfg"),
			filepath.Join(workspace, ".git"),
		},
		[]string{stateDir},
	)

	require.Error(t, pm.CheckFileAccess(context.Background(), "agent", permissions.FileSystemRead, filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")))
	require.Error(t, pm.CheckFileAccess(context.Background(), "agent", permissions.FileSystemRead, filepath.Join(workspace, ".git", "config")))
	require.Error(t, pm.CheckFileAccess(context.Background(), "agent", permissions.FileSystemWrite, filepath.Join(stateDir, "logs", "agent.log")))
}

func TestPermissionManagerAllowsExplicitStateDirDeclaration(t *testing.T) {
	workspace := t.TempDir()
	stateDir := filepath.Join(workspace, ".relurpify_state")
	declared := &permissions.PermissionSet{
		FileSystem: []permissions.FileSystemPermission{
			{Action: permissions.FileSystemWrite, Path: filepath.ToSlash(filepath.Join(stateDir, "**"))},
		},
	}

	pm, err := NewPermissionManager(workspace, declared, nil, nil)
	require.NoError(t, err)
	pm.SetDefaultPolicy("deny")
	pm.SetFilesystemGuardRoots(
		[]string{
			filepath.Join(workspace, "relurpify_cfg"),
			filepath.Join(workspace, ".git"),
		},
		[]string{stateDir},
	)

	require.NoError(t, pm.CheckFileAccess(context.Background(), "agent", permissions.FileSystemWrite, filepath.Join(stateDir, "logs", "agent.log")))
}
