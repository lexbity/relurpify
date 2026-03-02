package archive

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewXzTool exposes the xz CLI.
func NewXzTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_xz",
		Description: "Compresses data with xz.",
		Command:     "xz",
		Category:    "cli_archive",
		Tags:        []string{"execute"},
	})
}
