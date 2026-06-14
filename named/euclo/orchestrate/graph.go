package orchestrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/persistence"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// RootGraphDeps is the explicit dependency contract for graph execution.
// It replaces agentenv.AgentContext as the construction input to RootGraph.
type RootGraphDeps struct {
	Workspace            string
	DispatchCapabilities *registry.CapabilityRegistry
	ThoughtRecipes       *thoughtrecipepkg.ThoughtRecipeRegistry
	Families             *families.KeywordFamilyRegistry
	Paradigm             *paradigm.Deps
	StreamTrigger        *contextstream.Trigger
	MaxStreamTokens      int
	DefaultStreamMode    contextstream.Mode
	PermissionManager    policy.PermissionManager
	HITLBroker           policy.HITLBroker
	Checkpoints          agentlifecycle.Repository
	Persistence          *persistence.Writer
}

// RootGraph wires together orchestration nodes using the agentgraph runtime.
type RootGraph struct {
	graph *agentgraph.Graph
}

// NewRootGraph creates a new root graph from explicit dependencies.
func NewRootGraph(ctx context.Context, deps RootGraphDeps) (*RootGraph, error) {
	if err := validateRootGraphDeps(deps); err != nil {
		return nil, err
	}
	paradigmDeps := deps.Paradigm
	capReg := deps.DispatchCapabilities
	thoughtReg := deps.ThoughtRecipes
	famReg := deps.Families
	maxTokens := deps.MaxStreamTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	streamMode := deps.DefaultStreamMode
	if streamMode == "" {
		streamMode = contextstream.ModeBlocking
	}
	if famReg == nil {
		famReg = defaultFamilyRegistry()
	}
	hitl := deps.HITLBroker
	if hitl == nil {
		hitl = permissiveHITLBroker{}
	}

	g := agentgraph.NewGraph()
	nodes, err := buildNodes(ctx, buildNodeInput{
		capReg:               capReg,
		thoughtReg:           thoughtReg,
		paradigmDeps:         paradigmDeps,
		famReg:               famReg,
		maxTokens:            maxTokens,
		streamMode:           streamMode,
		streamTrigger:        deps.StreamTrigger,
		workspace:            deps.Workspace,
		permissionManager:    deps.PermissionManager,
		hitlBroker:           hitl,
		checkpointRepository: deps.Checkpoints,
		persistenceWriter:    deps.Persistence,
	})
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := wireEdges(g); err != nil {
		return nil, err
	}
	if err := g.SetStart("euclo.intake"); err != nil {
		return nil, err
	}
	return &RootGraph{graph: g}, nil
}

// Graph returns the underlying agentgraph graph.
func (g *RootGraph) Graph() *agentgraph.Graph {
	if g == nil {
		return nil
	}
	return g.graph
}

// Execute runs the root graph orchestration.
func (g *RootGraph) Execute(ctx context.Context, env *contextdata.Envelope) error {
	if g == nil || g.graph == nil {
		return fmt.Errorf("root graph not initialized")
	}
	_, err := g.graph.Execute(ctx, env)
	return err
}

// SetStart sets the start node for the graph (used for resume).
func (g *RootGraph) SetStart(nodeID string) error {
	if g == nil || g.graph == nil {
		return fmt.Errorf("root graph not initialized")
	}
	return g.graph.SetStart(nodeID)
}

func validateRootGraphDeps(deps RootGraphDeps) error {
	if deps.Paradigm == nil {
		return &errMissingDeps{name: "paradigm deps"}
	}
	return nil
}

type errMissingDeps struct {
	name string
}

func (e *errMissingDeps) Error() string {
	return "root graph: missing required dependency: " + e.name
}

type buildNodeInput struct {
	capReg               *registry.CapabilityRegistry
	thoughtReg           *thoughtrecipepkg.ThoughtRecipeRegistry
	paradigmDeps         *paradigm.Deps
	famReg               *families.KeywordFamilyRegistry
	maxTokens            int
	streamMode           contextstream.Mode
	streamTrigger        *contextstream.Trigger
	workspace            string
	permissionManager    policy.PermissionManager
	hitlBroker           policy.HITLBroker
	checkpointRepository agentlifecycle.Repository
	persistenceWriter    *persistence.Writer
}

