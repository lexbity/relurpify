package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewXxdTool exposes the xxd CLI.
func NewXxdTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_xxd",
		Description: "Creates hex dumps with xxd.",
		Command:     "xxd",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
