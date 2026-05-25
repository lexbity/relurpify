package authorization

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestPermissionManagerBlocksWorkspaceMetadataAndStateByDefault(t *testing.T) {
	workspace := t.TempDir()
	stateDir := filepath.Join(workspace, ".relurpify_state")
	declared := &contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{
			{Action: contracts.FileSystemRead, Path: filepath.ToSlash(filepath.Join(workspace, "**"))},
			{Action: contracts.FileSystemWrite, Path: filepath.ToSlash(filepath.Join(workspace, "**"))},
		},
	}

	pm, err := NewPermissionManager(workspace, declared, nil, nil)
	require.NoError(t, err)
	pm.SetDefaultPolicy(agentspec.AgentPermissionDeny)
	pm.SetFilesystemGuardRoots(
		[]string{
			filepath.Join(workspace, "relurpify_cfg"),
			filepath.Join(workspace, ".git"),
		},
		[]string{stateDir},
	)

	require.Error(t, pm.CheckFileAccess(context.Background(), "agent", contracts.FileSystemRead, filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")))
	require.Error(t, pm.CheckFileAccess(context.Background(), "agent", contracts.FileSystemRead, filepath.Join(workspace, ".git", "config")))
	require.Error(t, pm.CheckFileAccess(context.Background(), "agent", contracts.FileSystemWrite, filepath.Join(stateDir, "logs", "agent.log")))
}

func TestPermissionManagerAllowsExplicitStateDirDeclaration(t *testing.T) {
	workspace := t.TempDir()
	stateDir := filepath.Join(workspace, ".relurpify_state")
	declared := &contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{
			{Action: contracts.FileSystemWrite, Path: filepath.ToSlash(filepath.Join(stateDir, "**"))},
		},
	}

	pm, err := NewPermissionManager(workspace, declared, nil, nil)
	require.NoError(t, err)
	pm.SetDefaultPolicy(agentspec.AgentPermissionDeny)
	pm.SetFilesystemGuardRoots(
		[]string{
			filepath.Join(workspace, "relurpify_cfg"),
			filepath.Join(workspace, ".git"),
		},
		[]string{stateDir},
	)

	require.NoError(t, pm.CheckFileAccess(context.Background(), "agent", contracts.FileSystemWrite, filepath.Join(stateDir, "logs", "agent.log")))
}
