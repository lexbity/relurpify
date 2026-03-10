package network

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewNslookupTool exposes the nslookup CLI.
func NewNslookupTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_nslookup",
		Description: "Performs DNS lookups via nslookup.",
		Command:     "nslookup",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
