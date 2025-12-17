package build

import (
	"github.com/lexcodex/relurpify/framework"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewNPMTool exposes npm for Node.js package workflows.
func NewNPMTool(basePath string) framework.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_npm",
		Description: "Executes npm commands inside the workspace.",
		Command:     "npm",
		Category:    "cli_build",
	})
}

