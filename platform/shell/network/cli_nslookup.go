package network

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
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
