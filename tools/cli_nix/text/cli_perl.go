package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewPerlTool exposes the perl CLI.
func NewPerlTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_perl",
		Description: "Executes Perl one-liners for transformations.",
		Command:     "perl",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
