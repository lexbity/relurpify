package scheduler

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/shell/catalog"
)

// Tools returns scheduling helpers.
func Tools(basePath string) []contracts.Tool {
	return []contracts.Tool{
		NewCrontabTool(basePath),
		NewAtTool(basePath),
	}
}

// CatalogEntries returns declarative catalog metadata for the scheduler family.
func CatalogEntries() []catalog.ToolCatalogEntry {
	specs := []catalog.CommandToolSpec{
		{Name: "cli_crontab", Family: "scheduler", Intent: []string{"inspect", "reconcile"}, Description: "Edits or lists cron jobs via crontab.", Command: "crontab", Tags: []string{contracts.TagExecute}},
		{Name: "cli_at", Family: "scheduler", Intent: []string{"schedule"}, Description: "Schedules one-off jobs using at.", Command: "at", Tags: []string{contracts.TagExecute}},
	}
	entries := make([]catalog.ToolCatalogEntry, 0, len(specs))
	for _, spec := range specs {
		entries = append(entries, catalog.EntryFromCommandSpec(spec))
	}
	return entries
}
