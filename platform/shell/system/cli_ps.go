package system

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPSTool exposes the ps CLI.
func NewPSTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ps",
		Description: "Inspects running processes via ps.",
		Command:     "ps",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
