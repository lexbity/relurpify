package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewRevTool exposes the rev CLI.
func NewRevTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_rev",
		Description: "Reverses lines character-wise using rev.",
		Command:     "rev",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
