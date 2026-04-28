package archive

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewBzip2Tool exposes the bzip2 CLI.
func NewBzip2Tool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_bzip2",
		Description: "Compresses data using bzip2.",
		Command:     "bzip2",
		Category:    "cli_archive",
		Tags:        []string{"execute"},
	})
}
