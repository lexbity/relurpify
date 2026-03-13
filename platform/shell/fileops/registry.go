package fileops

import (
	"github.com/lexcodex/relurpify/framework/core"
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
