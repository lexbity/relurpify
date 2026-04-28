package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewAwkTool exposes the awk CLI.
func NewAwkTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_awk",
		Description: "Runs awk for advanced text processing.",
		Command:     "awk",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
