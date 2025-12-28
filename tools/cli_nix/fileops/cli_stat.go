package fileops

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewStatTool exposes the stat CLI.
func NewStatTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_stat",
		Description: "Shows file metadata with stat.",
		Command:     "stat",
		Category:    "cli_files",
	})
}
