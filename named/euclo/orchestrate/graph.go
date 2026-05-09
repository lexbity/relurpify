package orchestrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/persistence"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// RootGraph wires together orchestration nodes using the agentgraph runtime.
type RootGraph struct {
	graph *agentgraph.Graph
}

// RootGraphConfig configures dependency wiring for the root graph.
type RootGraphConfig struct {
	Env                   agentenv.WorkspaceEnvironment
	CapabilityRegistry    *capability.CapabilityRegistry
	ThoughtRecipeRegistry *thoughtrecipepkg.ThoughtRecipeRegistry
	FamilyRegistry        *families.KeywordFamilyRegistry
	Workspace             string
	StreamTrigger         *contextstream.Trigger
	MaxStreamTokens       int
	DefaultStreamMode     contextstream.Mode
	CapabilityClassifier  intake.Tier2Classifier
	PermissionManager     policy.PermissionManager
	HITLBroker            policy.HITLBroker
	CheckpointRepository  agentlifecycle.Repository
	PersistenceWriter     *persistence.Writer
}

// RootGraphOption mutates RootGraphConfig.
type RootGraphOption func(*RootGraphConfig)

// WithWorkspaceEnvironment wires the workspace environment.
func WithWorkspaceEnvironment(env agentenv.WorkspaceEnvironment) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.Env = env
	}
}

// WithCapabilityRegistry wires the capability registry.
func WithCapabilityRegistry(reg *capability.CapabilityRegistry) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.CapabilityRegistry = reg
	}
}

// WithThoughtRecipeRegistry wires the thoughtrecipe registry.
func WithThoughtRecipeRegistry(reg *thoughtrecipepkg.ThoughtRecipeRegistry) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.ThoughtRecipeRegistry = reg
	}
}

// WithFamilyRegistry wires the family registry.
func WithFamilyRegistry(reg *families.KeywordFamilyRegistry) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.FamilyRegistry = reg
	}
}

// WithWorkspace wires the workspace root.
func WithWorkspace(workspace string) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.Workspace = strings.TrimSpace(workspace)
	}
}

// WithStreamTrigger wires the context stream trigger.
func WithStreamTrigger(trigger *contextstream.Trigger) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.StreamTrigger = trigger
	}
}

// WithMaxStreamTokens sets the stream token budget.
func WithMaxStreamTokens(tokens int) RootGraphOption {
	return func(opts *RootGraphConfig) {
		if tokens > 0 {
			opts.MaxStreamTokens = tokens
		}
	}
}

// WithDefaultStreamMode sets the default stream mode.
func WithDefaultStreamMode(mode contextstream.Mode) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.DefaultStreamMode = mode
	}
}

// WithCapabilityClassifier sets the LLM-based classifier.
func WithCapabilityClassifier(classifier intake.Tier2Classifier) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.CapabilityClassifier = classifier
	}
}

// WithPermissionManager sets the permission manager for policy gates.
func WithPermissionManager(pm policy.PermissionManager) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.PermissionManager = pm
	}
}

// WithHITLBroker sets the HITL broker for policy gates.
func WithHITLBroker(broker policy.HITLBroker) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.HITLBroker = broker
	}
}

// WithCheckpointRepository wires the repository used for checkpoint persistence.
func WithCheckpointRepository(repo agentlifecycle.Repository) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.CheckpointRepository = repo
	}
}

// WithPersistenceWriter wires the optional persistence writer for mirrored checkpoint writes.
func WithPersistenceWriter(writer *persistence.Writer) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.PersistenceWriter = writer
	}
}

