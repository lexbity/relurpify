package system

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
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
