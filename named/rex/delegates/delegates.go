package delegates

import (
	"context"
	"fmt"

	blackboardpkg "codeburg.org/lexbit/relurpify/agents/blackboard"
	chainerpkg "codeburg.org/lexbit/relurpify/agents/chainer"
	goalconpkg "codeburg.org/lexbit/relurpify/agents/goalcon"
	htnpkg "codeburg.org/lexbit/relurpify/agents/htn"
	pipelinepkg "codeburg.org/lexbit/relurpify/agents/pipeline"
	plannerpkg "codeburg.org/lexbit/relurpify/agents/planner"
	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	rewoopkg "codeburg.org/lexbit/relurpify/agents/rewoo"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	rexroute "codeburg.org/lexbit/relurpify/named/rex/route"
)

// Delegate executes rex work for a specific orchestration family.
type Delegate interface {
	Family() string
	BuildGraph(task *core.Task) (*graph.Graph, error)
	Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error)
}

type agentDelegate struct {
	family string
	agent  graph.WorkflowExecutor
}

func (d agentDelegate) Family() string { return d.family }
func (d agentDelegate) BuildGraph(task *core.Task) (*graph.Graph, error) {
	return d.agent.BuildGraph(task)
}
func (d agentDelegate) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	return d.agent.Execute(ctx, task, env)
}

// Registry holds rex delegate adapters.
type Registry struct {
	delegates map[string]Delegate
}

func NewRegistry(env *agentenv.WorkspaceEnvironment, workspace string) *Registry {
	paths := manifest.New(workspace)
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

func reactWithPaths(env *agentenv.WorkspaceEnvironment, _ string) graph.WorkflowExecutor {
	agent := reactpkg.New(env)
	return agent
}

func architectWithPaths(env *agentenv.WorkspaceEnvironment, _, workflowStatePath string) graph.WorkflowExecutor {
	_ = workflowStatePath
	// Architect agent temporarily unavailable - using react as fallback
	agent := reactpkg.New(env)
	return agent
}

func pipelineWithPaths(env *agentenv.WorkspaceEnvironment, workflowStatePath string) graph.WorkflowExecutor {
	agent := pipelinepkg.New(env)
	agent.WorkflowStatePath = workflowStatePath
	return agent
}
