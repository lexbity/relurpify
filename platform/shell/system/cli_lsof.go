package system

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewLsofTool exposes the lsof CLI.
func NewLsofTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_lsof",
		Description: "Lists open files and sockets via lsof.",
		Command:     "lsof",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
