package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewCutTool exposes the cut CLI.
func NewCutTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_cut",
		Description: "Extracts fields or columns with cut.",
		Command:     "cut",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
