package network

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewIPTool exposes the ip CLI.
func NewIPTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ip",
		Description: "Manages network interfaces with ip.",
		Command:     "ip",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
