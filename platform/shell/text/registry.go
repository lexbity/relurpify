package text

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/shell/catalog"
)

// Tools returns text-processing related CLI helpers.
func Tools(basePath string) []core.Tool {
	return []core.Tool{
		NewAwkTool(basePath),
		NewEchoTool(basePath),
		NewSedTool(basePath),
		NewPerlTool(basePath),
		NewJQTool(basePath),
		NewYQTool(basePath),
		NewTRTool(basePath),
		NewCutTool(basePath),
		NewPasteTool(basePath),
		NewColumnTool(basePath),
		NewSortTool(basePath),
		NewUniqTool(basePath),
		NewCommTool(basePath),
		NewRevTool(basePath),
		NewWCTool(basePath),
		NewPatchTool(basePath),
		NewEdTool(basePath),
		NewExTool(basePath),
		NewXxdTool(basePath),
		NewHexdumpTool(basePath),
		NewDiffTool(basePath),
		NewColordiffTool(basePath),
	}
}

// CatalogEntries returns declarative catalog metadata for the text family.
func CatalogEntries() []catalog.ToolCatalogEntry {
	specs := []catalog.CommandToolSpec{
		{Name: "cli_awk", Family: "text", Intent: []string{"transform", "inspect"}, Description: "Runs awk for advanced text processing.", Command: "awk", Tags: []string{core.TagExecute}},
		{Name: "cli_echo", Family: "text", Intent: []string{"transform", "emit"}, Description: "Writes text to standard output using echo.", Command: "echo", Tags: []string{core.TagExecute}},
		{Name: "cli_sed", Family: "text", Intent: []string{"transform", "edit"}, Description: "Runs sed for stream editing tasks.", Command: "sed", Tags: []string{core.TagExecute}},
		{Name: "cli_perl", Family: "text", Intent: []string{"transform", "script"}, Description: "Runs perl for text processing tasks.", Command: "perl", Tags: []string{core.TagExecute}},
		{Name: "cli_jq", Family: "text", Intent: []string{"extract", "structured-data"}, Description: "Queries or transforms JSON using jq.", Command: "jq", Tags: []string{core.TagExecute}},
		{Name: "cli_yq", Family: "text", Intent: []string{"extract", "structured-data"}, Description: "Processes YAML content using yq.", Command: "yq", Tags: []string{core.TagExecute}},
		{Name: "cli_tr", Family: "text", Intent: []string{"transform"}, Description: "Translates characters and text with tr.", Command: "tr", Tags: []string{core.TagExecute}},
		{Name: "cli_cut", Family: "text", Intent: []string{"transform", "select"}, Description: "Extracts fields from text with cut.", Command: "cut", Tags: []string{core.TagExecute}},
		{Name: "cli_paste", Family: "text", Intent: []string{"merge"}, Description: "Merges lines using paste.", Command: "paste", Tags: []string{core.TagExecute}},
		{Name: "cli_column", Family: "text", Intent: []string{"format"}, Description: "Formats tabular text with column.", Command: "column", Tags: []string{core.TagExecute}},
		{Name: "cli_sort", Family: "text", Intent: []string{"format", "order"}, Description: "Sorts text with sort.", Command: "sort", Tags: []string{core.TagExecute}},
		{Name: "cli_uniq", Family: "text", Intent: []string{"deduplicate"}, Description: "Removes duplicate lines using uniq.", Command: "uniq", Tags: []string{core.TagExecute}},
		{Name: "cli_comm", Family: "text", Intent: []string{"compare"}, Description: "Compares sorted files with comm.", Command: "comm", Tags: []string{core.TagExecute}},
		{Name: "cli_rev", Family: "text", Intent: []string{"transform"}, Description: "Reverses lines with rev.", Command: "rev", Tags: []string{core.TagExecute}},
		{Name: "cli_wc", Family: "text", Intent: []string{"inspect", "measure"}, Description: "Counts lines, words, and bytes via wc.", Command: "wc", Tags: []string{core.TagExecute}},
		{Name: "cli_patch", Family: "text", Intent: []string{"patch"}, Description: "Applies unified diffs using patch.", Command: "patch", Tags: []string{core.TagExecute}},
		{Name: "cli_ed", Family: "text", Intent: []string{"edit"}, Description: "Edits files with ed.", Command: "ed", Tags: []string{core.TagExecute}},
		{Name: "cli_ex", Family: "text", Intent: []string{"edit"}, Description: "Exposes ex for line-oriented editing.", Command: "ex", Tags: []string{core.TagExecute}},
		{Name: "cli_xxd", Family: "text", Intent: []string{"inspect", "hex"}, Description: "Creates hex dumps with xxd.", Command: "xxd", Tags: []string{core.TagExecute}},
		{Name: "cli_hexdump", Family: "text", Intent: []string{"inspect", "hex"}, Description: "Creates hex dumps with hexdump.", Command: "hexdump", Tags: []string{core.TagExecute}},
		{Name: "cli_diff", Family: "text", Intent: []string{"compare"}, Description: "Runs diff to compare files.", Command: "diff", Tags: []string{core.TagExecute}},
		{Name: "cli_colordiff", Family: "text", Intent: []string{"compare", "format"}, Description: "Shows colorized diffs using colordiff.", Command: "colordiff", Tags: []string{core.TagExecute}},
	}
	entries := make([]catalog.ToolCatalogEntry, 0, len(specs))
	for _, spec := range specs {
		entries = append(entries, catalog.EntryFromCommandSpec(spec))
	}
	return entries
}
