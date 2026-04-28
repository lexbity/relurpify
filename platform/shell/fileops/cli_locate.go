package fileops

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewLocateTool exposes the locate CLI.
func NewLocateTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_locate",
		Description: "Queries the file database via locate.",
		Command:     "locate",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
