package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewCutTool exposes the cut CLI.
func NewCutTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_cut",
		Description: "Extracts fields or columns with cut.",
		Command:     "cut",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
