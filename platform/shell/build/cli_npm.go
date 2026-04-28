package build

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewNPMTool exposes npm for Node.js package workflows.
func NewNPMTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_npm",
		Description: "Executes npm commands inside the workspace.",
		Command:     "npm",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
