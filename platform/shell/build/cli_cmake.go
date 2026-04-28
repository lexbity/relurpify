package build

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewCMakeTool exposes the cmake CLI.
func NewCMakeTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_cmake",
		Description: "Configures builds with cmake.",
		Command:     "cmake",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
