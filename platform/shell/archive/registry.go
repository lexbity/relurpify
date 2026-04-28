package archive

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/shell/catalog"
)

// Tools returns archiving/compression helpers.
func Tools(basePath string) []contracts.Tool {
	return []contracts.Tool{
		NewTarTool(basePath),
		NewGzipTool(basePath),
		NewBzip2Tool(basePath),
		NewXzTool(basePath),
	}
}

// CatalogEntries returns declarative catalog metadata for the archive family.
func CatalogEntries() []catalog.ToolCatalogEntry {
	specs := []catalog.CommandToolSpec{
		{Name: "cli_tar", Family: "archive", Intent: []string{"compress", "decompress", "inspect"}, Description: "Creates or extracts tar archives.", Command: "tar", Tags: []string{contracts.TagExecute}},
		{Name: "cli_gzip", Family: "archive", Intent: []string{"compress"}, Description: "Compresses data with gzip.", Command: "gzip", Tags: []string{contracts.TagExecute}},
		{Name: "cli_bzip2", Family: "archive", Intent: []string{"compress"}, Description: "Compresses data using bzip2.", Command: "bzip2", Tags: []string{contracts.TagExecute}},
		{Name: "cli_xz", Family: "archive", Intent: []string{"compress"}, Description: "Compresses data with xz.", Command: "xz", Tags: []string{contracts.TagExecute}},
	}
	entries := make([]catalog.ToolCatalogEntry, 0, len(specs))
	for _, spec := range specs {
		entries = append(entries, catalog.EntryFromCommandSpec(spec))
	}
	return entries
}
