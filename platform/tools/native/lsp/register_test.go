package lsp_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	nativelsp "codeburg.org/lexbit/relurpify/platform/tools/native/lsp"
)

func TestConstructors_KeysAndWiring(t *testing.T) {
	constructors := nativelsp.Constructors()

	wantKeys := []string{
		"lsp_document_symbols", "lsp_format", "lsp_get_definition",
		"lsp_get_diagnostics", "lsp_get_hover", "lsp_get_references", "lsp_search_symbols",
	}
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
		// The proxy is wired at composition time, so the tool reports
		// unavailable until then.
		require.False(t, tool.IsAvailable(t.Context()), "tool must be unavailable before proxy wiring")
	}
}
