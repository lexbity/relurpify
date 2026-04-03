package execution

import (
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
	default:
		if f.EnsureReact != nil {
			if err := f.EnsureReact(); err != nil {
				return Selection{}, err
			}
		}
		return buildSelection("react_executor", f.React)
	}
}
