package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewCommTool exposes the comm CLI.
func NewCommTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_comm",
		Description: "Compares sorted files using comm.",
		Command:     "comm",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
