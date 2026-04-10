package fileops

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
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
		switch entry.Name {
		case "cli_git":
			require.Equal(t, "git", tool.Category())
		default:
			require.Equal(t, "cli_files", tool.Category())
		}
		require.Equal(t, entry.Preset.CommandTemplate[0], tool.Permissions().Permissions.Executables[0].Binary)
		require.Equal(t, entry.Preset.DefaultArgs, tool.Permissions().Permissions.Executables[0].Args)
		require.NotEmpty(t, tool.Tags())
		require.False(t, tool.IsAvailable(context.Background(), core.NewContext()))
	}

	mkdir := tools[len(tools)-1]
	perms := mkdir.Permissions()
	require.Equal(t, []string{"-p"}, perms.Permissions.Executables[0].Args)
	require.Equal(t, []string{"-p"}, entries[len(entries)-1].Preset.DefaultArgs)
}
