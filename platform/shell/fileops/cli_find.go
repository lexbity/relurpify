package fileops

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewFindTool exposes the find CLI.
func NewFindTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_find",
		Description: "Searches the filesystem using find.",
		Command:     "find",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
