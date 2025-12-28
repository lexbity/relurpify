package build

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewNPMTool exposes npm for Node.js package workflows.
func NewNPMTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_npm",
		Description: "Executes npm commands inside the workspace.",
		Command:     "npm",
		Category:    "cli_build",
	})
}
