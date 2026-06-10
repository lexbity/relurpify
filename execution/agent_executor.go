package execution

import (
	"context"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

type Task struct {
	ID          string
	Type        string
	Instruction string
	Data        map[string]any
	Context     map[string]any
	Metadata    map[string]any
}

type Result struct {
	Success  bool
	Data     ResultPayload
	Error    string
	Metadata map[string]any
	NodeID   string
}

type AgentExecutor interface {
	Initialize(config *Config) error
	Execute(ctx context.Context, task *Task, env *contextdata.Envelope) (*Result, error)
}
