package scheduler

import (
	"github.com/lexcodex/relurpify/framework/core"
)

// Tools returns scheduling helpers.
func Tools(basePath string) []core.Tool {
	return []core.Tool{
		NewCrontabTool(basePath),
		NewAtTool(basePath),
	}
}
