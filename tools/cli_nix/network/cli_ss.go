package network

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewSSTool exposes the ss CLI.
func NewSSTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ss",
		Description: "Inspects sockets using ss.",
		Command:     "ss",
		Category:    "cli_network",
	})
}
