package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
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
