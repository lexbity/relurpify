package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewColumnTool exposes the column CLI.
func NewColumnTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_column",
		Description: "Formats text into aligned columns.",
		Command:     "column",
		Category:    "cli_text",
	})
}
