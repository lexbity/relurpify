package system

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewDUTool exposes the du CLI.
func NewDUTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_du",
		Description: "Summarizes directory usage with du.",
		Command:     "du",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
