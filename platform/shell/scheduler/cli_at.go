package scheduler

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewAtTool exposes the at CLI.
func NewAtTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_at",
		Description: "Schedules one-off jobs using at.",
		Command:     "at",
		Category:    "cli_scheduler",
		Tags:        []string{"execute"},
	})
}
