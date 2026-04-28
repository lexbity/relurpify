package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewSortTool exposes the sort CLI.
func NewSortTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_sort",
		Description: "Sorts lines of text with the sort utility.",
		Command:     "sort",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
