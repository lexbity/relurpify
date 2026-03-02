package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewSedTool exposes the sed CLI.
func NewSedTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_sed",
		Description: "Runs sed for stream editing tasks.",
		Command:     "sed",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
