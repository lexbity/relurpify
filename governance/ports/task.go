package ports

import "context"

// TaskView is the governance-owned view of an execution task.
type TaskView interface {
	ID() string
	Type() string
	Instruction() string
}

// ResultView is the governance-owned view of an execution result.
type ResultView interface {
	Success() bool
	Data() map[string]any
}

// AgentView is the governance-owned view of an agent executor.
// Execution/agentlifecycle implements it.
type AgentView interface {
	Initialize(cfg AgentConfigView) error
	Execute(ctx context.Context, task TaskView, state StateView) (ResultView, error)
}

// AgentConfigView is the governance-owned config for agent initialization.
type AgentConfigView struct {
	Name              string
	NativeToolCalling bool
}

// TaskContextExtractor extracts a TaskView from a Go context.Context.
// execution/agentlifecycle implements this.
type TaskContextExtractor interface {
	FromContext(ctx context.Context) (TaskView, bool)
}
