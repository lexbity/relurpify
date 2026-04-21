package archive

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewXzTool exposes the xz CLI.
func NewXzTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_xz",
		Description: "Compresses data with xz.",
		Command:     "xz",
		Category:    "cli_archive",
		Tags:        []string{"execute"},
	})
}
