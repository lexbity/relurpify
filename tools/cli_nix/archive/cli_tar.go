package archive

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewTarTool exposes the tar CLI.
func NewTarTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_tar",
		Description: "Creates or extracts tar archives.",
		Command:     "tar",
		Category:    "cli_archive",
		Tags:        []string{"execute"},
	})
}
