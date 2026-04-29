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
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

// Agent is the Euclo coding agent. It implements agentgraph.WorkflowExecutor.
type Agent struct {
	resumeMu sync.Mutex

	env         agentenv.WorkspaceEnvironment
	config      EucloConfig
	initialized bool

	// Sub-package components (to be wired in later phases)
	// families         *families.KeywordFamilyRegistry
	// recipes          *recipes.RecipeRegistry
	// capabilities     *capabilities.EucloCapabilityRegistry
	// telemetry        *reporting.EucloTelemetry
	streamTrigger *contextstream.Trigger

	// resume state: populated by Execute before calling BuildGraph
	resumeClassification *intake.IntentClassification
	resumeRouteSelection *orchestrate.RouteSelection
}

// New creates a new Euclo agent with the given workspace environment and options.
func New(env agentenv.WorkspaceEnvironment, opts ...Option) *Agent {
	a := &Agent{
		env:    env,
		config: DefaultConfig(),
		// families:     families.NewRegistry(), // Phase 4
		// recipes:      recipes.NewRegistry(),  // Phase 7
		// capabilities: capabilities.NewRegistry(), // Phase 6
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

// WithFamilies sets the keyword family registry (Phase 4).
// func WithFamilies(registry *families.KeywordFamilyRegistry) Option {
// 	return func(a *Agent) {
// 		a.families = registry
// 	}
// }

// WithRecipes sets the recipe registry (Phase 7).
// func WithRecipes(registry *recipes.RecipeRegistry) Option {
// 	return func(a *Agent) {
// 		a.recipes = registry
// 	}
// }

// WithCapabilities sets the capability registry (Phase 6).
// func WithCapabilities(registry *capabilities.EucloCapabilityRegistry) Option {
// 	return func(a *Agent) {
// 		a.capabilities = registry
// 	}
// }

// WithTelemetry sets the telemetry sink (Phase 13).
// func WithTelemetry(sink core.Telemetry) Option {
// 	return func(a *Agent) {
// 		a.telemetry = reporting.NewTelemetry(sink)
// 	}
// }

// WithStreamTrigger sets the context stream trigger.
func WithStreamTrigger(trigger *contextstream.Trigger) Option {
	return func(a *Agent) {
		a.streamTrigger = trigger
	}
}

// Initialize initializes the agent with framework configuration.
// Registers built-in families and loads recipe directories.
func (a *Agent) Initialize(config *core.Config) error {
	if a.initialized {
		return nil
	}

	// Phase 4: Register built-in families
	// if a.config.BuiltinFamilies {
	// 	families.RegisterBuiltins(a.families)
	// }

	// Phase 13: Initialize telemetry
	// if a.telemetry == nil {
	// 	a.telemetry = reporting.NewTelemetry(nil)
	// }

	a.initialized = true
	return nil
}

// Execute implements agentgraph.WorkflowExecutor.Execute.
// It inspects the envelope for prior classification state, stashes resume context,
// calls BuildGraph, then executes the graph.
func (a *Agent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*agentgraph.Result, error) {
	if !a.initialized {
		if err := a.Initialize(nil); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	a.captureResumeState(env)
	a.seedResumeState(env)
	defer a.clearResumeState()

	// Step 2: Build the execution graph
	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, fmt.Errorf("failed to build execution graph: %w", err)
	}

	// Step 3: Execute the graph
	result, err := graph.Execute(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("graph execution failed: %w", err)
	}

	return result, nil
}

// BuildGraph implements agentgraph.WorkflowExecutor.BuildGraph.
// Constructs the root Euclo execution graph based on the task.
// Phase 14: Wires together all orchestration components.
func (a *Agent) BuildGraph(task *core.Task) (*agentgraph.Graph, error) {
	if !a.initialized {
		if err := a.Initialize(nil); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	resumeClassification, resumeRouteSelection := a.resumeStateSnapshot()
	graph := agentgraph.NewGraph()

	routeKindFromTask := func() string {
		if resumeRouteSelection != nil && resumeRouteSelection.RouteKind != "" {
			return resumeRouteSelection.RouteKind
		}
		if task != nil {
			switch task.Type {
			case "review", "analysis", "inspect":
				return "recipe"
			case "repair", "migration", "debug", "implementation":
				return "capability"
			}
			instruction := task.Instruction
			switch {
			case instruction == "":
				return "capability"
			case containsAny(instruction, []string{"review", "investigate", "plan", "design", "reflect"}):
				return "recipe"
			default:
				return "capability"
			}
		}
		return "capability"
	}

	familyFromTask := func() string {
		if resumeClassification != nil && resumeClassification.WinningFamily != "" {
			return resumeClassification.WinningFamily
		}
		if task == nil {
			return "general"
		}
		switch {
		case containsAny(task.Instruction, []string{"review", "audit", "check"}):
			return "review"
		case containsAny(task.Instruction, []string{"fix", "repair", "patch", "bug"}):
			return "repair"
		case containsAny(task.Instruction, []string{"migrate", "upgrade", "port"}):
			return "migration"
		case containsAny(task.Instruction, []string{"plan", "design", "architecture"}):
			return "planning"
		case containsAny(task.Instruction, []string{"trace", "investigate", "understand"}):
			return "investigation"
		default:
			if task != nil && task.Type != "" {
				return task.Type
			}
			return "general"
		}
	}

	add := func(node agentgraph.Node) error {
		if err := graph.AddNode(node); err != nil {
			return err
		}
		return nil
	}
	noop := func(id string, nodeType agentgraph.NodeType, data map[string]any, fn func(context.Context, *contextdata.Envelope) (*agentgraph.Result, error)) agentgraph.Node {
		return &workflowNode{id: id, nodeType: nodeType, data: data, execFn: fn}
	}

	intakeNode := noop("euclo.intake", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		if env != nil && task != nil {
			env.SetWorkingValue("task.id", task.ID, contextdata.MemoryClassTask)
			env.SetWorkingValue("task.type", task.Type, contextdata.MemoryClassTask)
			env.SetWorkingValue("task.instruction", task.Instruction, contextdata.MemoryClassTask)
		}
		return &agentgraph.Result{NodeID: "euclo.intake", Success: true}, nil
	})
	familySelectNode := noop("euclo.family_select", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		if env != nil {
			euclostate.SetFamilySelection(env, familyFromTask())
		}
		return &agentgraph.Result{NodeID: "euclo.family_select", Success: true, Data: map[string]any{"family": familyFromTask()}}, nil
	})
	ingestNode := noop("euclo.ingest", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		files, _ := euclostate.GetUserSelectedFiles(env)
		policy, _ := euclostate.GetIngestPolicy(env)
		result := map[string]any{
			"files_ingested": len(files),
			"policy":         policy,
		}
		if env != nil {
			env.SetWorkingValue(euclostate.KeyIngestionResult, result, contextdata.MemoryClassTask)
		}
		return &agentgraph.Result{NodeID: "euclo.ingest", Success: true, Data: result}, nil
	})
	streamNode := noop("euclo.stream", agentgraph.NodeTypeStream, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = env
		result := map[string]any{"streamed": false}
		if a.streamTrigger != nil && task != nil {
			req := contextstream.Request{
				ID:        "euclo.stream",
				MaxTokens: a.config.MaxStreamTokens,
				Mode:      a.config.DefaultStreamMode,
				Query: retrieval.RetrievalQuery{
					Text: task.Instruction,
				},
			}
			streamResult, err := a.streamTrigger.RequestBlocking(ctx, req)
			if err != nil {
				return &agentgraph.Result{NodeID: "euclo.stream", Success: false, Data: map[string]any{"error": err.Error()}}, err
			}
			result["streamed"] = true
			result["stream_result"] = streamResult
		}
		return &agentgraph.Result{NodeID: "euclo.stream", Success: true, Data: result}, nil
	})
	classifyNode := noop("euclo.capability_classify", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		selection := routeKindFromTask()
		caps := []string{"euclo:cap.ast_query"}
		if env != nil {
			env.SetWorkingValue(euclostate.KeyCapabilitySequence, caps, contextdata.MemoryClassTask)
		}
		return &agentgraph.Result{NodeID: "euclo.capability_classify", Success: true, Data: map[string]any{"capability_sequence": caps, "route_kind": selection}}, nil
	})
	interactionCheckNode := noop("euclo.interaction_check", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		if env != nil && a.config.HITLTimeout > 0 {
			env.SetWorkingValue(euclostate.KeyHITLTriggered, false, contextdata.MemoryClassTask)
		}
		return &agentgraph.Result{NodeID: "euclo.interaction_check", Success: true}, nil
	})
	policyGateNode := noop("euclo.policy_gate", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		if env != nil {
			env.SetWorkingValue(euclostate.KeyPolicyDecision, map[string]any{"approved": true}, contextdata.MemoryClassTask)
		}
		return &agentgraph.Result{NodeID: "euclo.policy_gate", Success: true}, nil
	})
	dispatchNode := noop("euclo.dispatch", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		selection := &orchestrate.RouteSelection{
			RouteKind:    routeKindFromTask(),
			RecipeID:     "",
			CapabilityID: "",
		}
		if selection.RouteKind == "recipe" {
			selection.RecipeID = "euclo.recipe.default"
		} else {
			selection.CapabilityID = "euclo:cap.ast_query"
		}
		if env != nil {
			euclostate.SetRouteSelection(env, selection)
			env.SetWorkingValue("euclo.dispatch.route_kind", selection.RouteKind, contextdata.MemoryClassTask)
			env.SetWorkingValue("euclo.route.kind", selection.RouteKind, contextdata.MemoryClassTask)
			env.SetWorkingValue("euclo.route.recipe_id", selection.RecipeID, contextdata.MemoryClassTask)
			env.SetWorkingValue("euclo.route.capability_id", selection.CapabilityID, contextdata.MemoryClassTask)
		}
		return &agentgraph.Result{NodeID: "euclo.dispatch", Success: true, Data: map[string]any{
			"route_kind":    selection.RouteKind,
			"recipe_id":     selection.RecipeID,
			"capability_id": selection.CapabilityID,
		}}, nil
	})
	routeForkNode := noop("euclo.route_fork", agentgraph.NodeTypeConditional, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		routeKind := "capability"
		if env != nil {
			if rs, ok := euclostate.GetRouteSelection(env); ok && rs != nil && rs.RouteKind != "" {
				routeKind = rs.RouteKind
			} else if v, ok := env.GetWorkingValue("euclo.dispatch.route_kind"); ok {
				if s, ok := v.(string); ok && s != "" {
					routeKind = s
				}
			}
		}
		next := "euclo.execute_capability"
		if routeKind == "recipe" {
			next = "euclo.execute_recipe"
		}
		return &agentgraph.Result{NodeID: "euclo.route_fork", Success: true, Data: map[string]any{
			"next":       next,
			"route_kind": routeKind,
		}}, nil
	})
	executeRecipeNode := noop("euclo.execute_recipe", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		if env != nil {
			env.SetWorkingValue(euclostate.KeyRecipeID, "euclo.recipe.default", contextdata.MemoryClassTask)
		}
		return &agentgraph.Result{NodeID: "euclo.execute_recipe", Success: true, Data: map[string]any{"execution_kind": "recipe"}}, nil
	})
	executeCapabilityNode := noop("euclo.execute_capability", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		if env != nil {
			env.SetWorkingValue(euclostate.KeyCapabilitySequence, []string{"euclo:cap.ast_query"}, contextdata.MemoryClassTask)
		}
		return &agentgraph.Result{NodeID: "euclo.execute_capability", Success: true, Data: map[string]any{"execution_kind": "capability"}}, nil
	})
	mergeNode := noop("euclo.merge", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		if env != nil {
			env.SetWorkingValue(euclostate.KeyOutcomeCategory, "completed", contextdata.MemoryClassTask)
		}
		return &agentgraph.Result{NodeID: "euclo.merge", Success: true}, nil
	})
	reportNode := noop("euclo.report", agentgraph.NodeTypeSystem, nil, func(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
		_ = ctx
		return &agentgraph.Result{NodeID: "euclo.report", Success: true, Data: map[string]any{"reported": true}}, nil
	})
	doneNode := agentgraph.NewTerminalNode("euclo.done")

	for _, node := range []agentgraph.Node{
		intakeNode,
		familySelectNode,
		ingestNode,
		streamNode,
		classifyNode,
		interactionCheckNode,
		policyGateNode,
		dispatchNode,
		routeForkNode,
		executeRecipeNode,
		executeCapabilityNode,
		mergeNode,
		reportNode,
		doneNode,
	} {
		if err := add(node); err != nil {
			return nil, fmt.Errorf("failed to add node %s: %w", node.ID(), err)
		}
	}

	edges := []struct {
		from, to string
		cond     agentgraph.ConditionFunc
	}{
		{"euclo.intake", "euclo.family_select", nil},
		{"euclo.family_select", "euclo.ingest", nil},
		{"euclo.ingest", "euclo.stream", nil},
		{"euclo.stream", "euclo.capability_classify", nil},
		{"euclo.capability_classify", "euclo.interaction_check", nil},
		{"euclo.interaction_check", "euclo.policy_gate", nil},
	}
	for _, edge := range edges {
		if err := graph.AddEdge(edge.from, edge.to, edge.cond, false); err != nil {
			return nil, err
		}
	}

	if resumeRouteSelection != nil && resumeRouteSelection.RouteKind != "" {
		if err := graph.AddEdge("euclo.policy_gate", "euclo.route_fork", nil, false); err != nil {
			return nil, err
		}
	} else {
		if err := graph.AddEdge("euclo.policy_gate", "euclo.dispatch", nil, false); err != nil {
			return nil, err
		}
		if err := graph.AddEdge("euclo.dispatch", "euclo.route_fork", nil, false); err != nil {
			return nil, err
		}
	}

	if err := graph.AddEdge("euclo.route_fork", "euclo.execute_recipe", func(result *agentgraph.Result, env *contextdata.Envelope) bool {
		if result == nil || result.Data == nil {
			return false
		}
		next, _ := result.Data["next"].(string)
		return next == "euclo.execute_recipe"
	}, false); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("euclo.route_fork", "euclo.execute_capability", func(result *agentgraph.Result, env *contextdata.Envelope) bool {
		if result == nil || result.Data == nil {
			return false
		}
		next, _ := result.Data["next"].(string)
		return next == "euclo.execute_capability"
	}, false); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("euclo.execute_recipe", "euclo.merge", nil, false); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("euclo.execute_capability", "euclo.merge", nil, false); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("euclo.merge", "euclo.report", nil, false); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("euclo.report", "euclo.done", nil, false); err != nil {
		return nil, err
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

// Capabilities implements agentgraph.WorkflowExecutor.Capabilities.
// Returns the capability IDs provided by this agent.
func (a *Agent) Capabilities() []string {
	return []string{
		"euclo.agent",
		"euclo.routing",
		"euclo.classification",
	}
}

// Compile-time interface assertion
var _ agentgraph.WorkflowExecutor = (*Agent)(nil)

// systemNode is a minimal Node implementation for Phase 1 stub nodes.
// Full implementation will use proper agentgraph system nodes in later phases.
type systemNode struct {
	id     string
	execFn func(context.Context, *contextdata.Envelope) (*agentgraph.Result, error)
}

func (n *systemNode) ID() string                { return n.id }
func (n *systemNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }
func (n *systemNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	return n.execFn(ctx, env)
}

type workflowNode struct {
	id       string
	nodeType agentgraph.NodeType
	data     map[string]any
	execFn   func(context.Context, *contextdata.Envelope) (*agentgraph.Result, error)
}

func (n *workflowNode) ID() string                { return n.id }
func (n *workflowNode) Type() agentgraph.NodeType { return n.nodeType }
func (n *workflowNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	return n.execFn(ctx, env)
}

func containsAny(instruction string, terms []string) bool {
	lower := strings.ToLower(instruction)
	for _, term := range terms {
		if strings.Contains(lower, strings.ToLower(term)) {
			return true
		}
	}
	return false
}
