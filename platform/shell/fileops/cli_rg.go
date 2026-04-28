package fileops

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewRGTool exposes the rg CLI.
func NewRGTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_rg",
		Description: "Uses ripgrep (rg) for recursive code search.",
		Command:     "rg",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