// NewRootGraph creates a new root graph with all components wired together.
func NewRootGraph(opts ...RootGraphOption) *RootGraph {
	cfg := RootGraphConfig{
		FamilyRegistry:    defaultFamilyRegistry(),
		MaxStreamTokens:   8192,
		DefaultStreamMode: contextstream.ModeBlocking,
		HITLBroker:        permissiveHITLBroker{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.FamilyRegistry == nil {
		cfg.FamilyRegistry = defaultFamilyRegistry()
	}
	if cfg.HITLBroker == nil {
		cfg.HITLBroker = permissiveHITLBroker{}
	}

	g := agentgraph.NewGraph()
	for _, node := range buildNodes(cfg) {
		if err := g.AddNode(node); err != nil {
			panic(err)
		}
	}
	if err := wireEdges(g); err != nil {
		panic(err)
	}
	if err := g.SetStart("euclo.intake"); err != nil {
		panic(err)
	}
	return &RootGraph{graph: g}
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

func buildNodes(cfg RootGraphConfig) []agentgraph.Node {
	dispatchCapReg := cfg.CapabilityRegistry
	thoughtrecipeReg := cfg.ThoughtRecipeRegistry
	if thoughtrecipeReg == nil {
		thoughtrecipeReg = thoughtrecipepkg.NewThoughtRecipeRegistry()
	}
	thoughtrecipeCapReg := cfg.Env.Registry
	if thoughtrecipeCapReg == nil {
		thoughtrecipeCapReg = capability.NewCapabilityRegistry()
		cfg.Env.Registry = thoughtrecipeCapReg
	}
	if err := registerClarificationCapability(thoughtrecipeCapReg); err != nil {
		panic(err)
	}
	intakePipeline := intake.NewIntakePipelineNode(
		"euclo.intake",
		cfg.FamilyRegistry,
		cfg.MaxStreamTokens,
		cfg.DefaultStreamMode,
		cfg.StreamTrigger,
		cfg.CapabilityClassifier,
	)
	intakeNode := newStageNode("euclo.intake", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
		seedDefaultTask(env)
		return intakePipeline.Execute(ctx, env)
	})

	familySelectNode := newStageNode("euclo.family_select", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
		if env == nil {
			return &core.Result{NodeID: "euclo.family_select", Success: true}, nil
		}
		family := ""
		if v, ok := env.GetWorkingValue("euclo.family_selection"); ok {
			if m, ok := v.(map[string]any); ok {
				if s, ok := m["winning_family"].(string); ok {
					family = strings.TrimSpace(s)
				}
			}
		}
		if family != "" {
			env.SetWorkingValue("euclo.family.selected", family, contextdata.MemoryClassTask)
		}
		return &core.Result{
			NodeID:  "euclo.family_select",
			Success: true,
			Data: map[string]any{
				"family": family,
			},
		}, nil
	})

	ingestionNode := NewIngestionNode("euclo.ingest")

	streamNode := newStageNode("euclo.stream", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
		if env == nil {
			return &core.Result{NodeID: "euclo.stream", Success: true}, nil
		}
		hasStream := false
		if _, ok := env.GetWorkingValue("euclo.stream_result"); ok {
			hasStream = true
		}
		env.SetWorkingValue("euclo.stream.requested", hasStream, contextdata.MemoryClassTask)
		return &core.Result{
			NodeID:  "euclo.stream",
			Success: true,
			Data: map[string]any{
				"streamed": hasStream,
			},
		}, nil
	})

	checkpointNode := agentgraph.NewCheckpointNode("euclo.checkpoint").
		WithRepository(cfg.CheckpointRepository).
		WithWriter(cfg.PersistenceWriter)

	capClassifyNode := newStageNode("euclo.capability_classify", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
		sequence := []string{}
		operator := ""
		if env != nil {
			if v, ok := env.GetWorkingValue("euclo.capability_sequence"); ok {
				if seq, ok := v.([]string); ok {
					sequence = append([]string(nil), seq...)
				}
			}
			if v, ok := env.GetWorkingValue("euclo.capability_operator"); ok {
				if s, ok := v.(string); ok {
					operator = strings.TrimSpace(s)
				}
			}
			env.SetWorkingValue("euclo.capability.classified", true, contextdata.MemoryClassTask)
		}
		return &core.Result{
			NodeID:  "euclo.capability_classify",
			Success: true,
			Data: map[string]any{
				"capability_sequence": sequence,
				"operator":            operator,
			},
		}, nil
	})

	interactionCheckNode := newStageNode("euclo.interaction_check", agentgraph.NodeTypeConditional, func(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
		needsInteraction := false
		if env != nil {
			if v, ok := env.GetWorkingValue("euclo.intent_classification"); ok {
				if classification, ok := v.(*intake.IntentClassification); ok && classification != nil {
					needsInteraction = classification.Ambiguous || classification.Confidence < 0.7
				}
			}
		}
		return &core.Result{
			NodeID:  "euclo.interaction_check",
			Success: true,
			Data: map[string]any{
				"needs_interaction": needsInteraction,
			},
		}, nil
	})

	interactionFrameNode := newStageNode("euclo.interaction_frame", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
		if env != nil {
			seedPolicyDefaults(env)
			env.SetWorkingValue("euclo.interaction.frame_requested", true, contextdata.MemoryClassTask)
		}
		return &core.Result{
			NodeID:  "euclo.interaction_frame",
			Success: true,
		}, nil
	})

	gate := policy.NewGateNode("euclo.policy_gate", policy.NewEvaluator()).
		WithPermissionManager(cfg.PermissionManager).
		WithHITLBroker(cfg.HITLBroker)
	policyGateNode := newStageNode("euclo.policy_gate", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
		if env != nil {
			seedPolicyDefaults(env)
		}
		data, err := gate.Execute(ctx, env)
		if err != nil {
			return &core.Result{NodeID: "euclo.policy_gate", Success: false, Data: map[string]any{"error": err.Error()}}, err
		}
		return &core.Result{
			NodeID:  "euclo.policy_gate",
			Success: true,
			Data:    data,
		}, nil
	})

	dispatchNode := NewDispatcher("euclo.dispatch").
		WithWorkspace(cfg.Workspace).
		WithCapabilityRegistry(dispatchCapReg).
		WithThoughtRecipeRegistry(thoughtrecipeReg)

	routeForkNode := NewRouteForkNode("euclo.route_fork")

	thoughtrecipeExec := NewThoughtRecipeExecutorNode("euclo.execute_thoughtrecipe").
		WithWorkspaceEnvironment(cfg.Env).
		WithIngestionPipeline(nil)
	thoughtrecipeExec.WithThoughtRecipeRegistry(thoughtrecipeReg)

	capabilityExec := NewCapabilityExecutionNode("euclo.execute_capability")
	if dispatchCapReg != nil {
		capabilityExec.WithCapabilityRegistry(dispatchCapReg)
	}

	mergeNode := NewMergeNode("euclo.merge")

	telemetryNode := reporting.NewTelemetryNode("euclo.report")
	reportNode := newStageNode("euclo.report", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
		if env != nil {
			if v, ok := env.GetWorkingValue("euclo.route.outcome"); ok {
				if s, ok := v.(string); ok && strings.EqualFold(strings.TrimSpace(s), string(reporting.RouteOutcomeDryRun)) {
					env.SetWorkingValue("euclo.execution.completed", true, contextdata.MemoryClassTask)
				}
			}
		}
		result, err := telemetryNode.Execute(ctx, env)
		if err != nil {
			return &core.Result{NodeID: "euclo.report", Success: false, Data: map[string]any{"error": err.Error()}}, err
		}
		return &core.Result{
			NodeID:  "euclo.report",
			Success: true,
			Data:    result,
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
	}
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
		{"euclo.interaction_check", "euclo.interaction_frame", func(result *core.Result, _ *contextdata.Envelope) bool {
			return result != nil && result.Data != nil && result.Data["needs_interaction"] == true
		}},
		{"euclo.interaction_check", "euclo.policy_gate", func(result *core.Result, _ *contextdata.Envelope) bool {
			return result == nil || result.Data == nil || result.Data["needs_interaction"] != true
		}},
		{"euclo.interaction_frame", "euclo.policy_gate", nil},
		{"euclo.policy_gate", "euclo.dispatch", nil},
		{"euclo.dispatch", "euclo.report", func(_ *core.Result, env *contextdata.Envelope) bool {
			if env == nil {
				return false
			}
			if v, ok := env.GetWorkingValue("euclo.route.outcome"); ok {
				if s, ok := v.(string); ok && strings.EqualFold(strings.TrimSpace(s), string(reporting.RouteOutcomeDryRun)) {
					return true
				}
			}
			if v, ok := env.GetWorkingValue("euclo.dry_run_mode"); ok {
				if dryRun, ok := v.(bool); ok && dryRun {
					return true
				}
			}
			return false
		}},
		{"euclo.dispatch", "euclo.route_fork", func(_ *core.Result, env *contextdata.Envelope) bool {
			if env == nil {
				return true
			}
			if v, ok := env.GetWorkingValue("euclo.route.outcome"); ok {
				if s, ok := v.(string); ok && strings.EqualFold(strings.TrimSpace(s), string(reporting.RouteOutcomeDryRun)) {
					return false
				}
			}
			if v, ok := env.GetWorkingValue("euclo.dry_run_mode"); ok {
				if dryRun, ok := v.(bool); ok && dryRun {
					return false
				}
			}
			return true
		}},
		{"euclo.route_fork", "euclo.execute_thoughtrecipe", func(result *core.Result, _ *contextdata.Envelope) bool {
			return result != nil && result.Data != nil && result.Data["next"] == "euclo.execute_thoughtrecipe"
		}},
		{"euclo.route_fork", "euclo.execute_capability", func(result *core.Result, _ *contextdata.Envelope) bool {
			return result != nil && result.Data != nil && result.Data["next"] == "euclo.execute_capability"
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

func seedPolicyDefaults(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if _, ok := env.GetWorkingValue("euclo.task_envelope.edit_permitted"); !ok {
		env.SetWorkingValue("euclo.task_envelope.edit_permitted", true, contextdata.MemoryClassTask)
	}
	if _, ok := env.GetWorkingValue("euclo.policy.risk_level"); !ok {
		env.SetWorkingValue("euclo.policy.risk_level", "low", contextdata.MemoryClassTask)
	}
	if _, ok := env.GetWorkingValue("euclo.policy.verification_required"); !ok {
		env.SetWorkingValue("euclo.policy.verification_required", false, contextdata.MemoryClassTask)
	}
}

func seedDefaultTask(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if _, ok := env.GetWorkingValue("task.input"); ok {
		return
	}
	if _, ok := env.GetWorkingValue("euclo.task.input"); ok {
		return
	}
	if _, ok := env.GetWorkingValue("euclo.task"); ok {
		return
	}
	task := &core.Task{
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
	env.SetWorkingValue("task.input", task, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.task.input", task, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.task", task, contextdata.MemoryClassTask)
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
	execFn   func(context.Context, *contextdata.Envelope) (*core.Result, error)
}

func newStageNode(id string, nodeType agentgraph.NodeType, execFn func(context.Context, *contextdata.Envelope) (*core.Result, error)) *stageNode {
	return &stageNode{id: id, nodeType: nodeType, execFn: execFn}
}

func (n *stageNode) ID() string                { return n.id }
func (n *stageNode) Type() agentgraph.NodeType { return n.nodeType }
func (n *stageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	return n.execFn(ctx, env)
}
