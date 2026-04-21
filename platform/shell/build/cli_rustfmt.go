package build

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewRustfmtTool exposes rustfmt for formatting Rust code.
func NewRustfmtTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_rustfmt",
		Description: "Formats Rust code using rustfmt.",
		Command:     "rustfmt",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
