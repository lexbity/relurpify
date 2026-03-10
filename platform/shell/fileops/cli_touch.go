package fileops

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
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
