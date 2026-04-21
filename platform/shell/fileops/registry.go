package fileops

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/shell/catalog"
)

// Tools returns file navigation/search helpers.
func Tools(basePath string) []core.Tool {
	return []core.Tool{
		NewGitTool(basePath),
		NewFindTool(basePath),
		NewFDTool(basePath),
		NewRGTool(basePath),
		NewAGTool(basePath),
		NewLocateTool(basePath),
		NewTreeTool(basePath),
		NewStatTool(basePath),
		NewFileTool(basePath),
		NewTouchTool(basePath),
		NewMkdirTool(basePath),
	}
}

// CatalogEntries returns declarative catalog metadata for the fileops family.
func CatalogEntries() []catalog.ToolCatalogEntry {
	specs := []catalog.CommandToolSpec{
		{Name: "cli_git", Family: "fileops", Intent: []string{"inspect", "repository"}, Description: "Runs git with the provided arguments.", Command: "git", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_find", Family: "fileops", Intent: []string{"locate", "browse"}, Description: "Searches the filesystem using find.", Command: "find", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_fd", Family: "fileops", Intent: []string{"locate", "browse"}, Description: "Performs fast file searches with fd.", Command: "fd", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_rg", Family: "fileops", Intent: []string{"search"}, Description: "Uses ripgrep (rg) for recursive code search.", Command: "rg", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_ag", Family: "fileops", Intent: []string{"search"}, Description: "Searches codebases with the silver searcher (ag).", Command: "ag", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_locate", Family: "fileops", Intent: []string{"locate"}, Description: "Queries the file database via locate.", Command: "locate", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_tree", Family: "fileops", Intent: []string{"browse"}, Description: "Displays directory trees using tree.", Command: "tree", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_stat", Family: "fileops", Intent: []string{"inspect", "metadata"}, Description: "Shows file metadata with stat.", Command: "stat", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_file", Family: "fileops", Intent: []string{"inspect", "metadata"}, Description: "Detects file types using the file command.", Command: "file", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_touch", Family: "fileops", Intent: []string{"create"}, Description: "Creates empty files or updates timestamps via touch.", Command: "touch", Tags: []string{core.TagExecute, core.TagReadOnly}},
		{Name: "cli_mkdir", Family: "fileops", Intent: []string{"create"}, Description: "Creates directories via mkdir (defaults to -p).", Command: "mkdir", DefaultArgs: []string{"-p"}, Tags: []string{core.TagExecute, core.TagReadOnly}},
	}
	entries := make([]catalog.ToolCatalogEntry, 0, len(specs))
	for _, spec := range specs {
		entries = append(entries, catalog.EntryFromCommandSpec(spec))
	}
	return entries
}
