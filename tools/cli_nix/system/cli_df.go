package system

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewDFTool exposes the df CLI.
func NewDFTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_df",
		Description: "Reports disk usage statistics with df.",
		Command:     "df",
		Category:    "cli_system",
		Tags:        []string{"execute", "read-only"},
	})
}
