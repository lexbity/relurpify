package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewCommTool exposes the comm CLI.
func NewCommTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_comm",
		Description: "Compares sorted files using comm.",
		Command:     "comm",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
