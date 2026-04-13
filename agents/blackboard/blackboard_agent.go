package blackboard

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
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
	Memory memory.MemoryStore
	// Config holds runtime configuration.
	Config *core.Config
	// Sources is the set of knowledge sources evaluated each cycle.
	// When empty, DefaultKnowledgeSources() is used.
	Sources []KnowledgeSource
	// CheckpointPath is an optional filesystem path for checkpoint storage.
	CheckpointPath string
	// MaxCycles is the upper bound on control-loop iterations (default 20).
	MaxCycles int

	// SemanticContext is the pre-resolved semantic context bundle passed
	// to the agent at construction time. It seeds the blackboard with
	// AST symbols and BKC chunks before the first KS cycle.
	SemanticContext core.AgentSemanticContext

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
func (a *BlackboardAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityCode,
		core.CapabilityReview,
	}
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
	var retrieveDeclarative *graph.RetrieveDeclarativeMemoryNode
	var retrieveProcedural *graph.RetrieveProceduralMemoryNode
	if blackboardUsesDeclarativeRetrieval(a.Config) && a.Memory != nil {
		retrieveDeclarative = graph.NewRetrieveDeclarativeMemoryNode("bb_retrieve_declarative", blackboardScopedMemoryRetriever{
			store:       a.Memory,
			scope:       memory.MemoryScopeProject,
			memoryClass: core.MemoryClassDeclarative,
		})
		retrieveDeclarative.Query = taskInstruction(task)
		retrieveProcedural = graph.NewRetrieveProceduralMemoryNode("bb_retrieve_procedural", blackboardScopedMemoryRetriever{
			store:       a.Memory,
			scope:       memory.MemoryScopeProject,
			memoryClass: core.MemoryClassProcedural,
		})
		retrieveProcedural.Query = taskInstruction(task)
	}
	load := &blackboardLoadNode{id: "bb_load", goal: goal, maxCycles: maxCycles(a.MaxCycles)}
	evaluate := &blackboardEvaluateNode{id: "bb_evaluate", controller: controller}
	dispatch := &blackboardDispatchNode{id: "bb_dispatch", controller: controller, tools: a.Tools, model: a.Model, semctx: a.SemanticContext}
	if cfg := a.Config; cfg != nil {
		load.telemetry = cfg.Telemetry
		evaluate.telemetry = cfg.Telemetry
		dispatch.telemetry = cfg.Telemetry
	}
	var summarize *graph.SummarizeContextNode
	var persist *graph.PersistenceWriterNode
	if blackboardUsesStructuredPersistence(a.Config) {
		if runtimeStore := blackboardRuntimeStore(a.Memory); runtimeStore != nil {
			summarize = graph.NewSummarizeContextNode("bb_summarize", &core.SimpleSummarizer{})
			summarize.IncludeHistory = false
			summarize.StateKeys = []string{
				contextKeySummary,
				contextKeyController,
				contextKeyMetrics,
				contextKeyPersistenceSummary,
				contextKeyPersistenceDecision,
				contextKeyPersistenceRoutine,
			}
			if cfg := a.Config; cfg != nil {
				summarize.Telemetry = cfg.Telemetry
			}
			persist = graph.NewPersistenceWriterNode("bb_persist", runtimeStore)
			persist.TaskID = taskID(task)
			if cfg := a.Config; cfg != nil {
				persist.Telemetry = cfg.Telemetry
			}
			persist.Declarative = []graph.DeclarativePersistenceRequest{
				{
					StateKey:            contextKeyPersistenceSummary,
					Scope:               string(memory.MemoryScopeProject),
					Kind:                graph.DeclarativeKindProjectKnowledge,
					Title:               taskInstruction(task),
					SummaryField:        "summary",
					ContentField:        "result",
					ArtifactRefStateKey: "graph.summary_ref",
					Tags:                []string{"blackboard", "summary"},
					Reason:              "blackboard-summary",
				},
				{
					StateKey:     contextKeyPersistenceDecision,
					Scope:        string(memory.MemoryScopeProject),
					Kind:         graph.DeclarativeKindDecision,
					Title:        taskInstruction(task),
					SummaryField: "summary",
					ContentField: "decision",
					Tags:         []string{"blackboard", "decision"},
					Reason:       "blackboard-decision",
				},
			}
			persist.Procedural = []graph.ProceduralPersistenceRequest{{
				StateKey:         contextKeyPersistenceRoutine,
				Scope:            string(memory.MemoryScopeProject),
				Kind:             graph.ProceduralKindRoutine,
				NameField:        "name",
				SummaryField:     "summary",
				DescriptionField: "description",
				InlineBodyField:  "inline_body",
				VerifiedField:    "verified",
				Reason:           "blackboard-routine",
			}}
			persist.Artifacts = []graph.ArtifactPersistenceRequest{{
				ArtifactRefStateKey: "graph.summary_ref",
				SummaryStateKey:     "graph.summary",
				Reason:              "blackboard-summary-artifact",
			}}
		}
	}
	done := graph.NewTerminalNode("bb_done")
	nodes := make([]graph.Node, 0, 8)
	if retrieveDeclarative != nil {
		nodes = append(nodes, retrieveDeclarative)
	}
	if retrieveProcedural != nil {
		nodes = append(nodes, retrieveProcedural)
	}
	nodes = append(nodes, load, evaluate, dispatch)
	if summarize != nil {
		nodes = append(nodes, summarize)
	}
	if persist != nil {
		nodes = append(nodes, persist)
	}
	nodes = append(nodes, done)
	nextAfterDispatch := evaluate.ID()
	nextAfterDoneDecision := done.ID()
	if summarize != nil {
		nextAfterDoneDecision = summarize.ID()
		if persist != nil {
			nextAfterDoneDecision = summarize.ID()
		}
	}
	if persist != nil {
		nextAfterDoneDecision = summarize.ID()
	}
	if blackboardUsesExplicitCheckpointNodes(a.Config) && a.CheckpointPath != "" && task != nil && task.ID != "" {
		cycleCheckpoint := graph.NewCheckpointNode("bb_checkpoint_cycle", evaluate.ID(), memory.NewCheckpointStore(filepath.Clean(a.CheckpointPath)))
		cycleCheckpoint.TaskID = task.ID
		terminalNext := done.ID()
		if summarize != nil {
			terminalNext = summarize.ID()
		}
		terminalCheckpoint := graph.NewCheckpointNode("bb_checkpoint_done", terminalNext, memory.NewCheckpointStore(filepath.Clean(a.CheckpointPath)))
		terminalCheckpoint.TaskID = task.ID
		nodes = append(nodes, cycleCheckpoint, terminalCheckpoint)
		nextAfterDispatch = cycleCheckpoint.ID()
		nextAfterDoneDecision = terminalCheckpoint.ID()
	}
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if catalog := a.executionCapabilityCatalog(); catalog != nil && len(catalog.InspectableCapabilities()) > 0 {
		g.SetCapabilityCatalog(catalog)
	}
	startNodeID := load.ID()
	if retrieveDeclarative != nil {
		startNodeID = retrieveDeclarative.ID()
	} else if retrieveProcedural != nil {
		startNodeID = retrieveProcedural.ID()
	}
	if err := g.SetStart(startNodeID); err != nil {
		return nil, err
	}
	if retrieveDeclarative != nil {
		next := load.ID()
		if retrieveProcedural != nil {
			next = retrieveProcedural.ID()
		}
		if err := g.AddEdge(retrieveDeclarative.ID(), next, nil, false); err != nil {
			return nil, err
		}
	}
	if retrieveProcedural != nil {
		if err := g.AddEdge(retrieveProcedural.ID(), load.ID(), nil, false); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge(load.ID(), evaluate.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(evaluate.ID(), dispatch.ID(), func(result *core.Result, state *core.Context) bool {
		return state.GetString(contextKeyControllerNext) == dispatch.ID()
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(evaluate.ID(), nextAfterDoneDecision, func(result *core.Result, state *core.Context) bool {
		return state.GetString(contextKeyControllerNext) == done.ID()
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
		if summarize != nil {
			nextAfterSummarize := done.ID()
			if persist != nil {
				nextAfterSummarize = persist.ID()
			}
			if err := g.AddEdge(summarize.ID(), nextAfterSummarize, nil, false); err != nil {
				return nil, err
			}
		}
		if persist != nil {
			if err := g.AddEdge(persist.ID(), done.ID(), nil, false); err != nil {
				return nil, err
			}
		}
		if summarize == nil && persist == nil {
			if err := g.AddEdge(nextAfterDoneDecision, done.ID(), nil, false); err != nil {
				return nil, err
			}
		}
	}
	return g, nil
}

// Execute initialises the blackboard with the task goal and runs the controller
// loop until the goal is satisfied or an error occurs.
func (a *BlackboardAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
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
	if state == nil {
		state = core.NewContext()
	}
	if task != nil {
		state.Set("task.id", task.ID)
		state.Set("task.type", string(task.Type))
		state.Set("task.instruction", task.Instruction)
	}
	if cfg := a.Config; cfg != nil {
		emitBlackboardEvent(cfg.Telemetry, state, core.EventAgentStart, "", taskID(task), "blackboard agent start", map[string]any{
			"checkpoint_path": a.CheckpointPath,
			"max_cycles":      maxCycles(a.MaxCycles),
			"source_count":    len(a.Sources),
		})
	}

	g, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		g.SetTelemetry(cfg.Telemetry)
	}
	if !blackboardUsesExplicitCheckpointNodes(a.Config) && a.CheckpointPath != "" && task != nil && task.ID != "" {
		store := memory.NewCheckpointStore(filepath.Clean(a.CheckpointPath))
		g.WithCheckpointing(1, store.Save)
	}
	if checkpoint, err := a.loadResumeCheckpoint(state, task); err != nil {
		return nil, err
	} else if checkpoint != nil {
		if cfg := a.Config; cfg != nil {
			emitBlackboardEvent(cfg.Telemetry, state, core.EventStateChange, "", taskID(task), "blackboard resume requested", map[string]any{
				"checkpoint_id": checkpoint.CheckpointID,
				"resume_node":   checkpoint.NextNodeID,
			})
		}
		if _, err := g.ResumeFromCheckpoint(ctx, checkpoint); err != nil {
			return nil, fmt.Errorf("blackboard: resume failed: %w", err)
		}
		if state != checkpoint.Context {
			state.Merge(checkpoint.Context)
		}
	} else if _, err := g.Execute(ctx, state); err != nil {
		bb := LoadFromContext(state, taskInstruction(task))
		PublishToContext(state, bb, ControllerState{
			Cycle:       currentCycle(state),
			MaxCycles:   maxCycles(a.MaxCycles),
			Termination: "controller_error",
			LastSource:  state.GetString(contextKnowledgeLastSource),
		})
		if cfg := a.Config; cfg != nil {
			emitBlackboardEvent(cfg.Telemetry, state, core.EventAgentFinish, "", taskID(task), "blackboard agent failed", map[string]any{
				"status":      "error",
				"termination": "controller_error",
				"error":       err.Error(),
			})
		}
		return nil, fmt.Errorf("blackboard: graph execution failed: %w", err)
	}
	mirrorBlackboardArtifactReferences(state)
	compactBlackboardPostExecutionState(state)
	bb := LoadFromContext(state, taskInstruction(task))
	controllerRaw, _ := state.Get(contextKeyController)
	controllerState, _ := controllerRaw.(ControllerState)
	switch controllerState.Termination {
	case "goal_satisfied":
	case "running":
		controllerState.Termination = "goal_satisfied"
		PublishToContext(state, bb, controllerState)
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
		emitBlackboardEvent(cfg.Telemetry, state, core.EventAgentFinish, "", taskID(task), "blackboard agent finished", map[string]any{
			"status":          "success",
			"termination":     controllerState.Termination,
			"cycle":           controllerState.Cycle,
			"goal_satisfied":  controllerState.GoalSatisfied,
			"artifact_count":  len(bb.Artifacts),
			"completed_count": len(bb.CompletedActions),
		})
	}

	// Collect artifact contents for the result payload.
	artifactSummaries := make([]string, 0, len(bb.Artifacts))
	for _, art := range bb.Artifacts {
		artifactSummaries = append(artifactSummaries, fmt.Sprintf("[%s] %s: %s", art.Kind, art.ID, art.Content))
	}

	return &core.Result{
		Success: true,
		Data: map[string]any{
			"artifacts":       artifactSummaries,
			"artifact_count":  len(bb.Artifacts),
			"fact_count":      len(bb.Facts),
			"issue_count":     len(bb.Issues),
			"completed_count": len(bb.CompletedActions),
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

func compactBlackboardPostExecutionState(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get(contextKeySummaryRef); !ok {
		return
	}
	rawAudit, ok := state.Get(contextKeyAuditTrail)
	if !ok {
		return
	}
	entries, ok := rawAudit.([]map[string]any)
	if !ok {
		return
	}
	state.Set(contextKeyAuditTrail, compactBlackboardAudit(entries))
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

func mirrorBlackboardArtifactReferences(state *core.Context) {
	if state == nil {
		return
	}
	if strings.TrimSpace(state.GetString(contextKeySummary)) != "" {
		if rawRef, ok := state.Get("graph.summary_ref"); ok {
			if ref, ok := rawRef.(core.ArtifactReference); ok {
				state.Set(contextKeySummaryRef, ref)
			}
		}
		if summary := strings.TrimSpace(state.GetString("graph.summary")); summary != "" {
			state.Set(contextKeySummaryArtifactSummary, summary)
		}
	}
	if rawRef, ok := state.Get("graph.checkpoint_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			state.Set(contextKeyCheckpointRef, ref)
		}
	}
}

func (a *BlackboardAgent) loadResumeCheckpoint(state *core.Context, task *core.Task) (*graph.GraphCheckpoint, error) {
	if a == nil || a.CheckpointPath == "" || task == nil || strings.TrimSpace(task.ID) == "" {
		return nil, nil
	}
	if blackboardUsesExplicitCheckpointNodes(a.Config) {
		return nil, nil
	}
	store := memory.NewCheckpointStore(filepath.Clean(a.CheckpointPath))
	checkpointID := ""
	if state != nil {
		checkpointID = strings.TrimSpace(state.GetString(contextKeyResumeCheckpointID))
	}
	if checkpointID != "" {
		return store.Load(task.ID, checkpointID)
	}
	resumeLatest := false
	if state != nil {
		if raw, ok := state.Get(contextKeyResumeLatest); ok {
			if flag, ok := raw.(bool); ok {
				resumeLatest = flag
			}
		}
	}
	if !resumeLatest {
		return nil, nil
	}
	checkpoints, err := store.List(task.ID)
	if err != nil || len(checkpoints) == 0 {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(checkpoints)))
	return store.Load(task.ID, checkpoints[0])
}

func blackboardUsesExplicitCheckpointNodes(cfg *core.Config) bool {
	if cfg == nil || cfg.UseExplicitCheckpointNodes == nil {
		return false
	}
	return *cfg.UseExplicitCheckpointNodes
}

func blackboardUsesDeclarativeRetrieval(cfg *core.Config) bool {
	if cfg == nil || cfg.UseDeclarativeRetrieval == nil {
		return true
	}
	return *cfg.UseDeclarativeRetrieval
}

func blackboardUsesStructuredPersistence(cfg *core.Config) bool {
	if cfg == nil || cfg.UseStructuredPersistence == nil {
		return true
	}
	return *cfg.UseStructuredPersistence
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
