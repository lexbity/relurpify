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

type compatibilityExecutor struct {
	agent       *Agent
	descriptor  eucloruntime.WorkUnitExecutorDescriptor
	buildGraphF func(*executorContext) (*graph.Graph, error)
}

type nativeExecutor struct {
	agent       *Agent
	descriptor  eucloruntime.WorkUnitExecutorDescriptor
	buildGraphF func(*executorContext) (*graph.Graph, error)
	executeF    func(context.Context, *executorContext) (*core.Result, error)
}

func (e compatibilityExecutor) Descriptor() eucloruntime.WorkUnitExecutorDescriptor {
	return e.descriptor
}

func (e compatibilityExecutor) BuildGraph(ctx *executorContext) (*graph.Graph, error) {
	if e.buildGraphF != nil {
		return e.buildGraphF(ctx)
	}
	if err := e.agent.ensureReactDelegate(); err != nil {
		return nil, err
	}
	return e.agent.Delegate.BuildGraph(e.agent.eucloTask(ctx.Task, ctx.Envelope, ctx.Classification, ctx.Mode, ctx.Profile, ctx.Work))
}

func (e compatibilityExecutor) Execute(ctx context.Context, exec *executorContext) (*core.Result, error) {
	return e.agent.executeWithCurrentFlow(ctx, exec.Task, exec.State)
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
	return e.executeF(ctx, exec)
}

func (a *Agent) selectExecutor(work eucloruntime.UnitOfWork) workUnitExecutor {
	descriptor := work.ExecutorDescriptor
	if descriptor.ExecutorID == "" {
		descriptor = eucloruntime.WorkUnitExecutorDescriptor{
			ExecutorID:    "euclo.executor.react",
			Family:        eucloruntime.ExecutorFamilyReact,
			Reason:        "default compatibility executor",
			Compatibility: true,
		}
	}
	switch descriptor.Family {
	case eucloruntime.ExecutorFamilyPlanner:
		return compatibilityExecutor{
			agent:      a,
			descriptor: descriptor,
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				agent := &plannerpkg.PlannerAgent{Model: a.Environment.Model, Tools: a.CapabilityRegistry(), Memory: a.Memory, Config: a.Config, CheckpointPath: a.CheckpointPath}
				_ = agent.Initialize(a.Config)
				return agent.BuildGraph(ctx.Task)
			},
		}
	case eucloruntime.ExecutorFamilyHTN:
		return compatibilityExecutor{
			agent:      a,
			descriptor: descriptor,
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				if err := a.ensureReactDelegate(); err != nil {
					return nil, err
				}
				agent := &htnpkg.HTNAgent{Model: a.Environment.Model, Tools: a.CapabilityRegistry(), Memory: a.Memory, Config: a.Config, PrimitiveExec: a.Delegate, CheckpointPath: a.CheckpointPath}
				_ = agent.Initialize(a.Config)
				return agent.BuildGraph(ctx.Task)
			},
		}
	case eucloruntime.ExecutorFamilyRewoo:
		return compatibilityExecutor{
			agent:      a,
			descriptor: descriptor,
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				agent := &rewoopkg.RewooAgent{Model: a.Environment.Model, Tools: a.CapabilityRegistry(), Memory: a.Memory, Config: a.Config, IndexManager: a.Environment.IndexManager, SearchEngine: a.Environment.SearchEngine, Telemetry: a.ConfigTelemetry()}
				_ = agent.Initialize(a.Config)
				return agent.BuildGraph(ctx.Task)
			},
		}
	case eucloruntime.ExecutorFamilyReflection:
		return nativeExecutor{
			agent:      a,
			descriptor: descriptor,
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				if err := a.ensureReactDelegate(); err != nil {
					return nil, err
				}
				agent := &reflectionpkg.ReflectionAgent{Reviewer: a.Environment.Model, Delegate: a.Delegate, Config: a.Config}
				_ = agent.Initialize(a.Config)
				return agent.BuildGraph(ctx.Task)
			},
			executeF: func(ctx context.Context, exec *executorContext) (*core.Result, error) {
				if err := a.ensureReactDelegate(); err != nil {
					return nil, err
				}
				agent := &reflectionpkg.ReflectionAgent{Reviewer: a.Environment.Model, Delegate: a.Delegate, Config: a.Config}
				return a.executeWithWorkflowExecutor(ctx, exec, agent)
			},
		}
	default:
		return compatibilityExecutor{
			agent:      a,
			descriptor: descriptor,
			buildGraphF: func(ctx *executorContext) (*graph.Graph, error) {
				if err := a.ensureReactDelegate(); err != nil {
					return nil, err
				}
				return a.Delegate.BuildGraph(a.eucloTask(ctx.Task, ctx.Envelope, ctx.Classification, ctx.Mode, ctx.Profile, ctx.Work))
			},
		}
	}
}
