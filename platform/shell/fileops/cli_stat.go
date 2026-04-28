package fileops

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewStatTool exposes the stat CLI.
func NewStatTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_stat",
		Description: "Shows file metadata with stat.",
		Command:     "stat",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
