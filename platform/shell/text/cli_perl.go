package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPerlTool exposes the perl CLI.
func NewPerlTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_perl",
		Description: "Executes Perl one-liners for transformations.",
		Command:     "perl",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