func buildNodes(ctx context.Context, in buildNodeInput) ([]agentgraph.Node, error) {
	dispatchCapReg := in.capReg
	thoughtrecipeReg := in.thoughtReg
	if thoughtrecipeReg == nil {
		thoughtrecipeReg = thoughtrecipepkg.NewThoughtRecipeRegistry()
	}
	ensureClarificationThoughtRecipe(thoughtrecipeReg)
	var thoughtrecipeCapReg *registry.CapabilityRegistry
	if in.paradigmDeps != nil {
		thoughtrecipeCapReg = in.paradigmDeps.Registry
	}
	if thoughtrecipeCapReg == nil {
		thoughtrecipeCapReg = registry.NewRegistry()
	}
	if err := registerClarificationCapability(ctx, thoughtrecipeCapReg); err != nil {
		return nil, err
	}
	intakePipeline := intake.NewIntakePipelineNode(
		"euclo.intake",
		in.famReg,
		in.maxTokens,
		in.streamMode,
		in.streamTrigger,
	)
	intakeNode := newStageNode("euclo.intake", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		seedDefaultTask(env)
		return intakePipeline.Execute(ctx, env)
	})

	familySelectNode := newStageNode("euclo.family_select", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		family := ""
		if m, ok := contextdata.GetTyped[map[string]any](env, euclostate.KeyFamilySelection); ok {
			if s, ok := m["winning_family"].(string); ok {
				family = strings.TrimSpace(s)
			}
		}
		if family != "" {
			euclostate.SetFamilySelected(env, family)
		}
		return &execution.Result{
			NodeID:  "euclo.family_select",
			Success: true,
			Data:    execution.NewToolResultPayload(map[string]any{"family": family}),
		}, nil
	})

	ingestionNode := NewIngestionNode("euclo.ingest")

	streamNode := newStageNode("euclo.stream", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		_, hasStream := contextdata.GetTyped[any](env, euclostate.KeyStreamResult)
		euclostate.SetStreamRequested(env, hasStream)
		return &execution.Result{
			NodeID:  "euclo.stream",
			Success: true,
			Data:    execution.NewToolResultPayload(map[string]any{"streamed": hasStream}),
		}, nil
	})

	checkpointNode := agentgraph.NewCheckpointNode("euclo.checkpoint").
		WithRepository(in.checkpointRepository).
		WithWriter(in.persistenceWriter)

	capClassifyNode := newStageNode("euclo.capability_classify", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		if env != nil {
			euclostate.SetCapabilityClassified(env, true)
		}
		return &execution.Result{
			NodeID:  "euclo.capability_classify",
			Success: true,
			Data:    execution.NewToolResultPayload(map[string]any{"classified": true}),
		}, nil
	})

	interactionCheckNode := newStageNode("euclo.interaction_check", agentgraph.NodeTypeConditional, func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		needsInteraction := false
		if cls, ok := euclostate.GetIntentClassification(env); ok && cls != nil {
			needsInteraction = cls.Ambiguous || cls.Confidence < 0.7
		}
		return &execution.Result{
			NodeID:  "euclo.interaction_check",
			Success: true,
			Data:    execution.NewToolResultPayload(map[string]any{"needs_interaction": needsInteraction}),
		}, nil
	})

	interactionFrameNode := newStageNode("euclo.interaction_frame", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		if env != nil {
			seedPolicyDefaults(env)
			euclostate.SetInteractionFrameRequested(env, true)
		}
		return &execution.Result{
			NodeID:  "euclo.interaction_frame",
			Success: true,
		}, nil
	})

	gate := policy.NewGateNode("euclo.policy_gate", policy.NewEvaluator()).
		WithPermissionManager(in.permissionManager).
		WithHITLBroker(in.hitlBroker)
	policyGateNode := newStageNode("euclo.policy_gate", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		seedPolicyDefaults(env)
		data, err := gate.Execute(ctx, env)
		if err != nil {
			return &execution.Result{NodeID: "euclo.policy_gate", Success: false, Data: execution.NewErrorResultPayload(err.Error())}, err
		}
		return &execution.Result{
			NodeID:  "euclo.policy_gate",
			Success: true,
			Data:    execution.NewToolResultPayload(data),
		}, nil
	})

	dispatchNode := NewDispatcher("euclo.dispatch").
		WithWorkspace(in.workspace).
		WithCapabilityRegistry(dispatchCapReg).
		WithThoughtRecipeRegistry(thoughtrecipeReg)

	routeForkNode := NewRouteForkNode("euclo.route_fork")

	thoughtrecipeExec := NewThoughtRecipeExecutorNode("euclo.execute_thoughtrecipe").
		WithParadigmDeps(in.paradigmDeps).
		WithIngestionPipeline(nil)
	thoughtrecipeExec.WithThoughtRecipeRegistry(thoughtrecipeReg)

	capabilityExec := NewCapabilityExecutionNode("euclo.execute_capability")
	if dispatchCapReg != nil {
		capabilityExec.WithCapabilityRegistry(dispatchCapReg)
	}

	mergeNode := NewMergeNode("euclo.merge")

	telemetryNode := reporting.NewTelemetryNode("euclo.report")
	reportNode := newStageNode("euclo.report", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		if strings.EqualFold(strings.TrimSpace(euclostate.GetRouteOutcome(env)), string(reporting.RouteOutcomeDryRun)) {
			euclostate.SetExecutionCompleted(env, true)
		}
		result, err := telemetryNode.Execute(ctx, env)
		if err != nil {
			return &execution.Result{NodeID: "euclo.report", Success: false, Data: execution.NewErrorResultPayload(err.Error())}, err
		}
		return &execution.Result{
			NodeID:  "euclo.report",
			Success: true,
			Data:    execution.NewToolResultPayload(result),
		}, nil
	})

	doneNode := agentgraph.NewTerminalNode("euclo.done")

	return []agentgraph.Node{
		intakeNode,
		familySelectNode,
		ingestionNode,
		streamNode,
		checkpointNode,
		capClassifyNode,
		interactionCheckNode,
		interactionFrameNode,
		policyGateNode,
		dispatchNode,
		routeForkNode,
		thoughtrecipeExec,
		capabilityExec,
		mergeNode,
		reportNode,
		doneNode,
	}, nil
}

