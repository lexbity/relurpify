package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewEdTool exposes the ed CLI.
func NewEdTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ed",
		Description: "Runs the ed line editor for scripted edits.",
		Command:     "ed",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
