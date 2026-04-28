package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewWCTool exposes the wc CLI.
func NewWCTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_wc",
		Description: "Counts lines, words, and bytes via wc.",
		Command:     "wc",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
