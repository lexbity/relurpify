package system

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewLsblkTool exposes the lsblk CLI.
func NewLsblkTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_lsblk",
		Description: "Lists block devices via lsblk.",
		Command:     "lsblk",
		Category:    "cli_system",
	})
}
