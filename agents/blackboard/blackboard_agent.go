package blackboard

import (
	"context"
	"fmt"
	"strings"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// BlackboardAgent implements graph.WorkflowExecutor using the Blackboard architecture.
// A shared Blackboard workspace is maintained across a control loop; multiple
// KnowledgeSource specialists read and write it each cycle. Execution order is
// data-driven rather than structurally predetermined.
type BlackboardAgent struct {
	// Model is the language model available to knowledge sources.
	Model core.LanguageModel
	// Tools is the capability registry available to knowledge sources.
	Tools *capability.Registry
	// Memory is the memory store for the agent.
	Memory *memory.WorkingMemoryStore
	// Config holds runtime configuration.
	Config *core.Config
	// Sources is the set of knowledge sources evaluated each cycle.
	// When empty, DefaultKnowledgeSources() is used.
	Sources []KnowledgeSource
	// MaxCycles is the upper bound on control-loop iterations (default 20).
	MaxCycles int

	StreamTrigger   *contextstream.Trigger
	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int

	// SemanticContext is the pre-resolved semantic context bundle passed
	// to the agent at construction time. It seeds the blackboard with
	// AST symbols and BKC chunks before the first KS cycle.
	SemanticContext agentspec.AgentSemanticContext

	initialised      bool
	executionCatalog *capability.ExecutionCapabilityCatalogSnapshot
}

// Initialize satisfies graph.WorkflowExecutor. It wires configuration and ensures
// knowledge sources are populated.
func (a *BlackboardAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	if len(a.Sources) == 0 {
		a.Sources = DefaultKnowledgeSources()
	}
	a.initialised = true
	return nil
}

// Capabilities declares what this agent can do.
func (a *BlackboardAgent) Capabilities() []string {
	return []string{"blackboard"}
}

// BuildGraph returns the graph-native blackboard controller loop. The
// blackboard-specific scheduling logic lives in agent-owned nodes and state,
// without extending framework/graph.
func (a *BlackboardAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	goal := ""
	if task != nil {
		goal = task.Instruction
	}
	g := graph.NewGraph()
	controller := &Controller{
		Sources:   a.Sources,
		MaxCycles: a.MaxCycles,
	}
	stream := a.streamTriggerNode(task)
	taskID := ""
	if task != nil {
		taskID = task.ID
	}
	load := &blackboardLoadNode{
		id:        "bb_load",
		goal:      goal,
		maxCycles: maxCycles(a.MaxCycles),
		taskID:    taskID,
		store:     a.Memory,
	}
	evaluate := &blackboardEvaluateNode{id: "bb_evaluate", controller: controller}
	dispatch := &blackboardDispatchNode{id: "bb_dispatch", controller: controller, tools: a.Tools, model: a.Model, semctx: a.SemanticContext}
	if cfg := a.Config; cfg != nil {
		load.telemetry = cfg.Telemetry
		evaluate.telemetry = cfg.Telemetry
		dispatch.telemetry = cfg.Telemetry
	}
	done := graph.NewTerminalNode("bb_done")
	nodes := make([]graph.Node, 0, 5)
	if stream != nil {
		nodes = append(nodes, stream)
	}
	nodes = append(nodes, load, evaluate, dispatch, done)
	nextAfterDispatch := evaluate.ID()
	nextAfterDoneDecision := done.ID()
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if catalog := a.executionCapabilityCatalog(); catalog != nil && len(catalog.InspectableCapabilities()) > 0 {
		g.SetCapabilityCatalog(catalog)
	}
	startNodeID := load.ID()
	if stream != nil {
		startNodeID = stream.ID()
	}
	if err := g.SetStart(startNodeID); err != nil {
		return nil, err
	}
	// Connect stream -> load if streaming is enabled, otherwise connect directly
	if stream != nil {
		if err := g.AddEdge(stream.ID(), load.ID(), nil, false); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge(load.ID(), evaluate.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(evaluate.ID(), dispatch.ID(), func(result *core.Result, env *contextdata.Envelope) bool {
		return envGetString(env, contextKeyControllerNext) == dispatch.ID()
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(evaluate.ID(), nextAfterDoneDecision, func(result *core.Result, env *contextdata.Envelope) bool {
		return envGetString(env, contextKeyControllerNext) == done.ID()
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(dispatch.ID(), nextAfterDispatch, nil, false); err != nil {
		return nil, err
	}
	if nextAfterDispatch != evaluate.ID() {
		if err := g.AddEdge(nextAfterDispatch, evaluate.ID(), nil, false); err != nil {
			return nil, err
		}
	}
	if nextAfterDoneDecision != done.ID() {
		if err := g.AddEdge(nextAfterDoneDecision, done.ID(), nil, false); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func envGetString(env *contextdata.Envelope, key string) string {
	val, _ := env.GetWorkingValue(key)
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

// Execute initialises the blackboard with the task goal and runs the controller
// loop until the goal is satisfied or an error occurs.
func (a *BlackboardAgent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	a.executionCatalog = nil
	if a.Tools != nil {
		a.executionCatalog = a.Tools.CaptureExecutionCatalogSnapshot()
	}
	defer func() {
		a.executionCatalog = nil
	}()
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if env == nil {
		env = contextdata.NewEnvelope("blackboard", "session")
	}
	if task != nil {
		env.SetWorkingValue("task.id", task.ID, contextdata.MemoryClassTask)
		env.SetWorkingValue("task.type", string(task.Type), contextdata.MemoryClassTask)
		env.SetWorkingValue("task.instruction", task.Instruction, contextdata.MemoryClassTask)
	}
	if a.Memory != nil {
		env.SetWorkingValue(contextKeyWorkingMemoryStore, a.Memory, contextdata.MemoryClassTask)
	}
	if cfg := a.Config; cfg != nil {
		// Telemetry event emission - keep commented until envelope equivalent available
		// emitBlackboardEvent(cfg.Telemetry, env, core.EventAgentStart, "", taskID(task), "blackboard agent start", map[string]any{
		// 	"checkpoint_path": a.CheckpointPath,
		// 	"max_cycles":      maxCycles(a.MaxCycles),
		// 	"source_count":    len(a.Sources),
		// })
	}

	g, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		g.SetTelemetry(cfg.Telemetry)
	}
	if _, err := g.Execute(ctx, env); err != nil {
		return nil, fmt.Errorf("blackboard: graph execution failed: %w", err)
	}
	controllerState := ControllerState{Termination: "goal_satisfied"}
	if raw, ok := env.GetWorkingValue(contextKeyController); ok {
		if typed, ok := raw.(ControllerState); ok {
			controllerState = typed
		}
	}
	switch controllerState.Termination {
	case "goal_satisfied":
	case "running":
		controllerState.Termination = "goal_satisfied"
	case "cycle_limit":
		return nil, fmt.Errorf("blackboard: reached cycle limit (%d) without satisfying goal", controllerState.MaxCycles)
	case "stuck":
		return nil, fmt.Errorf("blackboard: no knowledge source can activate — goal not satisfied")
	default:
		if controllerState.Termination != "" {
			return nil, fmt.Errorf("blackboard: terminated with status %s", controllerState.Termination)
		}
		return nil, fmt.Errorf("blackboard: controller terminated without status")
	}
	if cfg := a.Config; cfg != nil {
		// Telemetry event emission - agent-specific
		// emitBlackboardEvent(cfg.Telemetry, env, core.EventAgentFinish, "", taskID(task), "blackboard agent finished", map[string]any{
		// 	"status":          "success",
		// 	"termination":     controllerState.Termination,
		// 	"cycle":           controllerState.Cycle,
		// 	"goal_satisfied":  controllerState.GoalSatisfied,
		// 	"artifact_count":  len(bb.Artifacts),
		// 	"completed_count": len(bb.CompletedActions),
		// })
	}

	// Collect artifact contents for the result payload.
	// Framework: load artifacts from context
	// bb := LoadFromContext(env, taskInstruction(task))
	// Agent-specific artifact loading
	artifactSummaries := []string{}

	return &core.Result{
		Success: true,
		Data: map[string]any{
			"artifacts":       artifactSummaries,
			"artifact_count":  0,
			"fact_count":      0,
			"issue_count":     0,
			"completed_count": 0,
		},
	}, nil
}

func (a *BlackboardAgent) executionCapabilityCatalog() *capability.ExecutionCapabilityCatalogSnapshot {
	if a == nil {
		return nil
	}
	if a.executionCatalog != nil {
		return a.executionCatalog
	}
	if a.Tools == nil {
		return nil
	}
	return a.Tools.CaptureExecutionCatalogSnapshot()
}

func compactBlackboardPostExecutionState(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if _, ok := env.GetWorkingValue(contextKeySummaryRef); !ok {
		return
	}
	rawAudit, ok := env.GetWorkingValue(contextKeyAuditTrail)
	if !ok {
		return
	}
	entries, ok := rawAudit.([]map[string]any)
	if !ok {
		return
	}
	env.SetWorkingValue(contextKeyAuditTrail, compactBlackboardAudit(entries), contextdata.MemoryClassTask)
}

func compactBlackboardAudit(entries []map[string]any) map[string]any {
	value := map[string]any{
		"entry_count": len(entries),
	}
	if len(entries) == 0 {
		return value
	}
	last := entries[len(entries)-1]
	value["last_message"] = strings.TrimSpace(fmt.Sprint(last["message"]))
	value["first_message"] = strings.TrimSpace(fmt.Sprint(entries[0]["message"]))
	return value
}

func mirrorBlackboardArtifactReferences(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if strings.TrimSpace(envGetString(env, contextKeySummary)) != "" {
		if rawRef, ok := env.GetWorkingValue("graph.summary_ref"); ok {
			if ref, ok := rawRef.(core.ArtifactReference); ok {
				env.SetWorkingValue(contextKeySummaryRef, ref, contextdata.MemoryClassTask)
			}
		}
		if summary := strings.TrimSpace(envGetString(env, "graph.summary")); summary != "" {
			env.SetWorkingValue(contextKeySummaryArtifactSummary, summary, contextdata.MemoryClassTask)
		}
	}
	if rawRef, ok := env.GetWorkingValue("graph.checkpoint_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			env.SetWorkingValue(contextKeyCheckpointRef, ref, contextdata.MemoryClassTask)
		}
	}
}

func blackboardUsesStructuredPersistence(cfg *core.Config) bool {
	_ = cfg
	return true
}

func taskID(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ID)
}

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}

func maxCycles(max int) int {
	if max <= 0 {
		return defaultMaxCycles
	}
	return max
}

// streamMode returns the streaming mode, defaulting to blocking.
func (a *BlackboardAgent) streamMode() contextstream.Mode {
	if a.StreamMode != "" {
		return a.StreamMode
	}
	return contextstream.ModeBlocking
}

// streamQuery returns the query for streaming, defaulting to task instruction.
func (a *BlackboardAgent) streamQuery(task *core.Task) string {
	if a.StreamQuery != "" {
		return a.StreamQuery
	}
	if task != nil {
		return task.Instruction
	}
	return ""
}

// streamMaxTokens returns the max tokens for streaming, defaulting to 256.
func (a *BlackboardAgent) streamMaxTokens() int {
	if a.StreamMaxTokens > 0 {
		return a.StreamMaxTokens
	}
	return 256
}

// streamTriggerNode creates a streaming trigger node for the blackboard agent.
func (a *BlackboardAgent) streamTriggerNode(task *core.Task) graph.Node {
	if a.StreamTrigger == nil {
		return nil
	}
	query := a.streamQuery(task)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	node := graph.NewContextStreamNode("blackboard_stream", a.StreamTrigger, retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	node.BudgetShortfallPolicy = "emit_partial"
	node.Metadata = map[string]any{
		"agent": "blackboard",
		"stage": "pre_control_loop",
	}
	return node
}
