package euclo

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/persistence"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/named/euclo/euclokeys"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/services"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// defaultRegistrar provides default registration for capabilities, prompts,
// and thoughtrecipes.
var defaultRegistrar = services.NewRegistration()

// Agent is the Euclo coding agent. It implements agentgraph.WorkflowExecutor.
type Agent struct {
	resumeMu sync.Mutex

	deps        *paradigm.Deps
	config      EucloConfig
	initialized bool

	thoughtrecipeRegistry *thoughtrecipe.ThoughtRecipeRegistry

	// resume state: populated by Execute before calling BuildGraph
	resumeClassification *intake.IntentClassification
	resumeRouteSelection *euclotypes.RouteSelection
}

// New creates a new Euclo agent with the given paradigm deps and options.
func New(deps *paradigm.Deps, opts ...Option) *Agent {
	a := &Agent{
		deps:   deps,
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

// WithCheckpointRepository wires the repository used for checkpoint materialization.
func WithCheckpointRepository(repo agentlifecycle.Repository) Option {
	return func(a *Agent) {
		a.config.CheckpointRepository = repo
	}
}

// WithPersistenceWriter wires the generic persistence writer used for mirrored checkpoint writes.
func WithPersistenceWriter(writer *persistence.Writer) Option {
	return func(a *Agent) {
		a.config.PersistenceWriter = writer
	}
}

func (a *Agent) Initialize(config *execution.Config) error {
	if a.initialized {
		return nil
	}

	if a.deps.Registry == nil {
		return fmt.Errorf("workspace capability registry is nil")
	}

	registrar := services.NewRegistration(services.WithCapabilityDeps(services.CapabilityDeps{
		IndexManager:  a.deps.IndexManager,
		Workspace:     workspaceRootPath(a.deps),
		CommandRunner: a.deps.CommandRunner,
		CommandPolicy: a.deps.CommandPolicy,
		Model:         a.deps.Model,
	}))

	if err := registrar.RegisterCapabilities(a.deps.Registry); err != nil {
		return fmt.Errorf("failed to register capabilities: %w", err)
	}

	// Load the thoughtrecipe registry from the DSL source tree.
	workspace := ""
	if config != nil {
		workspace = config.Workspace
	}
	loaded, err := registrar.LoadThoughtRecipes(workspace, services.CapabilityLookup(a.deps.Registry))
	if err != nil {
		return fmt.Errorf("failed to load thoughtrecipes: %w", err)
	}
	if loaded != nil && loaded.Registry != nil {
		a.thoughtrecipeRegistry = loaded.Registry
	} else {
		a.thoughtrecipeRegistry = thoughtrecipe.NewThoughtRecipeRegistry()
	}

	a.initialized = true
	return nil
}

func (a *Agent) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	if !a.initialized {
		if err := a.Initialize(nil); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	if a.deps.StreamTrigger != nil {
		ctx = contextstream.WithTrigger(ctx, a.deps.StreamTrigger)
	}

	seedTaskEnvelope(env, task)
	a.captureResumeState(env)
	a.seedResumeState(env)
	defer a.clearResumeState()

	graph, err := a.BuildGraph(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to build execution graph: %w", err)
	}

	result, err := graph.Execute(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("graph execution failed: %w", err)
	}

	return result, nil
}

func (a *Agent) BuildGraph(ctx context.Context, task *execution.Task) (*agentgraph.Graph, error) {
	if !a.initialized {
		if err := a.Initialize(nil); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	resumeClassification, resumeRouteSelection := a.resumeStateSnapshot()
	deps := orchestrate.RootGraphDeps{
		Workspace:            workspaceRootPath(a.deps),
		DispatchCapabilities: a.deps.Registry,
		ThoughtRecipes:       a.thoughtrecipeRegistry,
		Paradigm:             a.deps,
		StreamTrigger:        a.deps.StreamTrigger,
		MaxStreamTokens:      a.config.MaxStreamTokens,
		DefaultStreamMode:    a.config.DefaultStreamMode,
		Checkpoints:          a.config.CheckpointRepository,
		Persistence:          a.config.PersistenceWriter,
	}
	rootGraph, err := orchestrate.NewRootGraph(ctx, deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build root graph: %w", err)
	}
	graph := rootGraph.Graph()
	if graph == nil {
		return nil, fmt.Errorf("root graph is nil")
	}

	start := "euclo.intake"
	switch {
	case resumeRouteSelection != nil:
		start = "euclo.policy_gate"
	case resumeClassification != nil:
		start = "euclo.dispatch"
	}
	if err := graph.SetStart(start); err != nil {
		return nil, fmt.Errorf("failed to set start node: %w", err)
	}

	return graph, nil
}

func workspaceRootPath(deps *paradigm.Deps) string {
	if deps.IndexManager == nil {
		return ""
	}
	return strings.TrimSpace(deps.IndexManager.WorkspacePath())
}

func seedTaskEnvelope(env *contextdata.Envelope, task *execution.Task) {
	if task == nil {
		return
	}
	env.SetWorkingValueWithClass(euclokeys.KeyTaskInput, task, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("task.id", task.ID, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("task.instruction", task.Instruction, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("task.type", task.Type, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("task.context", task.Context, contextdata.MemoryClassTask)
	if task.Metadata != nil {
		env.SetWorkingValueWithClass("task.metadata", task.Metadata, contextdata.MemoryClassTask)
	}
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
	if a.resumeClassification != nil {
		env.SetWorkingValueWithClass(euclostate.KeyResumeClassification, a.resumeClassification, contextdata.MemoryClassTask)
		euclostate.SetIntentClassification(env, a.resumeClassification)
	}
	if a.resumeRouteSelection != nil {
		env.SetWorkingValueWithClass(euclostate.KeyResumeRoute, a.resumeRouteSelection, contextdata.MemoryClassTask)
		euclostate.SetRouteSelection(env, a.resumeRouteSelection)
	}
}

func (a *Agent) resumeStateSnapshot() (*intake.IntentClassification, *euclotypes.RouteSelection) {
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
