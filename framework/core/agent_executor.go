package core

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// Task represents a unit of work to be executed by an agent.
type Task struct {
	ID          string
	Type        string
	Instruction string
	Data        map[string]interface{}
	Context     map[string]interface{} // Execution context/state
	Metadata    map[string]interface{} // Task metadata
}

// Result captures the outcome of a task execution.
type Result struct {
	Success  bool
	Data     map[string]interface{}
	Error    string
	Metadata map[string]interface{} // Result metadata
	NodeID   string                 // Node identifier
}

// AgentExecutor is the runtime-facing interface for agents that can be
// initialized and executed by the authorization subsystem. This interface
// decouples the authorization package from agentgraph implementation details.
type AgentExecutor interface {
	Initialize(config *Config) error
	Execute(ctx context.Context, task *Task, env *contextdata.Envelope) (*Result, error)
}
