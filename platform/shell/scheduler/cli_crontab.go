package scheduler

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
)

// NewCrontabTool exposes the crontab CLI.
func NewCrontabTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_crontab",
		Description: "Edits or lists cron jobs via crontab.",
		Command:     "crontab",
		Category:    "cli_scheduler",
		Tags:        []string{"execute"},
	})
}
