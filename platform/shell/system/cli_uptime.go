package system

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewUptimeTool exposes the uptime CLI.
func NewUptimeTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_uptime",
		Description: "Shows system uptime information.",
		Command:     "uptime",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
