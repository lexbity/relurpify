package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewColordiffTool exposes the colordiff CLI.
func NewColordiffTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_colordiff",
		Description: "Shows colorized diffs using colordiff.",
		Command:     "colordiff",
		Category:    "cli_text",
	})
}
