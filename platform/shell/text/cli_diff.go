package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewDiffTool exposes the diff CLI.
func NewDiffTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_diff",
		Description: "Runs diff to compare files.",
		Command:     "diff",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
