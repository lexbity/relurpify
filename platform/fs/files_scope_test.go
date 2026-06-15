package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

func TestAllFileTools_NilScopeDenies(t *testing.T) {
	dir := t.TempDir()
	canary := filepath.Join(dir, "canary.txt")

	t.Run("ReadFileTool", func(t *testing.T) {
		tool := &ReadFileTool{BasePath: dir}
		_, err := tool.Execute(context.Background(), map[string]any{"path": "canary.txt"})
		require.Error(t, err)
		require.ErrorIs(t, err, permissions.ErrSandboxScopeUnset)
		_, statErr := os.Stat(canary)
		require.True(t, os.IsNotExist(statErr))
	})
	t.Run("WriteFileTool", func(t *testing.T) {
		tool := &WriteFileTool{BasePath: dir}
		_, err := tool.Execute(context.Background(), map[string]any{"path": "canary.txt", "content": "data"})
		require.Error(t, err)
		require.ErrorIs(t, err, permissions.ErrSandboxScopeUnset)
		_, statErr := os.Stat(canary)
		require.True(t, os.IsNotExist(statErr))
	})
	t.Run("CreateFileTool", func(t *testing.T) {
		tool := &CreateFileTool{BasePath: dir}
		_, err := tool.Execute(context.Background(), map[string]any{"path": "canary.txt", "content": "data"})
		require.Error(t, err)
		require.ErrorIs(t, err, permissions.ErrSandboxScopeUnset)
		_, statErr := os.Stat(canary)
		require.True(t, os.IsNotExist(statErr))
	})
	t.Run("DeleteFileTool", func(t *testing.T) {
		tool := &DeleteFileTool{BasePath: dir}
		_, err := tool.Execute(context.Background(), map[string]any{"path": "canary.txt"})
		require.Error(t, err)
		require.ErrorIs(t, err, permissions.ErrSandboxScopeUnset)
	})
	t.Run("EditFileTool", func(t *testing.T) {
		tool := &EditFileTool{BasePath: dir}
		_, err := tool.Execute(context.Background(), map[string]any{"path": "canary.txt", "old_string": "x", "new_string": "y"})
		require.Error(t, err)
		require.ErrorIs(t, err, permissions.ErrSandboxScopeUnset)
		_, statErr := os.Stat(canary)
		require.True(t, os.IsNotExist(statErr))
	})
	t.Run("ListFilesTool", func(t *testing.T) {
		tool := &ListFilesTool{BasePath: dir}
		_, err := tool.Execute(context.Background(), map[string]any{"directory": ".", "pattern": "*"})
		require.Error(t, err)
		require.ErrorIs(t, err, permissions.ErrSandboxScopeUnset)
	})
	t.Run("SearchInFilesTool", func(t *testing.T) {
		tool := &SearchInFilesTool{BasePath: dir}
		_, err := tool.Execute(context.Background(), map[string]any{"directory": ".", "pattern": "test"})
		require.Error(t, err)
		require.ErrorIs(t, err, permissions.ErrSandboxScopeUnset)
	})
}

func TestAllFileTools_DenyAllScopeDenies(t *testing.T) {
	denyAll := permissions.NewDenyAllFileScopePolicy()
	dir := t.TempDir()
	canary := filepath.Join(dir, "canary.txt")
	require.NoError(t, os.WriteFile(canary, []byte("data"), 0o600))

	t.Run("ReadFileTool", func(t *testing.T) {
		tool := &ReadFileTool{BasePath: dir}
		tool.SetSandboxScope(denyAll)
		_, err := tool.Execute(context.Background(), map[string]any{"path": "canary.txt"})
		require.Error(t, err)
	})
	t.Run("WriteFileTool", func(t *testing.T) {
		tool := &WriteFileTool{BasePath: dir}
		tool.SetSandboxScope(denyAll)
		_, err := tool.Execute(context.Background(), map[string]any{"path": "new.txt", "content": "data"})
		require.Error(t, err)
	})
	t.Run("CreateFileTool", func(t *testing.T) {
		tool := &CreateFileTool{BasePath: dir}
		tool.SetSandboxScope(denyAll)
		_, err := tool.Execute(context.Background(), map[string]any{"path": "new.txt", "content": "data"})
		require.Error(t, err)
	})
	t.Run("DeleteFileTool", func(t *testing.T) {
		tool := &DeleteFileTool{BasePath: dir}
		tool.SetSandboxScope(denyAll)
		_, err := tool.Execute(context.Background(), map[string]any{"path": "canary.txt"})
		require.Error(t, err)
	})
	t.Run("EditFileTool", func(t *testing.T) {
		tool := &EditFileTool{BasePath: dir}
		tool.SetSandboxScope(denyAll)
		_, err := tool.Execute(context.Background(), map[string]any{"path": "canary.txt", "old_string": "data", "new_string": "changed"})
		require.Error(t, err)
	})
	t.Run("ListFilesTool", func(t *testing.T) {
		tool := &ListFilesTool{BasePath: dir}
		tool.SetSandboxScope(denyAll)
		_, err := tool.Execute(context.Background(), map[string]any{"directory": ".", "pattern": "*"})
		require.Error(t, err)
	})
	t.Run("SearchInFilesTool", func(t *testing.T) {
		tool := &SearchInFilesTool{BasePath: dir}
		tool.SetSandboxScope(denyAll)
		_, err := tool.Execute(context.Background(), map[string]any{"directory": ".", "pattern": "data"})
		require.Error(t, err)
	})
}

