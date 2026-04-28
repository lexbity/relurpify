package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPatchTool exposes the patch CLI.
func NewPatchTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_patch",
		Description: "Applies unified diffs using patch.",
		Command:     "patch",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
