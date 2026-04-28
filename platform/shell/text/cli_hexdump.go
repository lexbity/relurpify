package text

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewHexdumpTool exposes the hexdump CLI.
func NewHexdumpTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_hexdump",
		Description: "Inspects binary data using hexdump.",
		Command:     "hexdump",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
