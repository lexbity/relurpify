package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewEdTool exposes the ed CLI.
func NewEdTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ed",
		Description: "Runs the ed line editor for scripted edits.",
		Command:     "ed",
		Category:    "cli_text",
	})
}
