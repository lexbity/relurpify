package execution

import (
	"context"

	htnpkg "github.com/lexcodex/relurpify/agents/htn"
	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	rewoopkg "github.com/lexcodex/relurpify/agents/rewoo"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/search"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type WorkUnitExecutor interface {
	Descriptor() eucloruntime.WorkUnitExecutorDescriptor
	BuildGraph(ctx *ExecutorContext) (*graph.Graph, error)
	Execute(ctx context.Context, exec *ExecutorContext) (*core.Result, error)
}

type ExecutorContext struct {
	Task           *core.Task
	State          *core.Context
	Envelope       eucloruntime.TaskEnvelope
	Classification eucloruntime.TaskClassification
	Mode           eucloruntime.ModeResolution
	Profile        eucloruntime.ExecutionProfileSelection
	Work           eucloruntime.UnitOfWork
}

type ExecutorFactory struct {
	Model           core.LanguageModel
	Registry        *capability.Registry
	Memory          memory.MemoryStore
	Config          *core.Config
	CheckpointPath  string
	IndexManager    *ast.IndexManager
	SearchEngine    *search.SearchEngine
	Telemetry       core.Telemetry
	React           *reactpkg.ReActAgent
	EnsureReact     func() error
	RunWithWorkflow func(ctx context.Context, exec *ExecutorContext, executor graph.WorkflowExecutor) (*core.Result, error)
}

type nativeExecutor struct {
	factory     ExecutorFactory
	descriptor  eucloruntime.WorkUnitExecutorDescriptor
	pathLabel   string
	buildGraphF func(*ExecutorContext) (*graph.Graph, error)
	executeF    func(context.Context, *ExecutorContext) (*core.Result, error)
}

func (e nativeExecutor) Descriptor() eucloruntime.WorkUnitExecutorDescriptor { return e.descriptor }
func (e nativeExecutor) BuildGraph(ctx *ExecutorContext) (*graph.Graph, error) {
	if e.buildGraphF != nil {
		return e.buildGraphF(ctx)
	}
	return nil, nil
}
func (e nativeExecutor) Execute(ctx context.Context, exec *ExecutorContext) (*core.Result, error) {
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
	agent := &htnpkg.HTNAgent{Model: f.Model, Tools: f.Registry, Memory: f.Memory, Config: f.Config, PrimitiveExec: f.React, CheckpointPath: f.CheckpointPath}
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

func SelectExecutor(f ExecutorFactory, work eucloruntime.UnitOfWork) WorkUnitExecutor {
	descriptor := work.ExecutorDescriptor
	if descriptor.ExecutorID == "" {
		descriptor = eucloruntime.WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.react", Family: eucloruntime.ExecutorFamilyReact, Reason: "default managed react executor"}
	}
	switch descriptor.Family {
	case eucloruntime.ExecutorFamilyPlanner:
		return nativeExecutor{factory: f, descriptor: descriptor, pathLabel: "planner_executor", buildGraphF: func(ctx *ExecutorContext) (*graph.Graph, error) { return newPlannerAgent(f).BuildGraph(ctx.Task) }, executeF: func(ctx context.Context, exec *ExecutorContext) (*core.Result, error) {
			return f.RunWithWorkflow(ctx, exec, newPlannerAgent(f))
		}}
	case eucloruntime.ExecutorFamilyHTN:
		return nativeExecutor{factory: f, descriptor: descriptor, pathLabel: "htn_executor", buildGraphF: func(ctx *ExecutorContext) (*graph.Graph, error) {
			agent, err := newHTNAgent(f)
			if err != nil {
				return nil, err
			}
			return agent.BuildGraph(ctx.Task)
		}, executeF: func(ctx context.Context, exec *ExecutorContext) (*core.Result, error) {
			agent, err := newHTNAgent(f)
			if err != nil {
				return nil, err
			}
			return f.RunWithWorkflow(ctx, exec, agent)
		}}
	case eucloruntime.ExecutorFamilyRewoo:
		return nativeExecutor{factory: f, descriptor: descriptor, pathLabel: "rewoo_executor", buildGraphF: func(ctx *ExecutorContext) (*graph.Graph, error) { return newRewooAgent(f).BuildGraph(ctx.Task) }, executeF: func(ctx context.Context, exec *ExecutorContext) (*core.Result, error) {
			return f.RunWithWorkflow(ctx, exec, newRewooAgent(f))
		}}
	case eucloruntime.ExecutorFamilyReflection:
		return nativeExecutor{factory: f, descriptor: descriptor, pathLabel: "reflection_executor", buildGraphF: func(ctx *ExecutorContext) (*graph.Graph, error) {
			agent, err := newReflectionAgent(f)
			if err != nil {
				return nil, err
			}
			return agent.BuildGraph(ctx.Task)
		}, executeF: func(ctx context.Context, exec *ExecutorContext) (*core.Result, error) {
			agent, err := newReflectionAgent(f)
			if err != nil {
				return nil, err
			}
			return f.RunWithWorkflow(ctx, exec, agent)
		}}
	default:
		return nativeExecutor{factory: f, descriptor: descriptor, pathLabel: "react_executor", buildGraphF: func(ctx *ExecutorContext) (*graph.Graph, error) {
			if f.EnsureReact != nil {
				if err := f.EnsureReact(); err != nil {
					return nil, err
				}
			}
			return f.React.BuildGraph(ctx.Task)
		}, executeF: func(ctx context.Context, exec *ExecutorContext) (*core.Result, error) {
			if f.EnsureReact != nil {
				if err := f.EnsureReact(); err != nil {
					return nil, err
				}
			}
			return f.RunWithWorkflow(ctx, exec, f.React)
		}}
	}
}
