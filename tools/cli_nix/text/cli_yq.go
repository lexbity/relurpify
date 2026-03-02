package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewYQTool exposes the yq CLI.
func NewYQTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_yq",
		Description: "Processes YAML content using yq.",
		Command:     "yq",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
