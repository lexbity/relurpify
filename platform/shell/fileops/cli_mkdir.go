package fileops

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewMkdirTool exposes the mkdir CLI utility for directory creation.
func NewMkdirTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_mkdir",
		Description: "Creates directories via mkdir (defaults to -p).",
		Command:     "mkdir",
		Category:    "cli_files",
		DefaultArgs: []string{"-p"},
		Tags:        []string{"execute", "read-only"},
	})
}
