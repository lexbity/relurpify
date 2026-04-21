package network

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewSSTool exposes the ss CLI.
func NewSSTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ss",
		Description: "Inspects sockets using ss.",
		Command:     "ss",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
