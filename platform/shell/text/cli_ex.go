package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewExTool exposes the ex CLI.
func NewExTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ex",
		Description: "Executes ex for vi-style batch editing.",
		Command:     "ex",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
