package network

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPingTool exposes the ping CLI.
func NewPingTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ping",
		Description: "Checks host reachability with ping.",
		Command:     "ping",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
