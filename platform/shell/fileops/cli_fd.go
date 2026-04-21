package fileops

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewFDTool exposes the fd CLI.
func NewFDTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_fd",
		Description: "Performs fast file searches with fd.",
		Command:     "fd",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
