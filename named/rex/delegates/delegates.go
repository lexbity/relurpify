package delegates

import (
	"context"
	"fmt"

	architectpkg "github.com/lexcodex/relurpify/agents/architect"
	blackboardpkg "github.com/lexcodex/relurpify/agents/blackboard"
	goalconpkg "github.com/lexcodex/relurpify/agents/goalcon"
	htnpkg "github.com/lexcodex/relurpify/agents/htn"
	pipelinepkg "github.com/lexcodex/relurpify/agents/pipeline"
	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	rewoopkg "github.com/lexcodex/relurpify/agents/rewoo"
	chainerpkg "github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	rexroute "github.com/lexcodex/relurpify/named/rex/route"
)

// Delegate executes rex work for a specific orchestration family.
type Delegate interface {
	Family() string
	BuildGraph(task *core.Task) (*graph.Graph, error)
	Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error)
}

type agentDelegate struct {
	family string
	agent  graph.WorkflowExecutor
}

func (d agentDelegate) Family() string { return d.family }
func (d agentDelegate) BuildGraph(task *core.Task) (*graph.Graph, error) { return d.agent.BuildGraph(task) }
func (d agentDelegate) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	return d.agent.Execute(ctx, task, state)
}

// Registry holds rex delegate adapters.
type Registry struct {
	delegates map[string]Delegate
}

func NewRegistry(env agentenv.AgentEnvironment, workspace string) *Registry {
	paths := config.New(workspace)
	return &Registry{delegates: map[string]Delegate{
		rexroute.FamilyReAct:     agentDelegate{family: rexroute.FamilyReAct, agent: reactWithPaths(env, paths.CheckpointsDir())},
		rexroute.FamilyPlanner:   agentDelegate{family: rexroute.FamilyPlanner, agent: plannerpkg.New(env)},
		rexroute.FamilyArchitect: agentDelegate{family: rexroute.FamilyArchitect, agent: architectWithPaths(env, paths.CheckpointsDir(), paths.WorkflowStateFile())},
		rexroute.FamilyPipeline:  agentDelegate{family: rexroute.FamilyPipeline, agent: pipelineWithPaths(env, paths.WorkflowStateFile())},
		"blackboard":             agentDelegate{family: "blackboard", agent: blackboardpkg.New(env)},
		"goalcon":                agentDelegate{family: "goalcon", agent: goalconpkg.New(env, goalconpkg.NewOperatorRegistry())},
		"htn":                    agentDelegate{family: "htn", agent: htnpkg.New(env, htnpkg.NewMethodLibrary())},
		"rewoo":                  agentDelegate{family: "rewoo", agent: rewoopkg.New(env)},
		"chainer":                agentDelegate{family: "chainer", agent: chainerpkg.New(env)},
	}}
}

func (r *Registry) Resolve(plan rexroute.ExecutionPlan) (Delegate, error) {
	if delegate, ok := r.delegates[plan.PrimaryFamily]; ok {
		return delegate, nil
	}
	for _, fallback := range plan.Fallbacks {
		if delegate, ok := r.delegates[fallback]; ok {
			return delegate, nil
		}
	}
	return nil, fmt.Errorf("rex delegate %q unavailable", plan.PrimaryFamily)
}

func reactWithPaths(env agentenv.AgentEnvironment, checkpointPath string) graph.WorkflowExecutor {
	agent := reactpkg.New(env)
	agent.CheckpointPath = checkpointPath
	return agent
}

func architectWithPaths(env agentenv.AgentEnvironment, checkpointPath, workflowStatePath string) graph.WorkflowExecutor {
	agent := architectpkg.New(env, architectpkg.WithPlannerTools(env.Registry), architectpkg.WithExecutorTools(env.Registry))
	agent.CheckpointPath = checkpointPath
	agent.WorkflowStatePath = workflowStatePath
	return agent
}

func pipelineWithPaths(env agentenv.AgentEnvironment, workflowStatePath string) graph.WorkflowExecutor {
	agent := pipelinepkg.New(env)
	agent.WorkflowStatePath = workflowStatePath
	return agent
}
