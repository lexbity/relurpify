package network

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewCurlTool exposes the curl CLI.
func NewCurlTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_curl",
		Description: "Transfers data over HTTP(S) using curl.",
		Command:     "curl",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
