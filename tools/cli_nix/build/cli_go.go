package build

import (
	"github.com/lexcodex/relurpify/framework"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewGoTool exposes the go CLI for running builds/tests inside the workspace.
func NewGoTool(basePath string) framework.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_go",
		Description: "Executes Go commands (go test/build/etc) inside the workspace.",
		Command:     "go",
		Category:    "cli_build",
	})
}

