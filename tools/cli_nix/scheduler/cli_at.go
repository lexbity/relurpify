package scheduler

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewAtTool exposes the at CLI.
func NewAtTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_at",
		Description: "Schedules one-off jobs using at.",
		Command:     "at",
		Category:    "cli_scheduler",
	})
}
