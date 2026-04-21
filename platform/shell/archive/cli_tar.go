package archive

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewTarTool exposes the tar CLI.
func NewTarTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_tar",
		Description: "Creates or extracts tar archives.",
		Command:     "tar",
		Category:    "cli_archive",
		Tags:        []string{"execute"},
	})
}
