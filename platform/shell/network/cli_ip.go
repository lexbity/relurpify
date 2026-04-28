package network

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewIPTool exposes the ip CLI.
func NewIPTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ip",
		Description: "Manages network interfaces with ip.",
		Command:     "ip",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
