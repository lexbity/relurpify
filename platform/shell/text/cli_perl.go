package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewPerlTool exposes the perl CLI.
func NewPerlTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_perl",
		Description: "Executes Perl one-liners for transformations.",
		Command:     "perl",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
