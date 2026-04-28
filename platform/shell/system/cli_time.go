package system

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewTimeTool exposes the time CLI.
func NewTimeTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_time",
		Description: "Times the execution of commands with /usr/bin/time.",
		Command:     "time",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
