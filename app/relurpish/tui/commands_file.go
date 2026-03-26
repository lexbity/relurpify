package tui

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// filePickerResultMsg is emitted when file picker query results are ready.
type filePickerResultMsg struct {
	Query   string
	Results []string // relative paths matching the query
	Err     error
}

// filePickerQueryCmd performs an async file search for files matching a query
// prefix. It routes through the capability registry (cli_find) so that the
// same workspace-bounds, permission checks, and audit trail apply as for agent
// tool calls. Results are returned as workspace-relative paths.
func filePickerQueryCmd(rt RuntimeAdapter, workspace, prefix string) tea.Cmd {
	return func() tea.Msg {
		if prefix == "" {
			return filePickerResultMsg{Query: prefix}
		}

		// Strip leading '@' to get the raw pattern fragment
		pattern := strings.TrimPrefix(prefix, "@")
		if pattern == "" {
			pattern = "*"
		} else {
			pattern = pattern + "*"
		}

		result, err := rt.InvokeCapability(context.Background(), "cli_find", map[string]any{
			"args": []string{
				workspace,
				"-maxdepth", "4",
				"-name", pattern,
				"-type", "f",
				"-not", "-path", "*/.*",
			},
		})
		if err != nil {
			return filePickerResultMsg{Query: prefix, Err: err}
		}

		output := ""
		if result != nil {
			if s, ok := result.Data["stdout"].(string); ok {
				output = s
			}
		}

		var results []string
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			rel, err := filepath.Rel(workspace, line)
			if err != nil {
				continue
			}
			results = append(results, rel)
		}

		sort.Strings(results)
		if len(results) > 10 {
			results = results[:10]
		}

		return filePickerResultMsg{Query: prefix, Results: results}
	}
}
