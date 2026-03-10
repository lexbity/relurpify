package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewTRTool exposes the tr CLI.
func NewTRTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_tr",
		Description: "Translates or deletes characters with tr.",
		Command:     "tr",
		Category:    "cli_text",
		Tags:        []string{"execute"},
	})
}
