package build

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewMakeTool exposes the make CLI.
func NewMakeTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_make",
		Description: "Runs make targets for builds.",
		Command:     "make",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
