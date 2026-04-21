package build

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewCargoTool exposes the cargo CLI for Rust builds.
func NewCargoTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_cargo",
		Description: "Executes Rust cargo commands inside the workspace.",
		Command:     "cargo",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
