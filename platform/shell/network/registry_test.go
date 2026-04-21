package network

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestToolsAndCatalogEntriesMirror(t *testing.T) {
	base := t.TempDir()
	tools := Tools(base)
	entries := CatalogEntries()

	require.Len(t, tools, len(entries))
	for i, tool := range tools {
		entry := entries[i]
		require.Equal(t, entry.Name, tool.Name())
		require.NotEmpty(t, tool.Description())
		require.Equal(t, "cli_network", tool.Category())
		require.Equal(t, entry.Preset.CommandTemplate[0], tool.Permissions().Permissions.Executables[0].Binary)
		require.Equal(t, entry.Preset.DefaultArgs, tool.Permissions().Permissions.Executables[0].Args)
		require.NotEmpty(t, tool.Tags())
		require.False(t, tool.IsAvailable(context.Background(), core.NewContext()))
	}

	require.Contains(t, entries[0].Tags, core.TagNetwork)
	require.Contains(t, tools[0].Tags(), core.TagNetwork)
}
