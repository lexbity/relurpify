package search_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	nativesearch "codeburg.org/lexbit/relurpify/platform/tools/native/search"
)

func TestConstructors_KeysAndWiring(t *testing.T) {
	constructors := nativesearch.Constructors()

	wantKeys := []string{"search_find_similar", "search_grep", "search_semantic"}
	require.Len(t, constructors, len(wantKeys))
	for _, key := range wantKeys {
		require.Contains(t, constructors, key)
	}

	for key, ctor := range constructors {
		tool := ctor(t.TempDir())
		require.NotNil(t, tool, "constructor %q returned nil", key)
		require.Equal(t, key, tool.Name(), "tool name must match registry key")
		require.NotEmpty(t, tool.Description())
		require.NotEmpty(t, tool.Category())
		require.NotNil(t, tool.Parameters())
		require.NotNil(t, tool.Permissions().Permissions)
		require.True(t, tool.IsAvailable(context.Background()))
	}
}

func TestConstructors_GrepFindsMatches(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(base, "main.go"), []byte("package main\n\n// TODO fix this\n"), 0o600))

	grep := nativesearch.Constructors()["search_grep"](base)
	res, err := grep.Execute(context.Background(), map[string]any{"pattern": "todo"})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)

	data, err := json.Marshal(res.Data)
	require.NoError(t, err)
	require.Contains(t, string(data), "TODO fix this")
}
