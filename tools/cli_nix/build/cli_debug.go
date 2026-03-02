package build

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewGDBTool creates a GDB debugger wrapper.
func NewGDBTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:         "cli_gdb",
		Description:  "GNU Debugger.",
		Command:      "gdb",
		Category:     "cli_debug",
		HITLRequired: true,
		Tags:         []string{"execute"},
	})
}

// NewValgrindTool creates a Valgrind wrapper.
func NewValgrindTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_valgrind",
		Description: "Valgrind instrumentation framework (memcheck, cachegrind, etc).",
		Command:     "valgrind",
		Category:    "cli_debug",
		Tags:        []string{"execute"},
	})
}

// NewLddTool creates an ldd wrapper.
func NewLddTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_ldd",
		Description: "Print shared object dependencies.",
		Command:     "ldd",
		Category:    "cli_debug",
		Tags:        []string{"execute"},
	})
}

// NewObjdumpTool creates an objdump wrapper.
func NewObjdumpTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_objdump",
		Description: "Display information from object files.",
		Command:     "objdump",
		Category:    "cli_debug",
		Tags:        []string{"execute"},
	})
}

// NewPerfTool creates a perf wrapper.
func NewPerfTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:         "cli_perf",
		Description:  "Performance analysis tools for Linux.",
		Command:      "perf",
		Category:     "cli_debug",
		HITLRequired: true,
		Tags:         []string{"execute"},
	})
}

// NewStraceTool creates a strace wrapper.
func NewStraceTool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:         "cli_strace",
		Description:  "Trace system calls and signals.",
		Command:      "strace",
		Category:     "cli_debug",
		HITLRequired: true,
		Tags:         []string{"execute"},
	})
}
