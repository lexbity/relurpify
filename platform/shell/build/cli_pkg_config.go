package build

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
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
