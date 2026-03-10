package agents

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/agents/stages"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

// CodingAgent orchestrates multiple specialized modes inspired by the
// requirements document. It wraps existing planning/react agents with tailored
// tool scopes and temperatures while keeping a consistent interface for the
// runtime.
type CodingAgent struct {
	Model                core.LanguageModel
	Tools                *capability.Registry
	Memory               memory.MemoryStore
	Config               *core.Config
	IndexManager         *ast.IndexManager
	CheckpointPath       string
	WorkflowStatePath    string
	PipelineStages       []pipeline.Stage
	PipelineStageBuilder func(task *core.Task) ([]pipeline.Stage, error)
	PipelineStageFactory PipelineStageFactory
	modeProfiles         map[Mode]ModeProfile

	mu        sync.Mutex
	delegates map[Mode]graph.Agent
}

// Initialize wires configuration and default mode data.
func (a *CodingAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}

	if a.modeProfiles == nil {
		a.modeProfiles = defaultModeProfiles()
	}
	if a.delegates == nil {
		a.delegates = make(map[Mode]graph.Agent)
	}
	return nil
}

// ModeProfiles returns a copy of the current mode profile map.
func (a *CodingAgent) ModeProfiles() map[Mode]ModeProfile {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.modeProfiles == nil {
		a.modeProfiles = defaultModeProfiles()
	}
	out := make(map[Mode]ModeProfile, len(a.modeProfiles))
	for mode, profile := range a.modeProfiles {
		out[mode] = profile
	}
	return out
}

// Capabilities aggregates capabilities from all modes.
func (a *CodingAgent) Capabilities() []core.Capability {
	seen := map[core.Capability]struct{}{}
	var caps []core.Capability
	for _, profile := range a.modeProfiles {
		for _, cap := range profile.Capabilities {
			if _, ok := seen[cap]; ok {
				continue
			}
			seen[cap] = struct{}{}
			caps = append(caps, cap)
		}
	}
	return caps
}

// BuildGraph delegates to the graph for the task's effective mode.
func (a *CodingAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	mode := a.modeFromTask(task)
	profile, ok := a.modeProfiles[mode]
	if !ok {
		profile = a.modeProfiles[defaultMode]
	}
	delegate, err := a.delegateForMode(profile.Name)
	if err != nil {
		return nil, err
	}
	return delegate.BuildGraph(task)
}

// Execute selects the correct mode and proxies execution to the underlying
// pattern agent. The context is augmented with the mode metadata so downstream
// tooling can render diagnostics.
func (a *CodingAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if state == nil {
		state = core.NewContext()
	}
	mode := a.modeFromTask(task)
	profile, ok := a.modeProfiles[mode]
	if !ok {
		profile = a.modeProfiles[defaultMode]
	}
	delegate, err := a.delegateForMode(profile.Name)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	a.emitRunEvent(core.EventAgentStart, task, profile.Name, "coding agent run started", nil)
	enriched := *task
	enriched.Context = cloneContext(task.Context)
	enriched.Context["user_instruction"] = task.Instruction
	enriched.Context["mode"] = string(profile.Name)
	enriched.Context["restrictions"] = profile.Restrictions
	if a.Config != nil && a.Config.AgentSpec != nil {
		enriched.Context["agent_spec"] = a.Config.AgentSpec
	}
	enriched.Instruction = a.decorateInstruction(profile, task.Instruction)
	state.Set("coding_agent.mode", profile.Name)
	result, err := delegate.Execute(ctx, &enriched, state)
	if err != nil {
		a.emitRunEvent(core.EventAgentFinish, task, profile.Name, "coding agent run failed", map[string]any{
			"status": "failed",
			"error":  err.Error(),
		})
		return nil, err
	}
	if final, ok := state.Get("react.final_output"); ok {
		if result.Data == nil {
			result.Data = map[string]any{}
		}
		result.Data["final_output"] = final
	}
	if final, ok := state.Get("pipeline.final_output"); ok {
		if result.Data == nil {
			result.Data = map[string]any{}
		}
		result.Data["final_output"] = final
	}
	a.emitRunEvent(core.EventAgentFinish, task, profile.Name, "coding agent run completed", map[string]any{
		"status":  "completed",
		"success": result.Success,
	})
	return result, nil
}

