package fileops

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewTouchTool exposes the touch CLI utility.
func NewTouchTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_touch",
		Description: "Creates empty files or updates timestamps via touch.",
		Command:     "touch",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
