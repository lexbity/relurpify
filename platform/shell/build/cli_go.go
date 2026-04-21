package build

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewGoTool exposes the go CLI for running builds/tests inside the workspace.
func NewGoTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_go",
		Description: "Executes Go commands (go test/build/etc) inside the workspace.",
		Command:     "go",
		Category:    "cli_build",
		Tags:        []string{"execute"},
	})
}
