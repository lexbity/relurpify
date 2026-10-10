package authorization

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
	"github.com/stretchr/testify/require"
)

func TestPermissionManagerBlocksWorkspaceMetadataAndStateByDefault(t *testing.T) {
	workspace := t.TempDir()
	stateDir := filepath.Join(workspace, ".relurpify_state")
	declared := &ucperms.PermissionSet{
		FileSystem: []ucperms.FileSystemPermission{
			{Action: ucperms.FileSystemRead, Path: filepath.ToSlash(filepath.Join(workspace, "**"))},
			{Action: ucperms.FileSystemWrite, Path: filepath.ToSlash(filepath.Join(workspace, "**"))},
		},
	}

	pm, err := NewPermissionManager(workspace, declared, nil, nil)
	require.NoError(t, err)
	_ = pm.SetDefaultDecision(permissions.DecisionDeny)
	pm.SetFilesystemGuardRoots(
		[]string{
			filepath.Join(workspace, "relurpify_cfg"),
			filepath.Join(workspace, ".git"),
		},
		[]string{stateDir},
	)

	require.Error(t, pm.CheckFileAccess(context.Background(), "agent", ucperms.FileSystemRead, filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")))
	require.Error(t, pm.CheckFileAccess(context.Background(), "agent", ucperms.FileSystemRead, filepath.Join(workspace, ".git", "config")))
	require.Error(t, pm.CheckFileAccess(context.Background(), "agent", ucperms.FileSystemWrite, filepath.Join(stateDir, "logs", "agent.log")))
}

func TestPermissionManagerAllowsExplicitStateDirDeclaration(t *testing.T) {
	workspace := t.TempDir()
	stateDir := filepath.Join(workspace, ".relurpify_state")
	declared := &ucperms.PermissionSet{
		FileSystem: []ucperms.FileSystemPermission{
			{Action: ucperms.FileSystemWrite, Path: filepath.ToSlash(filepath.Join(stateDir, "**"))},
		},
	}

	pm, err := NewPermissionManager(workspace, declared, nil, nil)
	require.NoError(t, err)
	_ = pm.SetDefaultDecision(permissions.DecisionDeny)
	pm.SetFilesystemGuardRoots(
		[]string{
			filepath.Join(workspace, "relurpify_cfg"),
			filepath.Join(workspace, ".git"),
		},
		[]string{stateDir},
	)

	require.NoError(t, pm.CheckFileAccess(context.Background(), "agent", ucperms.FileSystemWrite, filepath.Join(stateDir, "logs", "agent.log")))
}
