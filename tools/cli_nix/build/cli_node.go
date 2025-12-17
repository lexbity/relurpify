package build

import (
	"github.com/lexcodex/relurpify/framework"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewNodeTool exposes the node CLI for running JavaScript inside the workspace.
func NewNodeTool(basePath string) framework.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_node",
		Description: "Executes Node.js commands inside the workspace.",
		Command:     "node",
		Category:    "cli_build",
	})
}

