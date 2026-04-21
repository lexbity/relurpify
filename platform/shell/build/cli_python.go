package build

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPythonTool exposes python3 for running Python inside the workspace.
func NewPythonTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_python",
		Description: "Executes Python commands using python3 inside the workspace.",
		Command:     "python3",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
