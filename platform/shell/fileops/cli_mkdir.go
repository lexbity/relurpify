package fileops

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewMkdirTool exposes the mkdir CLI utility for directory creation.
func NewMkdirTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_mkdir",
		Description: "Creates directories via mkdir (defaults to -p).",
		Command:     "mkdir",
		Category:    "cli_files",
		DefaultArgs: []string{"-p"},
		Tags:        []string{"execute", "read-only"},
	})
}
