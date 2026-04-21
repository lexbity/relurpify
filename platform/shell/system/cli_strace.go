package system

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewStraceTool exposes the strace CLI.
func NewStraceTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_strace",
		Description: "Traces syscalls made by a process using strace.",
		Command:     "strace",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
