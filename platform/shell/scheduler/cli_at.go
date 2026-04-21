package scheduler

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewAtTool exposes the at CLI.
func NewAtTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_at",
		Description: "Schedules one-off jobs using at.",
		Command:     "at",
		Category:    "cli_scheduler",
		Tags:        []string{"execute"},
	})
}
