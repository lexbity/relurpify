package execution

import (
	"context"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

type Task struct {
	ID          string
	Type        string
	Instruction string
	Data        map[string]interface{}
	Context     map[string]interface{}
	Metadata    map[string]interface{}
}

type Result struct {
	Success  bool
	Data     ResultPayload
	Error    string
	Metadata map[string]interface{}
	NodeID   string
}

type AgentExecutor interface {
	Initialize(config *Config) error
	Execute(ctx context.Context, task *Task, env *contextdata.Envelope) (*Result, error)
}
