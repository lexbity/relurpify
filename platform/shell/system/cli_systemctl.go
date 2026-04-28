package system

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewSystemctlTool exposes the systemctl CLI.
func NewSystemctlTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_systemctl",
		Description: "Manages systemd services via systemctl.",
		Command:     "systemctl",
		Category:    "cli_system",
		Tags:        []string{"execute", "destructive"},
	})
}
