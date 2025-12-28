package build

import (
	"github.com/lexcodex/relurpify/framework/core"
	clinix "github.com/lexcodex/relurpify/tools/cli_nix"
)

// NewSQLite3Tool exposes sqlite3 for running SQL scripts inside the workspace.
func NewSQLite3Tool(basePath string) core.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_sqlite3",
		Description: "Executes SQLite commands using sqlite3 inside the workspace.",
		Command:     "sqlite3",
		Category:    "cli_build",
	})
}
