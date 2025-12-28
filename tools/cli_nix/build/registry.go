package build

import (
	"github.com/lexcodex/relurpify/framework/core"
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
		NewPatchTool(basePath),
		NewDiffTool(basePath),
		NewLddTool(basePath),
		NewObjdumpTool(basePath),
		NewPerfTool(basePath),
		NewStraceTool(basePath),
	}
}
