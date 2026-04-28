package system

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewLsblkTool exposes the lsblk CLI.
func NewLsblkTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_lsblk",
		Description: "Lists block devices via lsblk.",
		Command:     "lsblk",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
