package network

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewDigTool exposes the dig CLI.
func NewDigTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_dig",
		Description: "Queries DNS records using dig.",
		Command:     "dig",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
