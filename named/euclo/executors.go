package euclo

import (
	"context"

	htnpkg "github.com/lexcodex/relurpify/agents/htn"
	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	rewoopkg "github.com/lexcodex/relurpify/agents/rewoo"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type workUnitExecutor interface {
	Descriptor() eucloruntime.WorkUnitExecutorDescriptor
	BuildGraph(ctx *executorContext) (*graph.Graph, error)
	Execute(ctx context.Context, exec *executorContext) (*core.Result, error)
}

type executorContext struct {
	Task           *core.Task
	State          *core.Context
	Envelope       eucloruntime.TaskEnvelope
	Classification eucloruntime.TaskClassification
	Mode           eucloruntime.ModeResolution
	Profile        eucloruntime.ExecutionProfileSelection
	Work           eucloruntime.UnitOfWork
}

type nativeExecutor struct {
	agent       *Agent
	descriptor  eucloruntime.WorkUnitExecutorDescriptor
	pathLabel   string
	buildGraphF func(*executorContext) (*graph.Graph, error)
	executeF    func(context.Context, *executorContext) (*core.Result, error)
}

func (e nativeExecutor) Descriptor() eucloruntime.WorkUnitExecutorDescriptor {
	return e.descriptor
}

func (e nativeExecutor) BuildGraph(ctx *executorContext) (*graph.Graph, error) {
	if e.buildGraphF != nil {
		return e.buildGraphF(ctx)
	}
	return nil, nil
}

func (e nativeExecutor) Execute(ctx context.Context, exec *executorContext) (*core.Result, error) {
	if exec != nil && exec.State != nil {
		path := e.pathLabel
		if path == "" {
			path = "workflow_executor"
		}
		exec.State.Set("euclo.executor_runtime", eucloruntime.ExecutorRuntimeState{
			ExecutorID: e.descriptor.ExecutorID,
			Family:     e.descriptor.Family,
			Path:       path,
			Reason:     e.descriptor.Reason,
		})
	}
	return e.executeF(ctx, exec)
}

func (a *Agent) plannerExecutorAgent() *plannerpkg.PlannerAgent {
	agent := &plannerpkg.PlannerAgent{
		Model:          a.Environment.Model,
		Tools:          a.CapabilityRegistry(),
		Memory:         a.Memory,
		Config:         a.Config,
		CheckpointPath: a.CheckpointPath,
	}
	_ = agent.Initialize(a.Config)
	return agent
}

func (a *Agent) htnExecutorAgent() (*htnpkg.HTNAgent, error) {
	if err := a.ensureReactDelegate(); err != nil {
		return nil, err
	}
	agent := &htnpkg.HTNAgent{
		Model:          a.Environment.Model,
		Tools:          a.CapabilityRegistry(),
		Memory:         a.Memory,
		Config:         a.Config,
		PrimitiveExec:  a.Delegate,
		CheckpointPath: a.CheckpointPath,
	}
	_ = agent.Initialize(a.Config)
	return agent, nil
}

func (a *Agent) rewooExecutorAgent() *rewoopkg.RewooAgent {
	agent := &rewoopkg.RewooAgent{
		Model:        a.Environment.Model,
		Tools:        a.CapabilityRegistry(),
		Memory:       a.Memory,
		Config:       a.Config,
		IndexManager: a.Environment.IndexManager,
		SearchEngine: a.Environment.SearchEngine,
		Telemetry:    a.ConfigTelemetry(),
	}
	_ = agent.Initialize(a.Config)
	return agent
}

func (a *Agent) reflectionExecutorAgent() (*reflectionpkg.ReflectionAgent, error) {
	if err := a.ensureReactDelegate(); err != nil {
		return nil, err
	}
	agent := &reflectionpkg.ReflectionAgent{
		Reviewer: a.Environment.Model,
		Delegate: a.Delegate,
		Config:   a.Config,
	}
	_ = agent.Initialize(a.Config)
	return agent, nil
}

func (a *Agent) selectExecutor(work eucloruntime.UnitOfWork) workUnitExecutor {
	descriptor := work.ExecutorDescriptor
	if descriptor.ExecutorID == "" {
		descriptor = eucloruntime.WorkUnitExecutorDescriptor{
			ExecutorID: "euclo.executor.react",
			Family:     eucloruntime.ExecutorFamilyReact,
			Reason:     "default managed react executor",
		}
	}
	switch descriptor.Family {
	case eucloruntime.ExecutorFamilyPlanner:
		return nativeExecutor{
			agent:      a,
			descriptor: descriptor,
			pathLabel:  "planner_executor",
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				return a.plannerExecutorAgent().BuildGraph(ctx.Task)
			},
			executeF: func(ctx context.Context, exec *executorContext) (*core.Result, error) {
				return a.executeWithWorkflowExecutor(ctx, exec, a.plannerExecutorAgent())
			},
		}
	case eucloruntime.ExecutorFamilyHTN:
		return nativeExecutor{
			agent:      a,
			descriptor: descriptor,
			pathLabel:  "htn_executor",
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				agent, err := a.htnExecutorAgent()
				if err != nil {
					return nil, err
				}
				return agent.BuildGraph(ctx.Task)
			},
			executeF: func(ctx context.Context, exec *executorContext) (*core.Result, error) {
				agent, err := a.htnExecutorAgent()
				if err != nil {
					return nil, err
				}
				return a.executeWithWorkflowExecutor(ctx, exec, agent)
			},
		}
	case eucloruntime.ExecutorFamilyRewoo:
		return nativeExecutor{
			agent:      a,
			descriptor: descriptor,
			pathLabel:  "rewoo_executor",
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				return a.rewooExecutorAgent().BuildGraph(ctx.Task)
			},
			executeF: func(ctx context.Context, exec *executorContext) (*core.Result, error) {
				return a.executeWithWorkflowExecutor(ctx, exec, a.rewooExecutorAgent())
			},
		}
	case eucloruntime.ExecutorFamilyReflection:
		return nativeExecutor{
			agent:      a,
			descriptor: descriptor,
			pathLabel:  "reflection_executor",
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				agent, err := a.reflectionExecutorAgent()
				if err != nil {
					return nil, err
				}
				return agent.BuildGraph(ctx.Task)
			},
			executeF: func(ctx context.Context, exec *executorContext) (*core.Result, error) {
				agent, err := a.reflectionExecutorAgent()
				if err != nil {
					return nil, err
				}
				return a.executeWithWorkflowExecutor(ctx, exec, agent)
			},
		}
	default:
		return nativeExecutor{
			agent:      a,
			descriptor: descriptor,
			pathLabel:  "react_executor",
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				if err := a.ensureReactDelegate(); err != nil {
					return nil, err
				}
				return a.Delegate.BuildGraph(a.eucloTask(ctx.Task, ctx.Envelope, ctx.Classification, ctx.Mode, ctx.Profile, ctx.Work))
			},
			executeF: func(ctx context.Context, exec *executorContext) (*core.Result, error) {
				if err := a.ensureReactDelegate(); err != nil {
					return nil, err
				}
				return a.executeWithWorkflowExecutor(ctx, exec, a.Delegate)
			},
		}
	}
}
