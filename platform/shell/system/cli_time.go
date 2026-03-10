package system

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewTimeTool exposes the time CLI.
func NewTimeTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_time",
		Description: "Times the execution of commands with /usr/bin/time.",
		Command:     "time",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
