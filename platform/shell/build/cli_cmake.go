package build

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
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
