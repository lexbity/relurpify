package archive

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewGzipTool exposes the gzip CLI.
func NewGzipTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_gzip",
		Description: "Compresses data with gzip.",
		Command:     "gzip",
		Category:    "cli_archive",
		Tags:        []string{"execute"},
	})
}
