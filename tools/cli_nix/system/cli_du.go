package system

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewDUTool exposes the du CLI.
func NewDUTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_du",
		Description: "Summarizes directory usage with du.",
		Command:     "du",
		Category:    "cli_system",
	})
}
