package fileops

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewAGTool exposes the ag CLI.
func NewAGTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ag",
		Description: "Searches codebases with the silver searcher (ag).",
		Command:     "ag",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
