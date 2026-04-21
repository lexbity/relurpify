package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewSedTool exposes the sed CLI.
func NewSedTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_sed",
		Description: "Runs sed for stream editing tasks.",
		Command:     "sed",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
