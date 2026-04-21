package execution

import (
	"context"

	htnpkg "codeburg.org/lexbit/relurpify/agents/htn"
	plannerpkg "codeburg.org/lexbit/relurpify/agents/planner"
	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	reflectionpkg "codeburg.org/lexbit/relurpify/agents/reflection"
	rewoopkg "codeburg.org/lexbit/relurpify/agents/rewoo"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	blackboardexec "codeburg.org/lexbit/relurpify/named/euclo/execution/blackboard"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

type Selection struct {
	Descriptor eucloruntime.WorkUnitExecutorDescriptor
	Runtime    eucloruntime.ExecutorRuntimeState
	Workflow   graph.WorkflowExecutor
}

type ExecutorFactory struct {
	Model          core.LanguageModel
	Registry       *capability.Registry
	Memory         memory.MemoryStore
	Config         *core.Config
	CheckpointPath string
	IndexManager   *ast.IndexManager
	SearchEngine   *search.SearchEngine
	Telemetry      core.Telemetry
	React          *reactpkg.ReActAgent
	EnsureReact    func() error

	// SemanticContext is the full Euclo-owned pre-resolved semantic context
	// bundle. The embedded AgentSemanticContext is propagated to all agents
	// constructed by this factory.
	SemanticContext euclotypes.ExecutorSemanticContext
}

func newPlannerAgent(f ExecutorFactory) *plannerpkg.PlannerAgent {
	agent := &plannerpkg.PlannerAgent{Model: f.Model, Tools: f.Registry, Memory: f.Memory, Config: f.Config, CheckpointPath: f.CheckpointPath}
	_ = agent.Initialize(f.Config)
	return agent
}

func newHTNAgent(f ExecutorFactory) (*htnpkg.HTNAgent, error) {
	if f.EnsureReact != nil {
		if err := f.EnsureReact(); err != nil {
			return nil, err
		}
	}
	agent := &htnpkg.HTNAgent{
		Model:           f.Model,
		Tools:           f.Registry,
		Memory:          f.Memory,
		Config:          f.Config,
		PrimitiveExec:   f.React,
		CheckpointPath:  f.CheckpointPath,
		SemanticContext: f.SemanticContext.AgentSemanticContext,
	}
	_ = agent.Initialize(f.Config)
	return agent, nil
}

func newRewooAgent(f ExecutorFactory) *rewoopkg.RewooAgent {
	agent := &rewoopkg.RewooAgent{Model: f.Model, Tools: f.Registry, Memory: f.Memory, Config: f.Config, IndexManager: f.IndexManager, SearchEngine: f.SearchEngine, Telemetry: f.Telemetry}
	_ = agent.Initialize(f.Config)
	return agent
}

func newReflectionAgent(f ExecutorFactory) (*reflectionpkg.ReflectionAgent, error) {
	if f.EnsureReact != nil {
		if err := f.EnsureReact(); err != nil {
			return nil, err
		}
	}
	agent := &reflectionpkg.ReflectionAgent{Reviewer: f.Model, Delegate: f.React, Config: f.Config}
	_ = agent.Initialize(f.Config)
	return agent, nil
}

func SelectExecutor(f ExecutorFactory, work eucloruntime.UnitOfWork) (Selection, error) {
	descriptor := work.ExecutorDescriptor
	if descriptor.ExecutorID == "" {
		descriptor = eucloruntime.WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.react", Family: eucloruntime.ExecutorFamilyReact, Reason: "default managed react executor"}
	}
	buildSelection := func(path string, workflow graph.WorkflowExecutor) (Selection, error) {
		if path == "" {
			path = "workflow_executor"
		}
		return Selection{
			Descriptor: descriptor,
			Runtime: eucloruntime.ExecutorRuntimeState{
				ExecutorID: descriptor.ExecutorID,
				Family:     descriptor.Family,
				Path:       path,
				Reason:     descriptor.Reason,
			},
			Workflow: workflow,
		}, nil
	}
	switch descriptor.Family {
	case eucloruntime.ExecutorFamilyPlanner:
		return buildSelection("planner_executor", newPlannerAgent(f))
	case eucloruntime.ExecutorFamilyHTN:
		agent, err := newHTNAgent(f)
		if err != nil {
			return Selection{}, err
		}
		return buildSelection("htn_executor", agent)
	case eucloruntime.ExecutorFamilyRewoo:
		return buildSelection("rewoo_executor", newRewooAgent(f))
	case eucloruntime.ExecutorFamilyReflection:
		agent, err := newReflectionAgent(f)
		if err != nil {
			return Selection{}, err
		}
		return buildSelection("reflection_executor", agent)
	// Blackboard executor for debug workflows with shared runtime state
	case eucloruntime.ExecutorFamilyBlackboard:
		agent := newBlackboardExecutor(f)
		return buildSelection("blackboard_executor", agent)
	default:
		if f.EnsureReact != nil {
			if err := f.EnsureReact(); err != nil {
				return Selection{}, err
			}
		}
		return buildSelection("react_executor", f.React)
	}
}

// blackboardExecutorWrapper adapts the blackboard execution package to graph.WorkflowExecutor interface
type blackboardExecutorWrapper struct {
	factory ExecutorFactory
	config  *core.Config
	semctx  euclotypes.ExecutorSemanticContext
}

func (w *blackboardExecutorWrapper) Initialize(cfg *core.Config) error {
	w.config = cfg
	return nil
}

func (w *blackboardExecutorWrapper) Capabilities() []core.Capability {
	return []core.Capability{core.CapabilityPlan, core.CapabilityExecute, core.CapabilityCode}
}

func (w *blackboardExecutorWrapper) BuildGraph(task *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("blackboard_done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	if err := g.SetStart("blackboard_done"); err != nil {
		return nil, err
	}
	return g, nil
}

func (w *blackboardExecutorWrapper) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	// Create execution envelope from task and state
	env := euclotypes.ExecutionEnvelope{
		Task:     task,
		State:    state,
		Registry: w.factory.Registry,
		Environment: agentenv.AgentEnvironment{
			Model:        w.factory.Model,
			Registry:     w.factory.Registry,
			Memory:       w.factory.Memory,
			Config:       w.factory.Config,
			IndexManager: w.factory.IndexManager,
			SearchEngine: w.factory.SearchEngine,
		},
	}

	// Use debug-specific KnowledgeSources for hypothesis-driven debugging
	// These KS share workspace context via the blackboard to avoid redundant
	// file exploration (the HTN context isolation bug).
	sources := blackboardexec.DebugKnowledgeSources()
	maxCycles := 10
	terminationPredicate := blackboardexec.DebugTerminationPredicate

	result, err := blackboardexec.Execute(ctx, env, w.semctx, sources, maxCycles, terminationPredicate)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}

	return &core.Result{
		Success: true,
		Data: map[string]any{
			"cycles":      result.Cycles,
			"termination": result.Termination,
			"artifacts":   len(result.Artifacts),
		},
	}, nil
}

func newBlackboardExecutor(f ExecutorFactory) *blackboardExecutorWrapper {
	return &blackboardExecutorWrapper{factory: f, semctx: f.SemanticContext}
}
