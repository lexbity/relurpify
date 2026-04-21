package shell

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/shell/catalog"
	"github.com/stretchr/testify/require"
)

func TestShellCatalogEntriesAreFamilyDrivenAndDeterministic(t *testing.T) {
	entries := CatalogEntries()
	require.NotEmpty(t, entries)

	names := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		require.NotEmpty(t, entry.Name)
		require.NotEmpty(t, entry.Family)
		require.NotEmpty(t, entry.Intent)
		require.NotEmpty(t, entry.Description)
		require.NotEmpty(t, entry.Preset.CommandTemplate)
		require.Equal(t, 1, len(entry.Preset.CommandTemplate))
		require.NotContains(t, seen, entry.Name)
		seen[entry.Name] = struct{}{}
		names = append(names, entry.Name)
	}

	expected := []string{
		"cli_awk",
		"cli_echo",
		"cli_sed",
		"cli_perl",
		"cli_jq",
		"cli_yq",
		"cli_tr",
		"cli_cut",
		"cli_paste",
		"cli_column",
		"cli_sort",
		"cli_uniq",
		"cli_comm",
		"cli_rev",
		"cli_wc",
		"cli_patch",
		"cli_ed",
		"cli_ex",
		"cli_xxd",
		"cli_hexdump",
		"cli_diff",
		"cli_colordiff",
		"cli_git",
		"cli_find",
		"cli_fd",
		"cli_rg",
		"cli_ag",
		"cli_locate",
		"cli_tree",
		"cli_stat",
		"cli_file",
		"cli_touch",
		"cli_mkdir",
		"cli_lsblk",
		"cli_df",
		"cli_du",
		"cli_ps",
		"cli_top",
		"cli_htop",
		"cli_lsof",
		"cli_strace",
		"cli_time",
		"cli_uptime",
		"cli_systemctl",
		"cli_make",
		"cli_cmake",
		"cli_cargo",
		"cli_go",
		"cli_python",
		"cli_node",
		"cli_npm",
		"cli_sqlite3",
		"cli_rustfmt",
		"cli_pkg_config",
		"cli_gdb",
		"cli_valgrind",
		"cli_ldd",
		"cli_objdump",
		"cli_perf",
		"cli_tar",
		"cli_gzip",
		"cli_bzip2",
		"cli_xz",
		"cli_curl",
		"cli_wget",
		"cli_nc",
		"cli_dig",
		"cli_nslookup",
		"cli_ip",
		"cli_ss",
		"cli_ping",
		"cli_crontab",
		"cli_at",
	}
	require.Equal(t, expected, names)
}

func TestShellCatalogEntriesPreserveFamilyIntentMetadata(t *testing.T) {
	entries := CatalogEntries()
	index := make(map[string]catalog.ToolCatalogEntry, len(entries))
	for _, entry := range entries {
		index[entry.Name] = entry
	}

	require.Equal(t, "text", index["cli_jq"].Family)
	require.Contains(t, index["cli_jq"].Intent, "extract")
	require.Equal(t, "build", index["cli_cargo"].Family)
	require.Contains(t, index["cli_cargo"].Intent, "rust")
	require.Equal(t, "system", index["cli_systemctl"].Family)
	require.Contains(t, index["cli_systemctl"].Tags, core.TagDestructive)
	require.Equal(t, "network", index["cli_curl"].Family)
	require.Contains(t, index["cli_curl"].Tags, core.TagNetwork)
}
