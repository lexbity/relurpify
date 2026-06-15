package conformance

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	capports "codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm/toolwire"
)

func TestRecipeVisibility_ExcludedCapabilityStaysHidden(t *testing.T) {
	workspace := t.TempDir()

	base := registry.NewRegistry()
	require.NoError(t, base.RegisterLegacyTool(context.Background(), &fs.ReadFileTool{BasePath: workspace}))
	require.NoError(t, base.RegisterLegacyTool(context.Background(), &fs.EditFileTool{BasePath: workspace}))

	readDesc, ok := base.GetCapability("file_read")
	require.True(t, ok)
	editDesc, ok := base.GetCapability("file_edit")
	require.True(t, ok)

	filtered := registry.NewFilteredRegistry(base, []string{readDesc.ID})
	require.True(t, filtered.IsAllowed(readDesc.ID))
	require.False(t, filtered.IsAllowed(editDesc.ID))

	callable := filtered.ModelCallableTools(context.Background())
	require.Len(t, callable, 1)
	require.Equal(t, "file_read", callable[0].Name())

	rendered := toolwire.RenderToolsToPrompt(capports.LLMToolSpecsFromTools(callable))
	require.Contains(t, rendered, "file_read")
	require.NotContains(t, rendered, "file_edit")
	require.NotContains(t, rendered, "old_string")
	require.NotContains(t, rendered, "new_string")
	require.True(t, strings.Contains(rendered, "```tool"))
}
