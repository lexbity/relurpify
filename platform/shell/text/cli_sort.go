package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewSortTool exposes the sort CLI.
func NewSortTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_sort",
		Description: "Sorts lines of text with the sort utility.",
		Command:     "sort",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
