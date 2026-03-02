package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewUniqTool exposes the uniq CLI.
func NewUniqTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_uniq",
		Description: "Filters or counts duplicate lines with uniq.",
		Command:     "uniq",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
