package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewJQTool exposes the jq CLI.
func NewJQTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_jq",
		Description: "Queries or transforms JSON using jq.",
		Command:     "jq",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
