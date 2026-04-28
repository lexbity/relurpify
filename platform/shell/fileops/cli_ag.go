package fileops

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewAGTool exposes the ag CLI.
func NewAGTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ag",
		Description: "Searches codebases with the silver searcher (ag).",
		Command:     "ag",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
