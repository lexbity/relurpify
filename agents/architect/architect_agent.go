package architect

import (
	"context"
	"errors"
	"fmt"

	"codeburg.org/lexbit/relurpify/agents/planner"
	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworksearch "codeburg.org/lexbit/relurpify/framework/search"
)

// ArchitectAgent uses a small-model-friendly workflow:
// 1. Generate an explicit plan once.
// 2. Execute one plan step at a time with a fresh, compact ReAct context.
// 3. Persist workflow state after planning and each completed step.
type ArchitectAgent struct {
	Model             core.LanguageModel
	PlannerTools      *capability.Registry
	ExecutorTools     *capability.Registry
	Memory            memory.MemoryStore
	Config            *core.Config
	IndexManager      *ast.IndexManager
	SearchEngine      *frameworksearch.SearchEngine
	CheckpointPath    string
	WorkflowStatePath string

	planner  *planner.PlannerAgent
	executor *reactpkg.ReActAgent
	planning *WorkflowPlanningService
}

var errArchitectNeedsReplan = errors.New("architect workflow requires replanning")

const ModeArchitect = "architect"

func (a *ArchitectAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.PlannerTools == nil {
		a.PlannerTools = capability.NewRegistry()
	}
	if a.ExecutorTools == nil {
		a.ExecutorTools = capability.NewRegistry()
	}
	a.planner = &planner.PlannerAgent{
		Model:  a.Model,
		Tools:  a.PlannerTools,
		Memory: a.Memory,
	}
	if err := a.planner.Initialize(cfg); err != nil {
		return err
	}
	a.executor = &reactpkg.ReActAgent{
		Model:          a.Model,
		Tools:          a.ExecutorTools,
		Memory:         a.Memory,
		Config:         cfg,
		IndexManager:   a.IndexManager,
		SearchEngine:   a.SearchEngine,
		Summarizer:     &core.SimpleSummarizer{},
		CheckpointPath: a.CheckpointPath,
		Mode:           "code",
	}
	a.planning = &WorkflowPlanningService{
		Model:        a.Model,
		Planner:      a.planner,
		PlannerTools: a.PlannerTools,
		Config:       cfg,
	}
	return a.executor.Initialize(cfg)
}

func (a *ArchitectAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityCode,
		core.CapabilityExplain,
	}
}

func (a *ArchitectAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.planner == nil {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	return a.planner.BuildGraph(task)
}

func (a *ArchitectAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	if state == nil {
		state = core.NewContext()
	}
	if a.planner == nil || a.executor == nil {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}

	state.Set("architect.mode", "plan_execute")
	state.Set("task.id", task.ID)
	state.Set("task.type", string(task.Type))
	state.Set("task.instruction", task.Instruction)

	store, err := a.openWorkflowStateStore()
	if err != nil {
		return nil, err
	}
	if store != nil {
		defer store.Close()
		return a.executeWithWorkflowStore(ctx, task, state, store)
	}
	return a.executeLegacyPlan(ctx, task, state)
}
