package build

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPkgConfigTool exposes the pkg-config CLI.
func NewPkgConfigTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_pkg_config",
		Description: "Queries compiler flags with pkg-manifest.",
		Command:     "pkg-config",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
