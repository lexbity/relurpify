package network

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewSSTool exposes the ss CLI.
func NewSSTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ss",
		Description: "Inspects sockets using ss.",
		Command:     "ss",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
