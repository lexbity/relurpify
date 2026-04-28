package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewColordiffTool exposes the colordiff CLI.
func NewColordiffTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_colordiff",
		Description: "Shows colorized diffs using colordiff.",
		Command:     "colordiff",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
