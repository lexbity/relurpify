package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewJQTool exposes the jq CLI.
func NewJQTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_jq",
		Description: "Queries or transforms JSON using jq.",
		Command:     "jq",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
