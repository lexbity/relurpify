package build

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewMakeTool exposes the make CLI.
func NewMakeTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_make",
		Description: "Runs make targets for builds.",
		Command:     "make",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
