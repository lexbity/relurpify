package build

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewNPMTool exposes npm for Node.js package workflows.
func NewNPMTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_npm",
		Description: "Executes npm commands inside the workspace.",
		Command:     "npm",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
