package fileops

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewTreeTool exposes the tree CLI.
func NewTreeTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_tree",
		Description: "Displays directory trees using tree.",
		Command:     "tree",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
