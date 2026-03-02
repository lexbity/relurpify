package build

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
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
