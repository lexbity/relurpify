package fileops

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewFDTool exposes the fd CLI.
func NewFDTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_fd",
		Description: "Performs fast file searches with fd.",
		Command:     "fd",
		Category:    "cli_files",
		Tags:        []string{"execute", "read-only"},
	})
}
