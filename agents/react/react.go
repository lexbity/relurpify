package react

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	frameworksearch "github.com/lexcodex/relurpify/framework/search"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
)

// ReActAgent implements the Reason+Act pattern.
// ModeRuntimeProfile conveys high-level runtime settings to the agent.
type ModeRuntimeProfile struct {
	Name        string
	Description string
	Temperature float64
	Context     ContextPreferences
}

// ContextPreferences tune context management for a mode.
type ContextPreferences struct {
	PreferredDetailLevel contextmgr.DetailLevel
	MinHistorySize       int
	CompressionThreshold float64
}

// ReActAgent implements the Reason+Act pattern.
type ReActAgent struct {
	Model          core.LanguageModel
	Tools          *capability.Registry
	Memory         memory.MemoryStore
	Config         *core.Config
	IndexManager   *ast.IndexManager
	SearchEngine   *frameworksearch.SearchEngine
	Summarizer     core.Summarizer
	CheckpointPath string
	maxIterations  int
	contextPolicy  *contextmgr.ContextPolicy

	Mode             string
	ModeProfile      ModeRuntimeProfile
	sharedContext    *core.SharedContext
	initialLoadDone  bool
	executionCatalog *capability.ExecutionCapabilityCatalogSnapshot
}

const (
	contextmgrPhaseExplore = "explore"
	contextmgrPhaseEdit    = "edit"
	contextmgrPhaseVerify  = "verify"
)

