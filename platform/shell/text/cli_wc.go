package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewWCTool exposes the wc CLI.
func NewWCTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_wc",
		Description: "Counts lines, words, and bytes via wc.",
		Command:     "wc",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
