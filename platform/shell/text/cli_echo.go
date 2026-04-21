package text

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewEchoTool exposes the echo CLI utility.
func NewEchoTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_echo",
		Description: "Writes text to standard output using echo.",
		Command:     "echo",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