func (a *CodingAgent) emitRunEvent(eventType core.EventType, task *core.Task, mode Mode, message string, metadata map[string]any) {
	if a == nil || a.Config == nil || a.Config.Telemetry == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["mode"] = string(mode)
	if task != nil {
		metadata["task_type"] = task.Type
	}
	a.Config.Telemetry.Emit(core.Event{
		Type:      eventType,
		TaskID:    taskID(task),
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
}

func taskID(task *core.Task) string {
	if task == nil {
		return ""
	}
	return task.ID
}

// modeFromTask inspects task metadata/context to decide which mode should own
// execution. It defaults to the general coding mode when nothing is specified.
func (a *CodingAgent) modeFromTask(task *core.Task) Mode {
	if task == nil {
		return defaultMode
	}
	if task.Metadata != nil {
		if mode, ok := task.Metadata["mode"]; ok {
			return Mode(strings.ToLower(mode))
		}
	}
	if task.Context != nil {
		if modeRaw, ok := task.Context["mode"]; ok {
			if mode, ok := modeRaw.(string); ok {
				return Mode(strings.ToLower(mode))
			}
		}
	}
	return defaultMode
}

// delegateForMode lazily instantiates the underlying agent for the requested
// mode and reuses it on subsequent calls.
func (a *CodingAgent) delegateForMode(mode Mode) (graph.Agent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if agent, ok := a.delegates[mode]; ok {
		return agent, nil
	}
	profile, ok := a.modeProfiles[mode]
	if !ok {
		return nil, fmt.Errorf("mode %s not configured", mode)
	}
	var agent graph.Agent
	switch profile.ControlFlow {
	case ControlFlowArchitect:
		agent = &ArchitectAgent{
			Model:             a.Model,
			PlannerTools:      a.scopedTools(profile.ToolScope),
			ExecutorTools:     a.scopedTools(ModeProfiles[ModeCode].ToolScope),
			Memory:            a.Memory,
			IndexManager:      a.IndexManager,
			CheckpointPath:    a.CheckpointPath,
			WorkflowStatePath: a.WorkflowStatePath,
		}
	case ControlFlowPipeline:
		agent = &PipelineAgent{
			Model:             a.Model,
			Tools:             a.scopedTools(profile.ToolScope),
			WorkflowStatePath: a.WorkflowStatePath,
			Stages:            append([]pipeline.Stage{}, a.PipelineStages...),
			StageBuilder:      a.PipelineStageBuilder,
			StageFactory:      a.pipelineStageFactoryForMode(profile.Name),
		}
	default:
		agent = &ReActAgent{
			Model:          a.Model,
			Tools:          a.scopedTools(profile.ToolScope),
			Memory:         a.Memory,
			IndexManager:   a.IndexManager,
			CheckpointPath: a.CheckpointPath,
			Mode:           string(profile.Name),
			ModeProfile:    convertModeRuntimeProfile(profile),
		}
	}
	if err := agent.Initialize(a.Config); err != nil {
		return nil, err
	}
	a.delegates[mode] = agent
	return agent, nil
}

// scopedTools clones the global registry but drops tools outside the mode's
// permission envelope.
func (a *CodingAgent) scopedTools(scope ToolScope) *capability.Registry {
	if a.Tools == nil {
		return capability.NewRegistry()
	}
	return a.Tools.CloneFiltered(func(tool core.Tool) bool {
		return toolAllowed(tool, scope)
	})
}

// toolAllowed checks whether the tool's declared permissions fit inside the
// mode's scope before the agent exposes it to the LLM.
func toolAllowed(tool core.Tool, scope ToolScope) bool {
	perms := tool.Permissions()
	if perms.Permissions == nil {
		return true
	}
	for _, fs := range perms.Permissions.FileSystem {
		switch fs.Action {
		case core.FileSystemWrite:
			if !scope.AllowWrite {
				return false
			}
		case core.FileSystemExecute:
			if !scope.AllowExecute {
				return false
			}
		}
	}
	if len(perms.Permissions.Executables) > 0 && !scope.AllowExecute {
		return false
	}
	if len(perms.Permissions.Network) > 0 && !scope.AllowNetwork {
		return false
	}
	return true
}

func (a *CodingAgent) pipelineStageFactoryForMode(mode Mode) PipelineStageFactory {
	if a.PipelineStageBuilder != nil || len(a.PipelineStages) > 0 {
		return nil
	}
	if a.PipelineStageFactory != nil {
		return a.PipelineStageFactory
	}
	switch mode {
	case ModeCode:
		return stages.CodingStageFactory{}
	default:
		return nil
	}
}

// OverrideControlFlow updates one mode profile at runtime and clears any cached
// delegate for that mode so the next execution uses the new runtime.
func (a *CodingAgent) OverrideControlFlow(mode Mode, flow ControlFlow) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.modeProfiles == nil {
		a.modeProfiles = defaultModeProfiles()
	}
	profile, ok := a.modeProfiles[mode]
	if !ok {
		return fmt.Errorf("mode %s not configured", mode)
	}
	profile.ControlFlow = flow
	a.modeProfiles[mode] = profile
	if a.delegates != nil {
		delete(a.delegates, mode)
	}
	return nil
}

// decorateInstruction wraps the user instruction with mode metadata so the LLM
// is primed with the current restrictions.
func (a *CodingAgent) decorateInstruction(profile ModeProfile, instruction string) string {
	if instruction == "" {
		return ""
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[Mode: %s]\n", profile.Title)
	fmt.Fprintf(builder, "Description: %s\n", profile.Description)
	if len(profile.Restrictions) > 0 {
		fmt.Fprintf(builder, "Restrictions: %s\n", strings.Join(profile.Restrictions, "; "))
	}
	fmt.Fprintf(builder, "\n%s", instruction)
	return builder.String()
}

func convertModeRuntimeProfile(profile ModeProfile) ModeRuntimeProfile {
	contextPrefs := ContextPreferences{
		PreferredDetailLevel: profile.ContextProfile.PreferredDetailLevel,
		MinHistorySize:       profile.ContextProfile.MinHistorySize,
		CompressionThreshold: profile.ContextProfile.CompressionThreshold,
	}
	return ModeRuntimeProfile{
		Name:        string(profile.Name),
		Description: profile.Description,
		Temperature: profile.Temperature,
		Context:     contextPrefs,
	}
}

// cloneContext performs a shallow copy of the task context map to avoid
// mutating the caller's state.
func cloneContext(ctx map[string]any) map[string]any {
	if ctx == nil {
		return map[string]any{}
	}
	clone := make(map[string]any, len(ctx))
	for k, v := range ctx {
		clone[k] = v
	}
	return clone
}
