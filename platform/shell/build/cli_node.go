package build

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewNodeTool exposes the node CLI for running JavaScript inside the workspace.
func NewNodeTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_node",
		Description: "Executes Node.js commands inside the workspace.",
		Command:     "node",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
