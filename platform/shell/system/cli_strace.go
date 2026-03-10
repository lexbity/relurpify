package system

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
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
