package archive

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
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
