package network

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewDigTool exposes the dig CLI.
func NewDigTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_dig",
		Description: "Queries DNS records using dig.",
		Command:     "dig",
		Category:    "cli_network",
	})
}
