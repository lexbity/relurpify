package ingestion

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/governance/identity"
	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestAcquireFromFileRespectsFileScope(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "relurpify_cfg", "config.yaml")
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(protected)))
	require.NoError(t, fs.WriteFileSecure(protected, []byte("secret")))

	_, err := AcquireFromFile(context.Background(), protected, identity.SubjectRef{ID: "scanner"}, nil, nil, nil, sandbox.NewFileScopePolicy(workspace, []string{protected}))
	require.Error(t, err)
}

func TestWorkspaceScannerSkipsProtectedConfigDir(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, fs.WriteFileSecure(filepath.Join(workspace, "main.go"), []byte("package main\n")))
	protectedDir := filepath.Join(workspace, "relurpify_cfg")
	require.NoError(t, fs.MkdirAllSecure(protectedDir))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(protectedDir, "config.yaml"), []byte("secret")))

	scanner := &WorkspaceScanner{
		FileScope: sandbox.NewFileScopePolicy(workspace, []string{protectedDir}),
	}

	files, err := scanner.discoverFiles(workspace)
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(workspace, "main.go")}, files)
}

func TestWorkspaceScannerAllowsWorkspaceFilesOutsideConfig(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, fs.WriteFileSecure(filepath.Join(workspace, "main.go"), []byte("package main\n")))
	scanner := &WorkspaceScanner{}

	files, err := scanner.discoverFiles(workspace)
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(workspace, "main.go")}, files)
}
