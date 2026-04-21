package system

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPSTool exposes the ps CLI.
func NewPSTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ps",
		Description: "Inspects running processes via ps.",
		Command:     "ps",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
