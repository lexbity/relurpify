package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewDiffTool exposes the diff CLI.
func NewDiffTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_diff",
		Description: "Runs diff to compare files.",
		Command:     "diff",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
