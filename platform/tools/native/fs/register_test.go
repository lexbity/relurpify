package fs_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	securefs "codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/capability/ports"
	governanceperms "codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/platform/tools/native/fs"
)

func TestConstructors_KeysAndWiring(t *testing.T) {
	constructors := fs.Constructors()

	wantKeys := []string{
		"file_create", "file_delete", "file_edit",
		"file_list", "file_read", "file_search", "file_write",
	}
	require.Len(t, constructors, len(wantKeys))
	for _, key := range wantKeys {
		require.Contains(t, constructors, key)
	}

	base := t.TempDir()
	for key, ctor := range constructors {
		tool := ctor(base)
		require.NotNil(t, tool, "constructor %q returned nil", key)
		require.Equal(t, key, tool.Name(), "tool name must match registry key")
		require.NotEmpty(t, tool.Description())
		require.NotEmpty(t, tool.Category())
		require.NotNil(t, tool.Parameters())
		require.NotNil(t, tool.Permissions().Permissions)
		require.True(t, tool.IsAvailable(context.Background()))
	}
}

func TestConstructors_WriteReadRoundTrip(t *testing.T) {
	constructors := fs.Constructors()
	base := t.TempDir()

	scope := func(tool ports.Tool) ports.Tool {
		if s, ok := tool.(interface {
			SetSandboxScope(*governanceperms.FileScopePolicy)
		}); ok {
			s.SetSandboxScope(securefs.NewFileScopePolicy(base, nil))
		}
		return tool
	}

	write := scope(constructors["file_write"](base))
	res, err := write.Execute(context.Background(), map[string]any{
		"path":    "notes/hello.txt",
		"content": "hello relurpify",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success, "write must succeed: %+v", res)

	read := scope(constructors["file_read"](base))
	res, err = read.Execute(context.Background(), map[string]any{"path": "notes/hello.txt"})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)

	marshaled, err := json.Marshal(res.Data)
	require.NoError(t, err)
	require.Contains(t, string(marshaled), "hello relurpify")
}