// ToolObservation records a single tool execution result within the ReAct loop.
type ToolObservation struct {
	Tool      string                 `json:"tool"`
	Phase     string                 `json:"phase"`
	Summary   string                 `json:"summary"`
	Args      map[string]interface{} `json:"args,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Success   bool                   `json:"success"`
	Timestamp time.Time              `json:"timestamp"`
}

// Initialize wires configuration.
func (a *ReActAgent) Initialize(config *core.Config) error {
	a.Config = config
	if config.MaxIterations <= 0 {
		a.maxIterations = 8
	} else {
		a.maxIterations = config.MaxIterations
	}
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	if a.Mode == "" {
		a.Mode = "code"
	}
	if a.ModeProfile.Name == "" {
		a.ModeProfile = ModeRuntimeProfile{
			Name:        a.Mode,
			Description: "Reason + Act agent",
			Temperature: 0.2,
			Context: ContextPreferences{
				PreferredDetailLevel: contextmgr.DetailDetailed,
				MinHistorySize:       5,
				CompressionThreshold: 0.8,
			},
		}
	}
	strategy := contextmgr.ContextStrategy(nil)
	if a.contextPolicy != nil {
		strategy = a.contextPolicy.Strategy
	}
	if strategy == nil {
		switch strings.ToLower(a.Mode) {
		case "debug", "ask":
			strategy = contextmgr.NewAggressiveStrategy()
		case "architect":
			strategy = contextmgr.NewConservativeStrategy()
		default:
			strategy = contextmgr.NewAdaptiveStrategy()
		}
	}
	var spec *core.AgentContextSpec
	if config != nil && config.AgentSpec != nil {
		spec = &config.AgentSpec.Context
	}
	if a.contextPolicy == nil {
		a.contextPolicy = contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
			Strategy:      strategy,
			LanguageModel: a.Model,
			IndexManager:  a.IndexManager,
			SearchEngine:  a.SearchEngine,
			MemoryStore:   a.Memory,
			Summarizer:    a.Summarizer,
			Preferences: contextmgr.ContextPolicyPreferences{
				PreferredDetailLevel: a.ModeProfile.Context.PreferredDetailLevel,
				MinHistorySize:       a.ModeProfile.Context.MinHistorySize,
				CompressionThreshold: a.ModeProfile.Context.CompressionThreshold,
			},
		}, spec)
	} else {
		a.contextPolicy.Strategy = strategy
		a.contextPolicy.Preferences = contextmgr.ContextPolicyPreferences{
			PreferredDetailLevel: a.ModeProfile.Context.PreferredDetailLevel,
			MinHistorySize:       a.ModeProfile.Context.MinHistorySize,
			CompressionThreshold: a.ModeProfile.Context.CompressionThreshold,
		}
		a.contextPolicy.ApplyAgentContextSpec(spec)
	}
	a.contextPolicy.Budget.SetReservations(1000, 2000, 1000)
	return nil
}

// debugf logs formatted messages whenever agent debug logging is enabled.
func (a *ReActAgent) debugf(format string, args ...interface{}) {
	if a == nil || a.Config == nil || !a.Config.DebugAgent {
		return
	}
	log.Printf("[react] "+format, args...)
}

// Execute runs the task through the workflow graph.
func (a *ReActAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	a.executionCatalog = nil
	if a.Tools != nil {
		a.executionCatalog = a.Tools.CaptureExecutionCatalogSnapshot()
	}
	g, err := a.BuildGraph(task)
	if err != nil {
		a.executionCatalog = nil
		return nil, err
	}
	a.initialLoadDone = false
	a.sharedContext = core.NewSharedContext(state, a.contextPolicy.Budget, a.contextPolicy.Summarizer)
	if a.contextPolicy != nil && task != nil {
		if err := a.contextPolicy.InitialLoad(task); err != nil {
			a.debugf("initial context load failed: %v", err)
		} else {
			a.initialLoadDone = true
		}
	}
	defer func() {
		a.sharedContext = nil
		a.initialLoadDone = false
		a.executionCatalog = nil
	}()
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		g.SetTelemetry(cfg.Telemetry)
	}
	a.initializePhase(state, task)
	if !reactUsesExplicitCheckpointNodes(a.Config) && a.CheckpointPath != "" && task != nil && task.ID != "" {
		store := memory.NewCheckpointStore(filepath.Clean(a.CheckpointPath))
		g.WithCheckpointing(2, store.Save)
	}
	result, err := g.Execute(ctx, state)
	if err == nil && result != nil {
		if followErr := a.completeExplicitReadOnlyRetrieval(ctx, task, state); followErr != nil {
			a.debugf("explicit retrieval follow-up failed: %v", followErr)
		}
		rawLast, _ := state.Get("react.last_tool_result")
		lastMap, _ := rawLast.(map[string]interface{})
		if summary, ok := completionSummaryFromState(a, task, state, lastMap); ok {
			state.Set("react.incomplete_reason", "")
			state.Set("react.synthetic_summary", summary)
			state.Set("react.final_output", map[string]interface{}{
				"summary": summary,
				"result":  lastMap,
			})
			result.Success = true
			result.Error = nil
		} else if summary, ok := finalResultFallbackSummary(task, state); ok {
			state.Set("react.incomplete_reason", "")
			state.Set("react.synthetic_summary", summary)
			state.Set("react.final_output", map[string]interface{}{
				"summary": summary,
				"result":  lastMap,
			})
			result.Success = true
			result.Error = nil
		}
		if final, ok := state.Get("react.final_output"); ok {
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["final_output"] = final
			if summary := finalOutputSummary(final); summary != "" {
				result.Data["text"] = summary
			}
		}
		mirrorReactFinalOutputReference(state)
		compactReactFinalOutputState(state)
		compactReactLastToolResultState(state)
		compactReactToolObservationsState(state)
		compactReactLoopState(state)
		mirrorReactCheckpointReference(state)
		if reason := strings.TrimSpace(state.GetString("react.incomplete_reason")); reason != "" {
			result.Success = false
			result.Error = fmt.Errorf("%s", reason)
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["incomplete_reason"] = reason
		}
	}
	return result, err
}

func mirrorReactFinalOutputReference(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output"); !ok {
		return
	}
	if rawRef, ok := state.Get("graph.summary_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			state.Set("react.final_output_ref", ref)
		}
	}
	if summary := strings.TrimSpace(state.GetString("graph.summary")); summary != "" {
		state.Set("react.final_output_summary", summary)
	}
}

func mirrorReactCheckpointReference(state *core.Context) {
	if state == nil {
		return
	}
	if rawRef, ok := state.Get("graph.checkpoint_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			state.Set("react.checkpoint_ref", ref)
		}
	}
}

func compactReactFinalOutputState(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output_ref"); !ok {
		return
	}
	raw, ok := state.Get("react.final_output")
	if !ok {
		return
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return
	}
	summary := strings.TrimSpace(fmt.Sprint(payload["summary"]))
	if summary == "" || summary == "<nil>" {
		return
	}
	state.Set("react.final_output", map[string]any{
		"summary": summary,
	})
}

func compactReactToolObservationsState(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output_ref"); !ok {
		return
	}
	raw, ok := state.Get("react.tool_observations")
	if !ok {
		return
	}
	observations, ok := raw.([]ToolObservation)
	if !ok {
		return
	}
	state.Set("react.tool_observations", compactReactToolObservations(observations))
}

func compactReactToolObservations(observations []ToolObservation) map[string]any {
	value := map[string]any{
		"observation_count": len(observations),
	}
	if len(observations) == 0 {
		return value
	}
	last := observations[len(observations)-1]
	value["last_tool"] = last.Tool
	value["last_success"] = last.Success
	if len(observations) > 0 {
		recent := make([]string, 0, len(observations))
		for _, observation := range observations {
			tool := strings.TrimSpace(observation.Tool)
			if tool == "" {
				continue
			}
			recent = append(recent, tool)
			if len(recent) == 3 {
				break
			}
		}
		if len(recent) > 0 {
			value["recent_tools"] = recent
		}
	}
	return value
}

func compactReactLastToolResultState(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output_ref"); !ok {
		return
	}
	raw, ok := state.Get("react.last_tool_result")
	if !ok {
		return
	}
	payload, ok := raw.(map[string]interface{})
	if !ok {
		return
	}
	state.Set("react.last_tool_result", compactReactLastToolResult(payload))
}

func compactReactLastToolResult(payload map[string]interface{}) map[string]any {
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	value := map[string]any{
		"key_count": len(keys),
		"keys":      keys,
	}
	if errText := strings.TrimSpace(fmt.Sprint(payload["error"])); errText != "" && errText != "<nil>" {
		value["error"] = errText
	}
	if success, ok := payload["success"].(bool); ok {
		value["success"] = success
	}
	return value
}

func compactReactLoopState(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output_ref"); !ok {
		return
	}
	if raw, ok := state.Get("react.decision"); ok && raw != nil {
		state.Set("react.decision", map[string]any{"present": true})
	}
	if raw, ok := state.Get("react.tool_calls"); ok {
		switch calls := raw.(type) {
		case []core.ToolCall:
			state.Set("react.tool_calls", map[string]any{"count": len(calls)})
		case []any:
			state.Set("react.tool_calls", map[string]any{"count": len(calls)})
		}
	}
	if _, ok := state.Get("react.last_tool_result_envelope"); ok {
		state.Set("react.last_tool_result_envelope", map[string]any{"present": true})
	}
	if raw, ok := state.Get("react.last_tool_result_envelopes"); ok {
		switch envelopes := raw.(type) {
		case []*core.CapabilityResultEnvelope:
			state.Set("react.last_tool_result_envelopes", map[string]any{"count": len(envelopes)})
		case []any:
			state.Set("react.last_tool_result_envelopes", map[string]any{"count": len(envelopes)})
		}
	}
}

func (a *ReActAgent) completeExplicitReadOnlyRetrieval(ctx context.Context, task *core.Task, state *core.Context) error {
	if a == nil || a.Tools == nil || task == nil || state == nil || taskNeedsEditing(task) {
		return nil
	}
	requested := explicitlyRequestedToolNames(task)
	if len(requested) == 0 {
		return nil
	}
	if _, wantsSemantic := requested["search_semantic"]; !wantsSemantic {
		return nil
	}
	if _, wantsRead := requested["file_read"]; !wantsRead || requestedReadOnlyToolsSatisfied(task, state) {
		return nil
	}
	rawLast, _ := state.Get("react.last_tool_result")
	lastMap, _ := rawLast.(map[string]interface{})
	path := firstSearchResultPath(lastMap["results"])
	if path == "" {
		return nil
	}
	res, err := a.Tools.InvokeCapability(ctx, state, "file_read", map[string]any{"path": path})
	if err != nil {
		return err
	}
	call := core.ToolCall{
		ID:   NewUUID(),
		Name: "file_read",
		Args: map[string]any{"path": path},
	}
	observation := summarizeToolResult(state, call, res)
	history := append(getToolObservations(state), observation)
	state.Set("react.tool_observations", history)
	state.Set("react.last_tool_result", res.Data)
	return nil
}

func firstSearchResultPath(raw any) string {
	switch items := raw.(type) {
	case []any:
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if path := strings.TrimSpace(fmt.Sprint(entry["file"])); path != "" {
				return path
			}
		}
	case []map[string]any:
		for _, entry := range items {
			if path := strings.TrimSpace(fmt.Sprint(entry["file"])); path != "" {
				return path
			}
		}
	}
	return ""
}

// Capabilities describes what the agent can do.
func (a *ReActAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityCode,
		core.CapabilityExplain,
	}
}

// BuildGraph constructs the ReAct workflow.
func (a *ReActAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.Model == nil {
		return nil, fmt.Errorf("react agent missing language model")
	}
	think := &reactThinkNode{
		id:    "react_think",
		agent: a,
		task:  task,
	}
	act := &reactActNode{
		id:    "react_act",
		agent: a,
		task:  task,
	}
	observe := &reactObserveNode{
		id:    "react_observe",
		agent: a,
		task:  task,
	}
	done := graph.NewTerminalNode("react_done")
	summarize := graph.NewSummarizeContextNode("react_summarize", a.contextSummarizer())
	summarize.StateKeys = []string{"react.last_tool_result", "react.tool_observations", "react.final_output", "react.incomplete_reason"}
	summarize.Telemetry = telemetryForConfig(a.Config)
	var persist *graph.PersistenceWriterNode
	if reactUsesStructuredPersistence(a.Config) {
		if runtimeStore := runtimeMemoryStore(a.Memory); runtimeStore != nil {
			persist = graph.NewPersistenceWriterNode("react_persist", runtimeStore)
			persist.TaskID = taskID(task)
			persist.Telemetry = telemetryForConfig(a.Config)
			persist.Declarative = []graph.DeclarativePersistenceRequest{{
				StateKey:            "react.final_output",
				Scope:               string(memory.MemoryScopeProject),
				Kind:                graph.DeclarativeKindProjectKnowledge,
				Title:               taskInstructionText(task),
				SummaryField:        "summary",
				ContentField:        "result",
				ArtifactRefStateKey: "graph.summary_ref",
				Tags:                []string{"react", "task-summary"},
				Reason:              "react-completion-summary",
			}}
			persist.Artifacts = []graph.ArtifactPersistenceRequest{{
				ArtifactRefStateKey: "graph.summary_ref",
				SummaryStateKey:     "graph.summary",
				Reason:              "react-context-summary-artifact",
			}}
		}
	}
	var checkpoint *graph.CheckpointNode
	if reactUsesExplicitCheckpointNodes(a.Config) && a.CheckpointPath != "" && task != nil && task.ID != "" {
		checkpoint = graph.NewCheckpointNode("react_checkpoint", done.ID(), memory.NewCheckpointStore(filepath.Clean(a.CheckpointPath)))
		checkpoint.TaskID = task.ID
		checkpoint.Telemetry = telemetryForConfig(a.Config)
	}
	g := graph.NewGraph()
	if catalog := a.executionCapabilityCatalog(); catalog != nil && len(catalog.InspectableCapabilities()) > 0 {
		g.SetCapabilityCatalog(catalog)
	}
	if reactUsesDeclarativeRetrieval(a.Config) && a.Memory != nil {
		retrieve := graph.NewRetrieveDeclarativeMemoryNode("react_retrieve_declarative", scopedMemoryRetriever{
			store:       a.Memory,
			scope:       memory.MemoryScopeProject,
			memoryClass: core.MemoryClassDeclarative,
		})
		retrieve.Query = taskInstructionText(task)
		if err := g.AddNode(retrieve); err != nil {
			return nil, err
		}
		if err := g.SetStart(retrieve.ID()); err != nil {
			return nil, err
		}
		if err := g.AddNode(think); err != nil {
			return nil, err
		}
		if err := g.AddEdge(retrieve.ID(), think.ID(), nil, false); err != nil {
			return nil, err
		}
	} else {
		if err := g.AddNode(think); err != nil {
			return nil, err
		}
		if err := g.SetStart(think.ID()); err != nil {
			return nil, err
		}
	}
	for _, node := range []graph.Node{act, observe, summarize, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if persist != nil {
		if err := g.AddNode(persist); err != nil {
			return nil, err
		}
	}
	if checkpoint != nil {
		if err := g.AddNode(checkpoint); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge(think.ID(), act.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(act.ID(), observe.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(observe.ID(), think.ID(), func(result *core.Result, ctx *core.Context) bool {
		done, _ := ctx.Get("react.done")
		return done == false || done == nil
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(observe.ID(), summarize.ID(), func(result *core.Result, ctx *core.Context) bool {
		done, _ := ctx.Get("react.done")
		return done == true
	}, false); err != nil {
		return nil, err
	}
	nextAfterSummarize := done.ID()
	if persist != nil {
		nextAfterSummarize = persist.ID()
	} else if checkpoint != nil {
		nextAfterSummarize = checkpoint.ID()
	}
	if err := g.AddEdge(summarize.ID(), nextAfterSummarize, nil, false); err != nil {
		return nil, err
	}
	if persist != nil && checkpoint != nil {
		if err := g.AddEdge(persist.ID(), checkpoint.ID(), nil, false); err != nil {
			return nil, err
		}
		if err := g.AddEdge(checkpoint.ID(), done.ID(), nil, false); err != nil {
			return nil, err
		}
	} else if persist != nil {
		if err := g.AddEdge(persist.ID(), done.ID(), nil, false); err != nil {
			return nil, err
		}
	} else if checkpoint != nil {
		if err := g.AddEdge(checkpoint.ID(), done.ID(), nil, false); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (a *ReActAgent) contextSummarizer() core.Summarizer {
	if a != nil && a.contextPolicy != nil && a.contextPolicy.Summarizer != nil {
		return a.contextPolicy.Summarizer
	}
	return &core.SimpleSummarizer{}
}

func telemetryForConfig(cfg *core.Config) core.Telemetry {
	if cfg == nil {
		return nil
	}
	return cfg.Telemetry
}

func taskID(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ID)
}

func runtimeMemoryStore(store memory.MemoryStore) graph.RuntimePersistenceStore {
	if runtimeStore, ok := store.(memory.RuntimeMemoryStore); ok {
		return memory.AdaptRuntimeStoreForGraph(runtimeStore)
	}
	return nil
}

func reactUsesExplicitCheckpointNodes(cfg *core.Config) bool {
	if cfg == nil || cfg.UseExplicitCheckpointNodes == nil {
		return true
	}
	return *cfg.UseExplicitCheckpointNodes
}

func reactUsesDeclarativeRetrieval(cfg *core.Config) bool {
	if cfg == nil || cfg.UseDeclarativeRetrieval == nil {
		return true
	}
	return *cfg.UseDeclarativeRetrieval
}

func reactUsesStructuredPersistence(cfg *core.Config) bool {
	if cfg == nil || cfg.UseStructuredPersistence == nil {
		return true
	}
	return *cfg.UseStructuredPersistence
}

func (a *ReActAgent) enforceBudget(state *core.Context) {
	if a.contextPolicy == nil {
		return
	}
	var tools []core.Tool
	if catalog := a.executionCapabilityCatalog(); catalog != nil {
		tools = catalog.ModelCallableTools()
	} else if a.Tools != nil {
		tools = a.Tools.ModelCallableTools()
	}
	a.contextPolicy.EnforceBudget(state, a.sharedContext, a.Model, tools, a.debugf)
}

func (a *ReActAgent) recordLatestInteraction(state *core.Context) {
	if a.contextPolicy == nil {
		return
	}
	a.contextPolicy.RecordLatestInteraction(state, a.debugf)
}

func (a *ReActAgent) manageContextSignals(state *core.Context) {
	if a.contextPolicy == nil {
		return
	}
	lastResult := a.getLastResult(state)
	a.contextPolicy.HandleSignals(state, a.sharedContext, lastResult)
}

func (a *ReActAgent) initializePhase(state *core.Context, task *core.Task) {
	if state == nil {
		return
	}
	if phase := state.GetString("react.phase"); phase != "" {
		return
	}
	phase := contextmgrPhaseExplore
	text := taskInstructionText(task)
	if task != nil && task.Context != nil {
		if _, ok := task.Context["current_step"]; ok {
			if strings.Contains(text, "verify") || strings.Contains(text, "test") || strings.Contains(text, "build") {
				phase = contextmgrPhaseVerify
			}
		}
	}
	if !taskNeedsEditing(task) && taskRequiresVerification(task) && len(explicitlyRequestedToolNames(task)) > 0 {
		phase = contextmgrPhaseVerify
	}
	if strings.EqualFold(a.Mode, "debug") && (strings.Contains(text, "test") || strings.Contains(text, "build") || strings.Contains(text, "lint") || strings.Contains(text, "cargo")) {
		phase = contextmgrPhaseVerify
	}
	if strings.EqualFold(a.Mode, "docs") {
		phase = contextmgrPhaseEdit
	}
	state.Set("react.phase", phase)
}

func (a *ReActAgent) availableToolsForPhase(state *core.Context, task *core.Task) []core.Tool {
	catalog := a.executionCapabilityCatalog()
	if catalog == nil && a.Tools == nil {
		return nil
	}
	phase := contextmgrPhaseExplore
	if state != nil {
		if current := state.GetString("react.phase"); current != "" {
			phase = current
		}
	}
	var filtered []core.Tool
	tools := executionCallableTools(a.Tools, catalog)
	for _, tool := range tools {
		if toolAllowedForPhase(tool, phase, task) || a.recoveryToolAllowed(state, task, tool.Name()) {
			if !a.toolAllowedBySkillConfig(task, phase, tool.Name()) {
				continue
			}
			if !a.toolAllowedByExecutionContext(state, task, phase, tool) {
				continue
			}
			filtered = append(filtered, tool)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name() < filtered[j].Name() })
	return filtered
}

func (a *ReActAgent) executionCapabilityCatalog() *capability.ExecutionCapabilityCatalogSnapshot {
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

func (a *ReActAgent) executionPolicySnapshot() *core.PolicySnapshot {
	if catalog := a.executionCapabilityCatalog(); catalog != nil {
		return catalog.PolicySnapshot()
	}
	if a == nil || a.Tools == nil {
		return nil
	}
	return a.Tools.CapturePolicySnapshot()
}

func (a *ReActAgent) executionCapabilityDescriptor(idOrName string) (core.CapabilityDescriptor, bool) {
	if catalog := a.executionCapabilityCatalog(); catalog != nil {
		if entry, ok := catalog.GetCapability(idOrName); ok {
			return entry.Descriptor, true
		}
	}
	if a == nil || a.Tools == nil {
		return core.CapabilityDescriptor{}, false
	}
	return a.Tools.GetCapability(idOrName)
}

func executionCallableTools(registry *capability.Registry, catalog *capability.ExecutionCapabilityCatalogSnapshot) []core.Tool {
	if catalog != nil {
		return catalog.ModelCallableTools()
	}
	if registry == nil {
		return nil
	}
	return registry.ModelCallableTools()
}

func (a *ReActAgent) toolAllowedByExecutionContext(state *core.Context, task *core.Task, phase string, tool core.Tool) bool {
	if tool == nil {
		return false
	}
	if strings.EqualFold(a.Mode, "docs") {
		name := strings.ToLower(strings.TrimSpace(tool.Name()))
		if name == "file_write" || name == "file_create" || name == "file_delete" {
			return false
		}
	}
	if requested := explicitlyRequestedToolNames(task); len(requested) > 0 && !taskNeedsEditing(task) && phase != contextmgrPhaseEdit {
		if _, ok := requested[strings.ToLower(strings.TrimSpace(tool.Name()))]; !ok {
			return false
		}
	}
	if requested := explicitlyRequestedToolNames(task); len(requested) > 0 && verificationLikeTool(tool) {
		if _, ok := requested[strings.ToLower(strings.TrimSpace(tool.Name()))]; !ok {
			return false
		}
	}
	if phase != contextmgrPhaseEdit {
		return true
	}
	if hasEditObservation(state) {
		return true
	}
	if tool.Name() == "file_read" && repeatedReadTarget(state) != "" {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(tool.Name()))
	if strings.Contains(name, "rustfmt") || strings.Contains(name, "format") || strings.Contains(name, "fmt") {
		return false
	}
	if taskNeedsEditing(task) && hasFailureFromState(state) && verificationLikeTool(tool) {
		return false
	}
	return true
}

func (a *ReActAgent) recoveryToolAllowed(state *core.Context, task *core.Task, toolName string) bool {
	if state == nil || !hasFailureFromState(state) {
		return false
	}
	for _, probe := range a.recoveryProbeTools(task) {
		if strings.EqualFold(strings.TrimSpace(probe), toolName) {
			return true
		}
	}
	return false
}

func (a *ReActAgent) toolAllowedBySkillConfig(task *core.Task, phase, toolName string) bool {
	resolved := a.resolvedSkillPolicy(task)
	if len(resolved.PhaseCapabilities) == 0 {
		return true
	}
	allowed, ok := resolved.PhaseCapabilities[phase]
	if !ok || len(allowed) == 0 {
		return true
	}
	for _, entry := range allowed {
		if strings.EqualFold(strings.TrimSpace(entry), toolName) {
			return true
		}
	}
	return false
}

func (a *ReActAgent) resolvedSkillPolicy(task *core.Task) frameworkskills.ResolvedSkillPolicy {
	return frameworkskills.ResolveEffectiveSkillPolicy(task, a.effectiveAgentSpec(task), a.Tools).Policy
}

func (a *ReActAgent) recoveryProbeTools(task *core.Task) []string {
	resolved := a.resolvedSkillPolicy(task)
	return append([]string{}, resolved.RecoveryProbeCapabilities...)
}

func (a *ReActAgent) verificationSuccessTools(task *core.Task) []string {
	resolved := a.resolvedSkillPolicy(task)
	return append([]string{}, resolved.VerificationSuccessCapabilities...)
}

func (a *ReActAgent) effectiveAgentSpec(task *core.Task) *core.AgentRuntimeSpec {
	if a == nil || a.Config == nil {
		return frameworkskills.EffectiveAgentSpec(task, nil)
	}
	return frameworkskills.EffectiveAgentSpec(task, a.Config.AgentSpec)
}

func toolAllowedForPhase(tool core.Tool, phase string, task *core.Task) bool {
	if tool == nil {
		return false
	}
	name := tool.Name()
	tags := tool.Tags()
	if len(tags) == 0 {
		return true
	}
	hasTag := func(target string) bool {
		for _, tag := range tags {
			if tag == target {
				return true
			}
		}
		return false
	}
	switch phase {
	case contextmgrPhaseEdit:
		if hasTag(core.TagDestructive) {
			return true
		}
		if hasTag(core.TagExecute) {
			return isLanguageExecutionTool(name, task)
		}
		if hasTag(core.TagReadOnly) {
			return strings.HasPrefix(name, "file_") || strings.HasPrefix(name, "ast_") || strings.HasPrefix(name, "lsp_") || strings.Contains(name, "grep")
		}
		return name == "exec_run_code"
	case contextmgrPhaseVerify:
		if hasTag(core.TagExecute) {
			return true
		}
		return strings.Contains(name, "rustfmt") || strings.Contains(name, "format") || strings.HasPrefix(name, "file_read")
	default:
		if hasTag(core.TagReadOnly) {
			return true
		}
		if hasTag(core.TagExecute) {
			return strings.EqualFold(taskMode(task), "debug") && isLanguageExecutionTool(name, task)
		}
		return strings.HasPrefix(name, "ast_") || strings.HasPrefix(name, "lsp_") || strings.Contains(name, "grep")
	}
}

func isLanguageExecutionTool(name string, task *core.Task) bool {
	name = strings.ToLower(name)
	if _, ok := explicitlyRequestedToolNames(task)[name]; ok {
		return true
	}
	if strings.Contains(name, "cargo") || strings.Contains(name, "rustfmt") {
		return true
	}
	if strings.Contains(name, "sqlite") {
		return true
	}
	if strings.Contains(name, "test") || strings.Contains(name, "build") || strings.Contains(name, "lint") || strings.Contains(name, "format") || strings.Contains(name, "check") {
		return true
	}
	if strings.Contains(name, "exec_run_code") {
		return true
	}
	text := ""
	if task != nil {
		text = strings.ToLower(task.Instruction)
	}
	return strings.Contains(text, "test") || strings.Contains(text, "build") || strings.Contains(text, "lint")
}

func taskMode(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(task.Context["mode"]))
}

func activeToolSet(state *core.Context) map[string]struct{} {
	out := map[string]struct{}{}
	if state == nil {
		return out
	}
	raw, ok := state.Get("react.active_tools")
	if !ok {
		return out
	}
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			out[value] = struct{}{}
		}
	case []any:
		for _, value := range values {
			out[fmt.Sprint(value)] = struct{}{}
		}
	}
	return out
}

func recordActiveToolNames(state *core.Context, tools []core.Tool) {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	state.Set("react.active_tools", names)
}

func (a *ReActAgent) getLastResult(state *core.Context) *core.Result {
	if state == nil {
		return nil
	}
	if val, ok := state.GetHandle("react.last_result"); ok {
		if res, ok := val.(*core.Result); ok {
			return res
		}
	}
	val, ok := state.Get("react.last_result")
	if ok {
		if res, ok := val.(*core.Result); ok {
			return res
		}
	}
	return nil
}
