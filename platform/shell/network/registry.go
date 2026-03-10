package network

import (
	"github.com/lexcodex/relurpify/framework/core"
)

// Tools returns networking helpers.
func Tools(basePath string) []core.Tool {
	return []core.Tool{
		NewCurlTool(basePath),
		NewWgetTool(basePath),
		NewNCTool(basePath),
		NewDigTool(basePath),
		NewNslookupTool(basePath),
		NewIPTool(basePath),
		NewSSTool(basePath),
		NewPingTool(basePath),
	}
}
