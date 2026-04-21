package network

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
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
