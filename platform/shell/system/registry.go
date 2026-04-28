package system

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/shell/catalog"
)

// Tools returns system inspection helpers.
func Tools(basePath string) []contracts.Tool {
	return []contracts.Tool{
		NewLsblkTool(basePath),
		NewDFTool(basePath),
		NewDUTool(basePath),
		NewPSTool(basePath),
		NewTopTool(basePath),
		NewHtopTool(basePath),
		NewLsofTool(basePath),
		NewStraceTool(basePath),
		NewTimeTool(basePath),
		NewUptimeTool(basePath),
		NewSystemctlTool(basePath),
	}
}

// CatalogEntries returns declarative catalog metadata for the system family.
func CatalogEntries() []catalog.ToolCatalogEntry {
	specs := []catalog.CommandToolSpec{
		{Name: "cli_lsblk", Family: "system", Intent: []string{"inspect", "storage"}, Description: "Lists block devices via lsblk.", Command: "lsblk", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_df", Family: "system", Intent: []string{"inspect", "storage"}, Description: "Reports disk usage statistics with df.", Command: "df", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_du", Family: "system", Intent: []string{"inspect", "storage"}, Description: "Summarizes directory usage with du.", Command: "du", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_ps", Family: "system", Intent: []string{"inspect", "process"}, Description: "Inspects running processes via ps.", Command: "ps", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_top", Family: "system", Intent: []string{"inspect", "process"}, Description: "Monitors processes interactively with top.", Command: "top", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_htop", Family: "system", Intent: []string{"inspect", "process"}, Description: "Runs htop for interactive process monitoring.", Command: "htop", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_lsof", Family: "system", Intent: []string{"inspect", "process"}, Description: "Lists open files and sockets via lsof.", Command: "lsof", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_strace", Family: "system", Intent: []string{"inspect", "diagnostics"}, Description: "Traces syscalls made by a process using strace.", Command: "strace", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_time", Family: "system", Intent: []string{"measure", "timing"}, Description: "Times the execution of commands with /usr/bin/time.", Command: "time", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_uptime", Family: "system", Intent: []string{"inspect", "uptime"}, Description: "Shows system uptime information.", Command: "uptime", Tags: []string{contracts.TagExecute, contracts.TagReadOnly}},
		{Name: "cli_systemctl", Family: "system", Intent: []string{"manage", "service"}, Description: "Manages systemd services via systemctl.", Command: "systemctl", Tags: []string{contracts.TagExecute, contracts.TagDestructive}},
	}
	entries := make([]catalog.ToolCatalogEntry, 0, len(specs))
	for _, spec := range specs {
		entries = append(entries, catalog.EntryFromCommandSpec(spec))
	}
	return entries
}
