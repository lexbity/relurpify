package network

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewNCTool exposes the nc CLI.
func NewNCTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_nc",
		Description: "Creates TCP/UDP connections via netcat (nc).",
		Command:     "nc",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
