package build

import (
	"github.com/lexcodex/relurpify/framework"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewRustfmtTool exposes rustfmt for formatting Rust code.
func NewRustfmtTool(basePath string) framework.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_rustfmt",
		Description: "Formats Rust code using rustfmt.",
		Command:     "rustfmt",
		Category:    "cli_build",
	})
}

