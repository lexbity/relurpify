package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewUniqTool exposes the uniq CLI.
func NewUniqTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_uniq",
		Description: "Filters or counts duplicate lines with uniq.",
		Command:     "uniq",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
