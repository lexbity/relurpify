package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewRevTool exposes the rev CLI.
func NewRevTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_rev",
		Description: "Reverses lines character-wise using rev.",
		Command:     "rev",
		Category:    "cli_text",
	})
}
