package build

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewCMakeTool exposes the cmake CLI.
func NewCMakeTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_cmake",
		Description: "Configures builds with cmake.",
		Command:     "cmake",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
