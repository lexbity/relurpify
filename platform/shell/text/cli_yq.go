package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
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
