package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestAcquireFromFileRespectsFileScope(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "relurpify_cfg", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(protected), 0o755))
	require.NoError(t, os.WriteFile(protected, []byte("secret"), 0o644))

	_, err := AcquireFromFile(context.Background(), protected, identity.SubjectRef{ID: "scanner"}, nil, nil, sandbox.NewFileScopePolicy(workspace, []string{protected}))
	require.Error(t, err)
}

func TestWorkspaceScannerSkipsProtectedConfigDir(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644))
	protectedDir := filepath.Join(workspace, "relurpify_cfg")
	require.NoError(t, os.MkdirAll(protectedDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(protectedDir, "config.yaml"), []byte("secret"), 0o644))

	scanner := &WorkspaceScanner{
		FileScope: sandbox.NewFileScopePolicy(workspace, []string{protectedDir}),
	}

	files, err := scanner.discoverFiles(workspace)
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(workspace, "main.go")}, files)
}

func TestWorkspaceScannerAllowsWorkspaceFilesOutsideConfig(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644))
	scanner := &WorkspaceScanner{}

	files, err := scanner.discoverFiles(workspace)
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(workspace, "main.go")}, files)
}
