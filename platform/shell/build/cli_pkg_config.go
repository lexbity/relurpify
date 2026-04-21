package build

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPkgConfigTool exposes the pkg-config CLI.
func NewPkgConfigTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_pkg_config",
		Description: "Queries compiler flags with pkg-config.",
		Command:     "pkg-config",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
