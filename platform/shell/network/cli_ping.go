package network

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewPingTool exposes the ping CLI.
func NewPingTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ping",
		Description: "Checks host reachability with ping.",
		Command:     "ping",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
