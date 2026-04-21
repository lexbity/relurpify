package system

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewTopTool exposes the top CLI.
func NewTopTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_top",
		Description: "Monitors processes interactively with top.",
		Command:     "top",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
