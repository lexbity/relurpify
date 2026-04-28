package fileops

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewFileTool exposes the file CLI.
func NewFileTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_file",
		Description: "Detects file types using the file command.",
		Command:     "file",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