func wireEdges(g *agentgraph.Graph) error {
	edges := []struct {
		from string
		to   string
		cond agentgraph.ConditionFunc
	}{
		{"euclo.intake", "euclo.family_select", nil},
		{"euclo.family_select", "euclo.ingest", nil},
		{"euclo.ingest", "euclo.stream", nil},
		{"euclo.stream", "euclo.checkpoint", nil},
		{"euclo.checkpoint", "euclo.capability_classify", nil},
		{"euclo.capability_classify", "euclo.interaction_check", nil},
		{"euclo.interaction_check", "euclo.interaction_frame", func(result *execution.Result, _ *contextdata.Envelope) bool {
			if result == nil {
				return false
			}
			if got, ok := execution.ResultField(result.Data, "needs_interaction"); ok {
				return got == true
			}
			return false
		}},
		{"euclo.interaction_check", "euclo.policy_gate", func(result *execution.Result, _ *contextdata.Envelope) bool {
			if result == nil {
				return true
			}
			if got, ok := execution.ResultField(result.Data, "needs_interaction"); ok {
				return got != true
			}
			return true
		}},
		{"euclo.interaction_frame", "euclo.policy_gate", nil},
		{"euclo.policy_gate", "euclo.dispatch", nil},
		{"euclo.dispatch", "euclo.report", func(_ *execution.Result, env *contextdata.Envelope) bool {
			if strings.EqualFold(strings.TrimSpace(euclostate.GetRouteOutcome(env)), string(reporting.RouteOutcomeDryRun)) {
				return true
			}
			dryRun, _ := euclostate.GetDryRunMode(env)
			return dryRun
		}},
		{"euclo.dispatch", "euclo.route_fork", func(_ *execution.Result, env *contextdata.Envelope) bool {
			if strings.EqualFold(strings.TrimSpace(euclostate.GetRouteOutcome(env)), string(reporting.RouteOutcomeDryRun)) {
				return false
			}
			dryRun, _ := euclostate.GetDryRunMode(env)
			return !dryRun
		}},
		{"euclo.route_fork", "euclo.execute_thoughtrecipe", func(result *execution.Result, _ *contextdata.Envelope) bool {
			if result == nil {
				return false
			}
			if got, ok := execution.ResultField(result.Data, "next"); ok {
				return got == "euclo.execute_thoughtrecipe"
			}
			return false
		}},
		{"euclo.route_fork", "euclo.execute_capability", func(result *execution.Result, _ *contextdata.Envelope) bool {
			if result == nil {
				return false
			}
			if got, ok := execution.ResultField(result.Data, "next"); ok {
				return got == "euclo.execute_capability"
			}
			return false
		}},
		{"euclo.execute_thoughtrecipe", "euclo.merge", nil},
		{"euclo.execute_capability", "euclo.merge", nil},
		{"euclo.merge", "euclo.report", nil},
		{"euclo.report", "euclo.done", nil},
	}

	for _, edge := range edges {
		if err := g.AddEdge(edge.from, edge.to, edge.cond, false); err != nil {
			return err
		}
	}
	return nil
}

