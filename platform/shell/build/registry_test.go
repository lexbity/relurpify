package build

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
		require.NotEmpty(t, tool.Tags())
		require.False(t, tool.IsAvailable(context.Background(), core.NewContext()))

		perms := tool.Permissions()
		require.NotNil(t, perms.Permissions)
		require.Len(t, perms.Permissions.Executables, 1)
		require.Equal(t, entry.Preset.CommandTemplate[0], perms.Permissions.Executables[0].Binary)
		require.Equal(t, entry.Preset.DefaultArgs, perms.Permissions.Executables[0].Args)

		switch tool.Name() {
		case "cli_gdb", "cli_perf", "cli_strace":
			require.Equal(t, "cli_debug", tool.Category())
			require.True(t, perms.Permissions.Executables[0].HITLRequired)
		case "cli_valgrind", "cli_ldd", "cli_objdump":
			require.Equal(t, "cli_debug", tool.Category())
			require.False(t, perms.Permissions.Executables[0].HITLRequired)
		default:
			require.Equal(t, "cli_build", tool.Category())
			require.False(t, perms.Permissions.Executables[0].HITLRequired)
		}
	}

	require.Contains(t, entries[10].Tags, core.TagDestructive)
	require.Contains(t, entries[14].Tags, core.TagDestructive)
	require.Contains(t, entries[15].Tags, core.TagDestructive)
}
