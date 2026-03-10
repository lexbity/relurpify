package archive

import (
	"github.com/lexcodex/relurpify/framework/core"
)

// Tools returns archiving/compression helpers.
func Tools(basePath string) []core.Tool {
	return []core.Tool{
		NewTarTool(basePath),
		NewGzipTool(basePath),
		NewBzip2Tool(basePath),
		NewXzTool(basePath),
	}
}
