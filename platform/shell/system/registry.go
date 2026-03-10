package system

import (
	"github.com/lexcodex/relurpify/framework/core"
)

// Tools returns system inspection helpers.
func Tools(basePath string) []core.Tool {
	return []core.Tool{
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
