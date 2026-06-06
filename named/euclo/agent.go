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
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/persistence"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	platformcontracts "codeburg.org/lexbit/relurpify/platform/contracts"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// Agent is the Euclo coding agent. It implements agentgraph.WorkflowExecutor.
type Agent struct {
	resumeMu sync.Mutex

	env         agentenv.WorkspaceEnvironment
	config      EucloConfig
	initialized bool

	thoughtrecipeRegistry *thoughtrecipe.ThoughtRecipeRegistry

	// resume state: populated by Execute before calling BuildGraph
	resumeClassification *intake.IntentClassification
	resumeRouteSelection *euclotypes.RouteSelection
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

	if a.env.Registry == nil {
		return fmt.Errorf("workspace capability registry is nil")
	}

	regFuncs := GetRegistrationFuncs()

	if regFuncs.RegisterCapabilities != nil {
		if err := regFuncs.RegisterCapabilities(a.env); err != nil {
			return fmt.Errorf("failed to register capabilities: %w", err)
		}
	}

	if regFuncs.RegisterPromptProviders != nil {
		if err := regFuncs.RegisterPromptProviders(a.env); err != nil {
			return fmt.Errorf("failed to register prompt providers: %w", err)
		}
	}

	// Load the thoughtrecipe registry from the DSL source tree.
	if regFuncs.LoadThoughtRecipes != nil {
		loaded, err := regFuncs.LoadThoughtRecipes()
		if err != nil {
			return fmt.Errorf("failed to load thoughtrecipes: %w", err)
		}
		switch v := loaded.(type) {
		case *thoughtrecipe.LoadResult:
			if v != nil {
				a.thoughtrecipeRegistry = v.Registry
			}
		case *thoughtrecipe.ThoughtRecipeRegistry:
			a.thoughtrecipeRegistry = v
		}
		if a.thoughtrecipeRegistry == nil {
			a.thoughtrecipeRegistry = thoughtrecipe.NewThoughtRecipeRegistry()
		}
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

	if a.env.StreamTrigger != nil {
		ctx = contextstream.WithTrigger(ctx, a.env.StreamTrigger)
	}

	seedTaskEnvelope(env, task)
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

func (a *Agent) BuildGraph(task *execution.Task) (*agentgraph.Graph, error) {
	if !a.initialized {
		if err := a.Initialize(nil); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	resumeClassification, resumeRouteSelection := a.resumeStateSnapshot()
	rootGraph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(a.env),
		orchestrate.WithWorkspace(workspaceRootPath(a.env)),
		orchestrate.WithCapabilityRegistry(a.env.Registry),
		orchestrate.WithThoughtRecipeRegistry(a.thoughtrecipeRegistry),
		orchestrate.WithMaxStreamTokens(a.config.MaxStreamTokens),
		orchestrate.WithDefaultStreamMode(a.config.DefaultStreamMode),
		orchestrate.WithStreamTrigger(a.env.StreamTrigger),
		orchestrate.WithCheckpointRepository(a.config.CheckpointRepository),
		orchestrate.WithPersistenceWriter(a.config.PersistenceWriter),
	)
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

func workspaceRootPath(env agentenv.WorkspaceEnvironment) string {
	if env.IndexManager == nil {
		return ""
	}
	return strings.TrimSpace(env.IndexManager.WorkspacePath())
}

func seedTaskEnvelope(env *contextdata.Envelope, task *execution.Task) {
	if task == nil {
		return
	}
	env.SetWorkingValue("task.input", task, contextdata.MemoryClassTask)
	env.SetWorkingValue("task.id", task.ID, contextdata.MemoryClassTask)
	env.SetWorkingValue("task.instruction", task.Instruction, contextdata.MemoryClassTask)
	env.SetWorkingValue("task.type", task.Type, contextdata.MemoryClassTask)
	env.SetWorkingValue("task.context", task.Context, contextdata.MemoryClassTask)
	if task.Metadata != nil {
		env.SetWorkingValue("task.metadata", task.Metadata, contextdata.MemoryClassTask)
	}
}

type modelAdapter struct {
	model platformcontracts.LanguageModel
}

func (m *modelAdapter) Complete(ctx context.Context, req intake.CompletionRequest) (intake.CompletionResponse, error) {
	if m == nil || m.model == nil {
		return intake.CompletionResponse{}, fmt.Errorf("no language model configured")
	}
	resp, err := m.model.Generate(ctx, req.Prompt, &platformcontracts.LLMOptions{
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	if err != nil {
		return intake.CompletionResponse{}, err
	}
	if resp == nil {
		return intake.CompletionResponse{}, fmt.Errorf("language model returned nil response")
	}
	return intake.CompletionResponse{Text: resp.Text}, nil
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
		env.SetWorkingValue(euclostate.KeyResumeClassification, a.resumeClassification, contextdata.MemoryClassTask)
		euclostate.SetIntentClassification(env, a.resumeClassification)
	}
	if a.resumeRouteSelection != nil {
		env.SetWorkingValue(euclostate.KeyResumeRoute, a.resumeRouteSelection, contextdata.MemoryClassTask)
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
