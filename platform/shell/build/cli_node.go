package build

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewNodeTool exposes the node CLI for running JavaScript inside the workspace.
func NewNodeTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_node",
		Description: "Executes Node.js commands inside the workspace.",
		Command:     "node",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
