package system

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewHtopTool exposes the htop CLI.
func NewHtopTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_htop",
		Description: "Runs htop for interactive process monitoring.",
		Command:     "htop",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
