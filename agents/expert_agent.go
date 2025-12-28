package agents

import (
	"context"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/toolsys"
)

// ExpertCoderAgent chains the architect planner with the coding delegate,
// mirroring the pipeline pattern from the specification.
//
// Deprecated: This logic has been consolidated into AgentCoordinator.
// This struct now acts as a pre-configured wrapper around AgentCoordinator.
type ExpertCoderAgent struct {
	Model  core.LanguageModel
	Tools  *toolsys.ToolRegistry
	Memory memory.MemoryStore
	Config *core.Config

	coordinator *AgentCoordinator
}

// Initialize configures the planner and coding delegates.
func (a *ExpertCoderAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = toolsys.NewToolRegistry()
	}

	planner := &PlannerAgent{Model: a.Model, Tools: a.Tools, Memory: a.Memory}
	if err := planner.Initialize(cfg); err != nil {
		return err
	}

	coder := &CodingAgent{Model: a.Model, Tools: a.Tools, Memory: a.Memory}
	if err := coder.Initialize(cfg); err != nil {
		return err
	}

	// Initialize coordinator with a default budget
	a.coordinator = NewAgentCoordinator(cfg.Telemetry, core.NewContextBudget(16000))
	a.coordinator.RegisterAgent("planner", planner)
	a.coordinator.RegisterAgent("executor", coder)

	// Register an 'ask' agent for self-healing diagnostics if available
	asker := &ReActAgent{
		Model:  a.Model,
		Tools:  a.Tools,
		Memory: a.Memory,
		Mode:   "ask",
	}
	if err := asker.Initialize(cfg); err == nil {
		a.coordinator.RegisterAgent("ask", asker)
	}

	return nil
}

// Capabilities merges planning and coding skills.
func (a *ExpertCoderAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityCode,
		core.CapabilityReview,
		core.CapabilityExplain,
	}
}

// BuildGraph constructs a pipeline graph.
func (a *ExpertCoderAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	graph := graph.NewGraph()
	// We wrap the coordinator in a single system node
	node := &expertCoordinatorNode{
		id:    "expert_coordination",
		agent: a,
		task:  task,
	}

	if err := graph.AddNode(node); err != nil {
		return nil, err
	}
	if err := graph.SetStart(node.ID()); err != nil {
		return nil, err
	}
	return graph, nil
}

// Execute runs plan then coding mode.
func (a *ExpertCoderAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if task.Metadata == nil {
		task.Metadata = make(map[string]string)
	}
	// Force plan_execute strategy to maintain backward compatibility behavior
	task.Metadata["strategy"] = "plan_execute"
	return a.coordinator.Execute(ctx, task, state)
}

type expertCoordinatorNode struct {
	id    string
	agent *ExpertCoderAgent
	task  *core.Task
}

func (n *expertCoordinatorNode) ID() string           { return n.id }
func (n *expertCoordinatorNode) Type() graph.NodeType { return graph.NodeTypeSystem }
func (n *expertCoordinatorNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	return n.agent.Execute(ctx, n.task, state)
}
