package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewColumnTool exposes the column CLI.
func NewColumnTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_column",
		Description: "Formats text into aligned columns.",
		Command:     "column",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
