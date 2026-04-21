package shell

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/shell/archive"
	"codeburg.org/lexbit/relurpify/platform/shell/build"
	"codeburg.org/lexbit/relurpify/platform/shell/fileops"
	"codeburg.org/lexbit/relurpify/platform/shell/network"
	"codeburg.org/lexbit/relurpify/platform/shell/scheduler"
	"codeburg.org/lexbit/relurpify/platform/shell/system"
	"codeburg.org/lexbit/relurpify/platform/shell/text"
	"github.com/stretchr/testify/require"
)

func toolNames(tools []core.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

func TestShellFamilyInventoryMatchesCurrentRegistries(t *testing.T) {
	base := t.TempDir()

	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{
			name: "text",
			got:  toolNames(text.Tools(base)),
			want: []string{
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
			},
		},
		{
			name: "fileops",
			got:  toolNames(fileops.Tools(base)),
			want: []string{
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
			},
		},
		{
			name: "network",
			got:  toolNames(network.Tools(base)),
			want: []string{
				"cli_curl",
				"cli_wget",
				"cli_nc",
				"cli_dig",
				"cli_nslookup",
				"cli_ip",
				"cli_ss",
				"cli_ping",
			},
		},
		{
			name: "system",
			got:  toolNames(system.Tools(base)),
			want: []string{
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
			},
		},
		{
			name: "build",
			got:  toolNames(build.Tools(base)),
			want: []string{
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
				"cli_strace",
			},
		},
		{
			name: "archive",
			got:  toolNames(archive.Tools(base)),
			want: []string{
				"cli_tar",
				"cli_gzip",
				"cli_bzip2",
				"cli_xz",
			},
		},
		{
			name: "scheduler",
			got:  toolNames(scheduler.Tools(base)),
			want: []string{
				"cli_crontab",
				"cli_at",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.got)
		})
	}
}

func TestCommandLineToolsPreservesShellOrderAndDedupesStrace(t *testing.T) {
	base := t.TempDir()
	runner := &recordingRunner{}

	tools := CommandLineTools(base, runner)
	got := toolNames(tools)

	wantPrefix := []string{}
	seen := map[string]struct{}{}
	appendUnique := func(names ...string) {
		for _, name := range names {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			wantPrefix = append(wantPrefix, name)
		}
	}

	appendUnique(
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
	)
	appendUnique(
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
	)
	appendUnique(
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
	)
	appendUnique(
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
		"cli_strace",
	)
	appendUnique(
		"cli_tar",
		"cli_gzip",
		"cli_bzip2",
		"cli_xz",
	)
	appendUnique(
		"cli_curl",
		"cli_wget",
		"cli_nc",
		"cli_dig",
		"cli_nslookup",
		"cli_ip",
		"cli_ss",
		"cli_ping",
	)
	appendUnique(
		"cli_crontab",
		"cli_at",
	)

	require.Greater(t, len(got), len(wantPrefix))
	require.Equal(t, wantPrefix, got[:len(wantPrefix)])
	require.Equal(t, 1, countName(got, "cli_strace"))
}

func TestExecutionHelperInventory(t *testing.T) {
	cases := []struct {
		name string
		tool interface {
			Name() string
			Description() string
			Category() string
			Parameters() []core.ToolParameter
			Tags() []string
			IsAvailable(context.Context, *core.Context) bool
		}
		wantName string
		wantDesc string
		wantCat  string
		wantTags []string
		wantArgs []core.ToolParameter
	}{
		{
			name:     "tests",
			tool:     &RunTestsTool{},
			wantName: "exec_run_tests",
			wantDesc: "Runs project tests.",
			wantCat:  "execution",
			wantTags: []string{core.TagExecute, "test", "verification"},
			wantArgs: []core.ToolParameter{{Name: "pattern", Type: "string", Required: false}},
		},
		{
			name:     "code",
			tool:     &ExecuteCodeTool{},
			wantName: "exec_run_code",
			wantDesc: "Executes arbitrary code snippets in a sandbox.",
			wantCat:  "execution",
			wantTags: []string{core.TagExecute, "code"},
			wantArgs: []core.ToolParameter{{Name: "code", Type: "string", Required: true}},
		},
		{
			name:     "linter",
			tool:     &RunLinterTool{},
			wantName: "exec_run_linter",
			wantDesc: "Runs linting tools.",
			wantCat:  "execution",
			wantTags: []string{core.TagExecute, "lint", "verification"},
			wantArgs: []core.ToolParameter{{Name: "path", Type: "string", Required: false}},
		},
		{
			name:     "build",
			tool:     &RunBuildTool{},
			wantName: "exec_run_build",
			wantDesc: "Runs builds or compiles the project.",
			wantCat:  "execution",
			wantTags: []string{core.TagExecute, "build", "verification"},
			wantArgs: []core.ToolParameter{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantName, tc.tool.Name())
			require.Equal(t, tc.wantDesc, tc.tool.Description())
			require.Equal(t, tc.wantCat, tc.tool.Category())
			require.Equal(t, tc.wantTags, tc.tool.Tags())
			require.Equal(t, tc.wantArgs, tc.tool.Parameters())
			require.False(t, tc.tool.IsAvailable(nil, core.NewContext()))
		})
	}
}

func countName(names []string, target string) int {
	count := 0
	for _, name := range names {
		if name == target {
			count++
		}
	}
	return count
}
