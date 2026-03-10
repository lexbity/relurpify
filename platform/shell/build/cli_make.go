package build

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
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
