package build

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/shell/catalog"
)

// Tools returns build-system helpers.
func Tools(basePath string) []core.Tool {
	return []core.Tool{
		NewMakeTool(basePath),
		NewCMakeTool(basePath),
		NewCargoTool(basePath),
		NewGoTool(basePath),
		NewPythonTool(basePath),
		NewNodeTool(basePath),
		NewNPMTool(basePath),
		NewSQLite3Tool(basePath),
		NewRustfmtTool(basePath),
		NewPkgConfigTool(basePath),
		NewGDBTool(basePath),
		NewValgrindTool(basePath),
		NewLddTool(basePath),
		NewObjdumpTool(basePath),
		NewPerfTool(basePath),
		NewStraceTool(basePath),
	}
}

// CatalogEntries returns declarative catalog metadata for the build family.
func CatalogEntries() []catalog.ToolCatalogEntry {
	specs := []catalog.CommandToolSpec{
		{Name: "cli_make", Family: "build", Intent: []string{"build", "verify"}, Description: "Runs make targets for builds.", Command: "make", Tags: []string{core.TagExecute}},
		{Name: "cli_cmake", Family: "build", Intent: []string{"build", "configure"}, Description: "Configures builds with cmake.", Command: "cmake", Tags: []string{core.TagExecute}},
		{Name: "cli_cargo", Family: "build", Intent: []string{"build", "rust"}, Description: "Executes Rust cargo commands inside the workspace.", Command: "cargo", Tags: []string{core.TagExecute}},
		{Name: "cli_go", Family: "build", Intent: []string{"build", "go"}, Description: "Executes Go commands (go test/build/etc) inside the workspace.", Command: "go", Tags: []string{core.TagExecute}},
		{Name: "cli_python", Family: "build", Intent: []string{"build", "python"}, Description: "Executes Python commands using python3 inside the workspace.", Command: "python3", Tags: []string{core.TagExecute}},
		{Name: "cli_node", Family: "build", Intent: []string{"build", "node"}, Description: "Executes Node.js commands inside the workspace.", Command: "node", Tags: []string{core.TagExecute}},
		{Name: "cli_npm", Family: "build", Intent: []string{"build", "node"}, Description: "Executes npm commands inside the workspace.", Command: "npm", Tags: []string{core.TagExecute}},
		{Name: "cli_sqlite3", Family: "build", Intent: []string{"build", "sql"}, Description: "Executes SQLite commands using sqlite3 inside the workspace.", Command: "sqlite3", Tags: []string{core.TagExecute}},
		{Name: "cli_rustfmt", Family: "build", Intent: []string{"format", "rust"}, Description: "Formats Rust code using rustfmt.", Command: "rustfmt", Tags: []string{core.TagExecute}},
		{Name: "cli_pkg_config", Family: "build", Intent: []string{"inspect", "native"}, Description: "Queries compiler flags with pkg-config.", Command: "pkg-config", Tags: []string{core.TagExecute}},
		{Name: "cli_gdb", Family: "build", Intent: []string{"debug"}, Description: "GNU Debugger.", Command: "gdb", Tags: []string{core.TagExecute, core.TagDestructive}},
		{Name: "cli_valgrind", Family: "build", Intent: []string{"debug"}, Description: "Valgrind instrumentation framework (memcheck, cachegrind, etc).", Command: "valgrind", Tags: []string{core.TagExecute}},
		{Name: "cli_ldd", Family: "build", Intent: []string{"inspect", "native"}, Description: "Print shared object dependencies.", Command: "ldd", Tags: []string{core.TagExecute}},
		{Name: "cli_objdump", Family: "build", Intent: []string{"inspect", "native"}, Description: "Display information from object files.", Command: "objdump", Tags: []string{core.TagExecute}},
		{Name: "cli_perf", Family: "build", Intent: []string{"profile"}, Description: "Performance analysis tools for Linux.", Command: "perf", Tags: []string{core.TagExecute, core.TagDestructive}},
		{Name: "cli_strace", Family: "build", Intent: []string{"profile", "diagnostics"}, Description: "Trace system calls and signals.", Command: "strace", Tags: []string{core.TagExecute, core.TagDestructive}},
	}
	entries := make([]catalog.ToolCatalogEntry, 0, len(specs))
	for _, spec := range specs {
		entries = append(entries, catalog.EntryFromCommandSpec(spec))
	}
	return entries
}
