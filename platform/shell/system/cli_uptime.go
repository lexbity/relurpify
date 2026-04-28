package system

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewUptimeTool exposes the uptime CLI.
func NewUptimeTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_uptime",
		Description: "Shows system uptime information.",
		Command:     "uptime",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
