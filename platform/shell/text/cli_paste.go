package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPasteTool exposes the paste CLI.
func NewPasteTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_paste",
		Description: "Combines lines from files using paste.",
		Command:     "paste",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