func ensureClarificationThoughtRecipe(reg *thoughtrecipepkg.ThoughtRecipeRegistry) {
	if reg == nil {
		return
	}
	if _, ok := reg.Get(clarificationThoughtRecipeID); ok {
		return
	}
	thoughtrecipe := &surface.ThoughtRecipe{
		ID:   clarificationThoughtRecipeID,
		Name: "intent clarification",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "intent clarification",
		},
	}
	plan := &thoughtrecipepkg.ExecutionPlan{
		ThoughtRecipe: thoughtrecipe,
		Steps: []thoughtrecipepkg.ExecutionStep{{
			ID:           clarificationThoughtRecipeID + ".step0",
			Type:         "run",
			Paradigm:     "goalcon",
			CapabilityID: clarificationCapabilityID,
			Prompt:       "Clarify the user's request.",
			Goal:         "Clarify the user's request.",
			Step: surface.ThoughtRecipeStep{
				ID:      clarificationThoughtRecipeID + ".step0",
				Type:    "run",
				Prompt:  "Clarify the user's request.",
				Parent:  surface.ThoughtRecipeStepAgent{Paradigm: "goalcon"},
				Config:  map[string]any{},
				Context: surface.ThoughtRecipeStepContext{},
			},
		}},
	}
	_, _ = reg.RegisterCompiledFirstWins(thoughtrecipe, plan, "built-in clarification route")
}

func seedPolicyDefaults(env *contextdata.Envelope) {
	euclostate.SeedPolicyDefaults(env)
}

func seedDefaultTask(env *contextdata.Envelope) {
	if _, ok := contextdata.GetTyped[any](env, euclostate.KeyTaskInput); ok {
		return
	}
	if _, ok := contextdata.GetTyped[any](env, euclostate.KeyTaskRaw); ok {
		return
	}
	task := &execution.Task{
		ID:          strings.TrimSpace(env.TaskID),
		Type:        "euclo",
		Instruction: "",
		Data:        map[string]any{},
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}
	if task.ID == "" {
		task.ID = "euclo.task"
	}
	env.SetWorkingValueWithClass(euclostate.KeyTaskInput, task, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass(euclostate.KeyTaskRaw, task, contextdata.MemoryClassTask)
}

func defaultFamilyRegistry() *families.KeywordFamilyRegistry {
	reg := families.NewRegistry()
	if err := families.RegisterBuiltins(reg); err != nil {
		return reg
	}
	return reg
}

type permissiveHITLBroker struct{}

func (permissiveHITLBroker) RequestPermission(_ context.Context, req authorization.PermissionRequest) (*authorization.PermissionGrant, error) {
	now := time.Now().UTC()
	return &authorization.PermissionGrant{
		ID:         "euclo-auto-approve",
		Permission: req.Permission,
		Scope:      req.Scope,
		ExpiresAt:  now.Add(5 * time.Minute),
		ApprovedBy: "euclo",
		Conditions: map[string]string{},
		GrantedAt:  now,
	}, nil
}

type stageNode struct {
	id       string
	nodeType agentgraph.NodeType
	execFn   func(context.Context, *contextdata.Envelope) (*execution.Result, error)
}

func newStageNode(id string, nodeType agentgraph.NodeType, execFn func(context.Context, *contextdata.Envelope) (*execution.Result, error)) *stageNode {
	return &stageNode{id: id, nodeType: nodeType, execFn: execFn}
}

func (n *stageNode) ID() string                { return n.id }
func (n *stageNode) Type() agentgraph.NodeType { return n.nodeType }
func (n *stageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	return n.execFn(ctx, env)
}
