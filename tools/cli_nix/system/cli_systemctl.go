package system

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewSystemctlTool exposes the systemctl CLI.
func NewSystemctlTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_systemctl",
		Description: "Manages systemd services via systemctl.",
		Command:     "systemctl",
		Category:    "cli_system",
		Tags:        []string{"execute", "destructive"},
	})
}
