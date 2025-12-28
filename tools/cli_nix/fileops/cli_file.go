package fileops

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewFileTool exposes the file CLI.
func NewFileTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_file",
		Description: "Detects file types using the file command.",
		Command:     "file",
		Category:    "cli_files",
	})
}
