package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewPasteTool exposes the paste CLI.
func NewPasteTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_paste",
		Description: "Combines lines from files using paste.",
		Command:     "paste",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
