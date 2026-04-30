// Package euclo implements the coding-specialized top-level agent that resolves
// user requests to deterministic execution routes through agentgraph workflows.
//
// Euclo is a WorkflowExecutor: it builds and executes agentgraph.Graph structures
// that compose framework system nodes, context stream triggers, ingestion nodes,
// /agents execution backends, and HITL gates.
package euclo

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	recipe "codeburg.org/lexbit/relurpify/named/euclo/recipes"
	"codeburg.org/lexbit/relurpify/named/euclo/recipetemplates"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

// Agent is the Euclo coding agent. It implements agentgraph.WorkflowExecutor.
type Agent struct {
	resumeMu sync.Mutex

	env         agentenv.WorkspaceEnvironment
	config      EucloConfig
	initialized bool

	streamTrigger  *contextstream.Trigger
	recipeRegistry *recipe.RecipeRegistry

	// resume state: populated by Execute before calling BuildGraph
	resumeClassification *intake.IntentClassification
	resumeRouteSelection *orchestrate.RouteSelection
}

// New creates a new Euclo agent with the given workspace environment and options.
func New(env agentenv.WorkspaceEnvironment, opts ...Option) *Agent {
	a := &Agent{
		env:    env,
		config: DefaultConfig(),
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// Option configures the Agent.
type Option func(*Agent)

// WithConfig sets the EucloConfig for the agent.
func WithConfig(config EucloConfig) Option {
	return func(a *Agent) {
		a.config = config
	}
}

// WithStreamTrigger sets the context stream trigger.
func WithStreamTrigger(trigger *contextstream.Trigger) Option {
	return func(a *Agent) {
		a.streamTrigger = trigger
	}
}

func (a *Agent) Initialize(config *core.Config) error {
	if a.initialized {
		return nil
	}

	// Register all relurpic capability handlers
	if err := relurpicabilities.RegisterAll(a.env); err != nil {
		return fmt.Errorf("failed to register relurpic capabilities: %w", err)
	}

	// Load all recipe templates
	var err error
	a.recipeRegistry, err = recipetemplates.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load recipe templates: %w", err)
	}

	a.initialized = true
	return nil
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*agentgraph.Result, error) {
	if !a.initialized {
		if err := a.Initialize(nil); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	a.captureResumeState(env)
	a.seedResumeState(env)
	defer a.clearResumeState()

	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, fmt.Errorf("failed to build execution graph: %w", err)
	}

	result, err := graph.Execute(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("graph execution failed: %w", err)
	}

	return result, nil
}

func (a *Agent) BuildGraph(task *core.Task) (*agentgraph.Graph, error) {
	if !a.initialized {
		if err := a.Initialize(nil); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	resumeClassification, resumeRouteSelection := a.resumeStateSnapshot()
	rootGraph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(a.env),
		orchestrate.WithContextStreamTrigger(a.streamTrigger),
		orchestrate.WithWorkspace(workspaceRootPath(a.env)),
		orchestrate.WithCapabilityRegistry(a.env.Registry),
		orchestrate.WithRecipeRegistry(a.recipeRegistry),
	)
	graph := rootGraph.Graph()
	if graph == nil {
		return nil, fmt.Errorf("root graph is nil")
	}

	start := "euclo.intake"
	switch {
	case resumeClassification != nil && resumeRouteSelection != nil:
		start = "euclo.policy_gate"
	case resumeClassification != nil:
		start = "euclo.dispatch"
	}
	if err := graph.SetStart(start); err != nil {
		return nil, fmt.Errorf("failed to set start node: %w", err)
	}

	return graph, nil
}

func workspaceRootPath(env agentenv.WorkspaceEnvironment) string {
	if env.IndexManager == nil {
		return ""
	}
	return strings.TrimSpace(env.IndexManager.WorkspacePath())
}

func (a *Agent) captureResumeState(env *contextdata.Envelope) {
	classification, _ := euclostate.GetIntentClassification(env)
	routeSelection, _ := euclostate.GetRouteSelection(env)

	a.resumeMu.Lock()
	defer a.resumeMu.Unlock()
	a.resumeClassification = classification
	a.resumeRouteSelection = routeSelection
}

func (a *Agent) seedResumeState(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if a.resumeClassification != nil {
		env.SetWorkingValue(euclostate.KeyResumeClassification, a.resumeClassification, contextdata.MemoryClassTask)
		euclostate.SetIntentClassification(env, a.resumeClassification)
	}
	if a.resumeRouteSelection != nil {
		env.SetWorkingValue(euclostate.KeyResumeRoute, a.resumeRouteSelection, contextdata.MemoryClassTask)
		euclostate.SetRouteSelection(env, a.resumeRouteSelection)
	}
}

func (a *Agent) resumeStateSnapshot() (*intake.IntentClassification, *orchestrate.RouteSelection) {
	a.resumeMu.Lock()
	defer a.resumeMu.Unlock()
	return a.resumeClassification, a.resumeRouteSelection
}

func (a *Agent) clearResumeState() {
	a.resumeMu.Lock()
	defer a.resumeMu.Unlock()
	a.resumeClassification = nil
	a.resumeRouteSelection = nil
}

func (a *Agent) Capabilities() []string {
	return []string{
		"euclo.agent",
		"euclo.routing",
		"euclo.classification",
	}
}

var _ agentgraph.WorkflowExecutor = (*Agent)(nil)
