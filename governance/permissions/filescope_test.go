package permissions

import (
	"os"
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

func TestCanonicalScopePathSymlinkedParentEscape(t *testing.T) {
	workspace := t.TempDir()

	// Create a symlink inside the workspace that points outside.
	outsideDir := t.TempDir()
	linkPath := filepath.Join(workspace, "link")
	require.NoError(t, os.Symlink(outsideDir, linkPath))

	// SEC-5: Writing to /workspace/link/newfile should be rejected because
	// link -> outsideDir, so the effective path is outsideDir/newfile.
	policy := NewFileScopePolicy(workspace, nil)
	escapePath := filepath.Join(linkPath, "newfile")
	err := policy.Check(FileSystemWrite, escapePath)
	require.Error(t, err, "write through symlinked parent to outside workspace must be rejected")
	require.ErrorIs(t, err, ErrFileScopeOutsideWorkspace)

	// Also test read through symlink.
	err = policy.Check(FileSystemRead, escapePath)
	require.Error(t, err, "read through symlinked parent to outside workspace must be rejected")
	require.ErrorIs(t, err, ErrFileScopeOutsideWorkspace)
}

func TestCanonicalScopePathExistingLeafSymlink(t *testing.T) {
	workspace := t.TempDir()

	// Create a file outside, then symlink to it from inside the workspace.
	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("data"), 0o600))

	linkPath := filepath.Join(workspace, "link_target.txt")
	require.NoError(t, os.Symlink(outsideFile, linkPath))

	// Reading an existing symlink that points outside must be rejected.
	policy := NewFileScopePolicy(workspace, nil)
	err := policy.Check(FileSystemRead, linkPath)
	require.Error(t, err, "existing symlink to outside workspace must be rejected")
	require.ErrorIs(t, err, ErrFileScopeOutsideWorkspace)

	// Create a symlink that points inside the workspace.
	insideDir := filepath.Join(workspace, "subdir")
	require.NoError(t, os.MkdirAll(insideDir, 0o700))
	insideLink := filepath.Join(workspace, "inside_link")
	require.NoError(t, os.Symlink(insideDir, insideLink))

	// Accessing under an inside-pointing symlink must be allowed.
	legitPath := filepath.Join(insideLink, "nested.txt")
	require.NoError(t, os.WriteFile(legitPath, []byte("data"), 0o600))
	require.NoError(t, policy.Check(FileSystemRead, legitPath), "legitimate nested write through inside symlink must be allowed")
}

func TestCanonicalScopePathLegitimateNestedWrites(t *testing.T) {
	workspace := t.TempDir()

	// Direct writes inside workspace must be allowed.
	policy := NewFileScopePolicy(workspace, nil)
	path := filepath.Join(workspace, "a", "b", "c", "newfile.txt")
	require.NoError(t, policy.Check(FileSystemWrite, path), "new file inside workspace must be allowed")

	// Write to an existing directory inside workspace must be allowed.
	existingDir := filepath.Join(workspace, "existing")
	require.NoError(t, os.MkdirAll(existingDir, 0o700))
	existingFile := filepath.Join(existingDir, "file.txt")
	require.NoError(t, policy.Check(FileSystemWrite, existingFile), "write to existing dir inside workspace must be allowed")
}

func TestCanonicalScopePathDeepSymlinkChain(t *testing.T) {
	workspace := t.TempDir()

	// Create a chain: ws/lvl1 -> ws/target, ws/lvl2 -> ws/lvl1
	target := filepath.Join(workspace, "target")
	require.NoError(t, os.MkdirAll(target, 0o700))

	lvl1 := filepath.Join(workspace, "lvl1")
	require.NoError(t, os.Symlink(target, lvl1))

	lvl2 := filepath.Join(workspace, "lvl2")
	require.NoError(t, os.Symlink(lvl1, lvl2))

	// New file through double symlink chain must be allowed (stays inside workspace).
	policy := NewFileScopePolicy(workspace, nil)
	deepPath := filepath.Join(lvl2, "deep_new.txt")
	require.NoError(t, policy.Check(FileSystemWrite, deepPath), "deep symlink chain inside workspace must be allowed")

	// Symlink chain leading outside must be rejected.
	outside := t.TempDir()
	outsideLink := filepath.Join(workspace, "outside_chain")
	require.NoError(t, os.Symlink(outside, outsideLink))
	require.ErrorIs(t, policy.Check(FileSystemRead, filepath.Join(outsideLink, "any.txt")), ErrFileScopeOutsideWorkspace)
}