func TestAllFileTools_PermissiveScopeAllowsInside(t *testing.T) {
	dir := t.TempDir()
	scope := NewFileScopePolicy(dir, nil)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("hello"), 0o600))

	readTool := &ReadFileTool{BasePath: dir}
	readTool.SetSandboxScope(scope)
	res, err := readTool.Execute(context.Background(), map[string]any{"path": "existing.txt"})
	require.NoError(t, err)
	require.NotNil(t, res)

	writeTool := &WriteFileTool{BasePath: dir}
	writeTool.SetSandboxScope(scope)
	_, err = writeTool.Execute(context.Background(), map[string]any{"path": "new.txt", "content": "world"})
	require.NoError(t, err)

	listTool := &ListFilesTool{BasePath: dir}
	listTool.SetSandboxScope(scope)
	_, err = listTool.Execute(context.Background(), map[string]any{"directory": ".", "pattern": "*.txt"})
	require.NoError(t, err)
}

func TestAllFileTools_PermissiveScopeDeniesOutside(t *testing.T) {
	dir := t.TempDir()
	scope := NewFileScopePolicy(dir, nil)

	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("secret"), 0o600))

	readTool := &ReadFileTool{}
	readTool.SetSandboxScope(scope)
	_, err := readTool.Execute(context.Background(), map[string]any{"path": outsideFile})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrFileScopeOutsideWorkspace)

	writeTool := &WriteFileTool{}
	writeTool.SetSandboxScope(scope)
	_, err = writeTool.Execute(context.Background(), map[string]any{"path": outsideFile, "content": "bad"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrFileScopeOutsideWorkspace)
}

func TestScopedFileTool_EnforceSandboxScope(t *testing.T) {
	t.Run("nil scope denies", func(t *testing.T) {
		s := &scopedFileTool{scope: nil}
		err := s.enforceSandboxScope(permissions.FileSystemRead, "/any/path")
		require.Error(t, err)
		require.ErrorIs(t, err, permissions.ErrSandboxScopeUnset)
	})

	t.Run("valid scope checks path", func(t *testing.T) {
		dir := t.TempDir()
		scope := NewFileScopePolicy(dir, nil)
		s := &scopedFileTool{scope: scope}
		err := s.enforceSandboxScope(permissions.FileSystemRead, filepath.Join(dir, "foo.txt"))
		require.NoError(t, err)
		err = s.enforceSandboxScope(permissions.FileSystemRead, filepath.Join(t.TempDir(), "foo.txt"))
		require.Error(t, err)
		require.ErrorIs(t, err, ErrFileScopeOutsideWorkspace)
	})

	t.Run("SetSandboxScope sets scope", func(t *testing.T) {
		s := &scopedFileTool{}
		scope := NewFileScopePolicy(t.TempDir(), nil)
		s.SetSandboxScope(scope)
		require.NotNil(t, s.scope)
	})
}

func TestCheckOrDeny(t *testing.T) {
	t.Run("nil scope returns ErrSandboxScopeUnset", func(t *testing.T) {
		err := permissions.CheckOrDeny(nil, permissions.FileSystemRead, "/path")
		require.ErrorIs(t, err, permissions.ErrSandboxScopeUnset)
	})

	t.Run("deny-all scope rejects", func(t *testing.T) {
		denyAll := permissions.NewDenyAllFileScopePolicy()
		err := permissions.CheckOrDeny(denyAll, permissions.FileSystemRead, "/any/path")
		require.Error(t, err)
	})

	t.Run("permissive scope allows allowed path", func(t *testing.T) {
		dir := t.TempDir()
		scope := NewFileScopePolicy(dir, nil)
		err := permissions.CheckOrDeny(scope, permissions.FileSystemRead, filepath.Join(dir, "x.txt"))
		require.NoError(t, err)
	})
}
