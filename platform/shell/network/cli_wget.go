package network

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewWgetTool exposes the wget CLI.
func NewWgetTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_wget",
		Description: "Downloads resources with wget.",
		Command:     "wget",
		Category:    "cli_network",
		Tags:        []string{"execute", "network"},
	})
}
