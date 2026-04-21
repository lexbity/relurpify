package system

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
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
