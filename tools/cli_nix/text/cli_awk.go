package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewAwkTool exposes the awk CLI.
func NewAwkTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_awk",
		Description: "Runs awk for advanced text processing.",
		Command:     "awk",
		Category:    "cli_text",
	})
}
