package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents"
	runtimesvc "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/persistence"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/workspacecfg"
)

const contextFileMaxBytes = 8000

// ToolInfo describes a registered tool and its current policy for the Tools pane.
type ToolInfo struct {
	Name      string
	Tags      []string
	Policy    fruntime.AgentPermissionLevel // per-tool override; "" means no override
	HasPolicy bool
}

// RuntimeAdapter decouples the TUI from the concrete runtime implementation.
type RuntimeAdapter interface {
	hitlService
	ExecuteInstruction(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any) (*core.Result, error)
	ExecuteInstructionStream(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any, callback func(string)) (*core.Result, error)
	AvailableAgents() []string
	SwitchAgent(name string) error
	SessionInfo() SessionInfo
	ResolveContextFiles(ctx context.Context, files []string) ContextFileResolution
	SessionArtifacts() SessionArtifacts
	OllamaModels(ctx context.Context) ([]string, error)
	RecordingMode() string
	SetRecordingMode(mode string) error
	SaveModel(model string) error
	// SaveToolPolicy persists a per-tool execution policy to the agent manifest.
	// toolName is the bare tool name (e.g. "cli_mkdir"); level is typically AgentPermissionAllow.
	SaveToolPolicy(toolName string, level fruntime.AgentPermissionLevel) error
	// ListToolsInfo returns the current tool list with per-tool policy overrides.
	ListToolsInfo() []ToolInfo
	// GetTagPolicies returns the current tag-based permission policies.
	GetTagPolicies() map[string]fruntime.AgentPermissionLevel
	// SetToolPolicyLive updates a per-tool execution policy in-memory (current session only).
	// Pass level="" to clear the override.
	SetToolPolicyLive(name string, level fruntime.AgentPermissionLevel)
	// SetTagPolicyLive updates a tag permission policy in-memory (current session only).
	// Pass level="" to clear the tag policy.
	SetTagPolicyLive(tag string, level fruntime.AgentPermissionLevel)
	ListWorkflows(limit int) ([]WorkflowInfo, error)
	GetWorkflow(workflowID string) (*WorkflowDetails, error)
	CancelWorkflow(workflowID string) error
}

type runtimeAdapter struct {
	rt *runtimesvc.Runtime
}

func newRuntimeAdapter(rt *runtimesvc.Runtime) RuntimeAdapter {
	if rt == nil {
		return nil
	}
	return &runtimeAdapter{rt: rt}
}

func (r *runtimeAdapter) ExecuteInstruction(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any) (*core.Result, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	return r.rt.ExecuteInstruction(ctx, instruction, taskType, metadata)
}

func (r *runtimeAdapter) AvailableAgents() []string {
	if r == nil || r.rt == nil {
		return nil
	}
	return r.rt.AvailableAgents()
}

func (r *runtimeAdapter) SwitchAgent(name string) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	return r.rt.SwitchAgent(name)
}

func (r *runtimeAdapter) SessionInfo() SessionInfo {
	info := SessionInfo{
		Workspace: "",
		Model:     "",
		Agent:     "",
		Role:      "",
		Mode:      "",
		Strategy:  "",
		MaxTokens: 100000,
	}
	if r == nil || r.rt == nil {
		return info
	}
	cfg := r.rt.Config
	info.Workspace = cfg.Workspace
	info.Model = cfg.OllamaModel
	info.Agent = cfg.AgentLabel()

	if r.rt.Registration != nil && r.rt.Registration.Manifest != nil {
		manifest := r.rt.Registration.Manifest
		info.Agent = manifest.Metadata.Name
		if manifest.Spec.Agent != nil {
			if manifest.Spec.Agent.Model.Name != "" {
				info.Model = manifest.Spec.Agent.Model.Name
			}
			if manifest.Spec.Agent.Mode != "" {
				info.Role = string(manifest.Spec.Agent.Mode)
			}
			if manifest.Spec.Agent.Context.MaxTokens > 0 {
				info.MaxTokens = manifest.Spec.Agent.Context.MaxTokens
			}
		}
	}
	info.Mode, info.Strategy = describeAgentRuntime(r.rt.Agent)
	return info
}

func describeAgentRuntime(agent graph.Agent) (string, string) {
	switch typed := agent.(type) {
	case *agents.CodingAgent:
		return describeCodingMode(agents.ModeCode)
	case *agents.ArchitectAgent:
		return string(agents.ModeArchitect), "plan-execute"
	case *agents.ReActAgent:
		mode := agents.Mode(strings.TrimSpace(typed.Mode))
		return describeCodingMode(mode)
	case *agents.ReflectionAgent:
		mode, _ := describeAgentRuntime(typed.Delegate)
		if mode == "" {
			mode = string(agents.ModeCode)
		}
		return mode, "reflection"
	case *agents.PlannerAgent:
		return "plan", "plan-execute-verify"
	case *agents.EternalAgent:
		return "loop", "eternal"
	default:
		return "", ""
	}
}

func describeCodingMode(mode agents.Mode) (string, string) {
	if profile, ok := agents.ModeProfiles[mode]; ok {
		return string(profile.Name), profile.PreferredStrategy
	}
	return string(agents.ModeCode), agents.ModeProfiles[agents.ModeCode].PreferredStrategy
}

func (r *runtimeAdapter) ResolveContextFiles(ctx context.Context, files []string) ContextFileResolution {
	paths := normalizePaths(files)
	res := ContextFileResolution{
		Allowed:  make([]string, 0, len(paths)),
		Contents: make([]core.ContextFileContent, 0, len(paths)),
		Denied:   make(map[string]string),
	}
	if r == nil || r.rt == nil {
		res.Allowed = paths
		return res
	}
	workspace := r.rt.Config.Workspace
	perm := r.rt.Registration.Permissions

	for _, path := range paths {
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(workspace, abs)
		}
		abs = filepath.Clean(abs)

		if perm != nil {
			if err := perm.CheckFileAccess(ctx, r.rt.Registration.ID, core.FileSystemRead, abs); err != nil {
				res.Denied[path] = err.Error()
				continue
			}
		}
		info, err := os.Stat(abs)
		if err != nil {
			res.Denied[path] = err.Error()
			continue
		}
		if info.IsDir() {
			res.Denied[path] = "path is a directory"
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			res.Denied[path] = err.Error()
			continue
		}
		content := string(data)
		truncated := false
		if len(content) > contextFileMaxBytes {
			content = content[:contextFileMaxBytes]
			truncated = true
		}
		res.Allowed = append(res.Allowed, abs)
		res.Contents = append(res.Contents, core.ContextFileContent{
			Path:      path,
			Content:   content,
			Truncated: truncated,
		})
	}
	return res
}

func (r *runtimeAdapter) ExecuteInstructionStream(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any, callback func(string)) (*core.Result, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	return r.rt.ExecuteInstructionStream(ctx, instruction, taskType, metadata, callback)
}

func (r *runtimeAdapter) OllamaModels(ctx context.Context) ([]string, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	endpoint := r.rt.Config.OllamaEndpoint
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	u := strings.TrimRight(endpoint, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

func (r *runtimeAdapter) RecordingMode() string {
	if r == nil || r.rt == nil {
		return "off"
	}
	if r.rt.Config.RecordingMode != "" {
		return r.rt.Config.RecordingMode
	}
	return "off"
}

func (r *runtimeAdapter) SetRecordingMode(mode string) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	r.rt.Config.RecordingMode = mode
	return nil
}

func (r *runtimeAdapter) SaveModel(model string) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	cfgPath := r.rt.Config.ConfigPath
	if cfgPath == "" {
		return fmt.Errorf("config path not set")
	}
	wsCfg, err := runtimesvc.LoadWorkspaceConfig(cfgPath)
	if err != nil {
		wsCfg = runtimesvc.WorkspaceConfig{}
	}
	wsCfg.Model = model
	wsCfg.LastUpdated = time.Now().Unix()
	return runtimesvc.SaveWorkspaceConfig(cfgPath, wsCfg)
}

func (r *runtimeAdapter) ListWorkflows(limit int) ([]WorkflowInfo, error) {
	store, err := r.openWorkflowStore()
	if err != nil {
		return nil, err
	}
	defer store.Close()
	workflows, err := store.ListWorkflows(context.Background(), limit)
	if err != nil {
		return nil, err
	}
	out := make([]WorkflowInfo, 0, len(workflows))
	for _, workflow := range workflows {
		out = append(out, WorkflowInfo{
			WorkflowID:   workflow.WorkflowID,
			TaskID:       workflow.TaskID,
			Status:       string(workflow.Status),
			CursorStepID: workflow.CursorStepID,
			Instruction:  workflow.Instruction,
			UpdatedAt:    workflow.UpdatedAt,
		})
	}
	return out, nil
}

func (r *runtimeAdapter) GetWorkflow(workflowID string) (*WorkflowDetails, error) {
	store, err := r.openWorkflowStore()
	if err != nil {
		return nil, err
	}
	defer store.Close()
	workflow, ok, err := store.GetWorkflow(context.Background(), workflowID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("workflow %s not found", workflowID)
	}
	steps, err := store.ListSteps(context.Background(), workflowID)
	if err != nil {
		return nil, err
	}
	events, err := store.ListEvents(context.Background(), workflowID, 20)
	if err != nil {
		return nil, err
	}
	facts, err := store.ListKnowledge(context.Background(), workflowID, persistence.KnowledgeKindFact, false)
	if err != nil {
		return nil, err
	}
	issues, err := store.ListKnowledge(context.Background(), workflowID, persistence.KnowledgeKindIssue, false)
	if err != nil {
		return nil, err
	}
	decisions, err := store.ListKnowledge(context.Background(), workflowID, persistence.KnowledgeKindDecision, false)
	if err != nil {
		return nil, err
	}
	info := &WorkflowDetails{
		Workflow: WorkflowInfo{
			WorkflowID:   workflow.WorkflowID,
			TaskID:       workflow.TaskID,
			Status:       string(workflow.Status),
			CursorStepID: workflow.CursorStepID,
			Instruction:  workflow.Instruction,
			UpdatedAt:    workflow.UpdatedAt,
		},
		Steps:     make([]WorkflowStepInfo, 0, len(steps)),
		Events:    make([]WorkflowEventInfo, 0, len(events)),
		Facts:     make([]WorkflowKnowledgeInfo, 0, len(facts)),
		Issues:    make([]WorkflowKnowledgeInfo, 0, len(issues)),
		Decisions: make([]WorkflowKnowledgeInfo, 0, len(decisions)),
	}
	for _, step := range steps {
		info.Steps = append(info.Steps, WorkflowStepInfo{
			StepID:       step.StepID,
			Description:  step.Step.Description,
			Status:       string(step.Status),
			Summary:      step.Summary,
			Dependencies: append([]string{}, step.Dependencies...),
		})
	}
	for _, event := range events {
		info.Events = append(info.Events, WorkflowEventInfo{
			EventType: event.EventType,
			StepID:    event.StepID,
			Message:   event.Message,
			CreatedAt: event.CreatedAt,
		})
	}
	info.Facts = append(info.Facts, convertKnowledgeInfos(facts)...)
	info.Issues = append(info.Issues, convertKnowledgeInfos(issues)...)
	info.Decisions = append(info.Decisions, convertKnowledgeInfos(decisions)...)
	return info, nil
}

func (r *runtimeAdapter) CancelWorkflow(workflowID string) error {
	store, err := r.openWorkflowStore()
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.UpdateWorkflowStatus(context.Background(), workflowID, 0, persistence.WorkflowRunStatusCanceled, "")
	return err
}

func (r *runtimeAdapter) openWorkflowStore() (*persistence.SQLiteWorkflowStateStore, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	path := workspacecfg.New(r.rt.Config.Workspace).WorkflowStateFile()
	return persistence.NewSQLiteWorkflowStateStore(path)
}

func convertKnowledgeInfos(records []persistence.KnowledgeRecord) []WorkflowKnowledgeInfo {
	out := make([]WorkflowKnowledgeInfo, 0, len(records))
	for _, record := range records {
		out = append(out, WorkflowKnowledgeInfo{
			StepID:    record.StepID,
			Kind:      string(record.Kind),
			Title:     record.Title,
			Content:   record.Content,
			Status:    record.Status,
			CreatedAt: record.CreatedAt,
		})
	}
	return out
}

func (r *runtimeAdapter) SaveToolPolicy(toolName string, level fruntime.AgentPermissionLevel) error {
	if r == nil || r.rt == nil || r.rt.Registration == nil || r.rt.Registration.Manifest == nil {
		return fmt.Errorf("runtime unavailable")
	}
	sourcePath := r.rt.Registration.Manifest.SourcePath
	if sourcePath == "" {
		return fmt.Errorf("manifest source path not set")
	}
	// Reload from disk to avoid saving already-resolved permissions.
	m, err := fruntime.LoadAgentManifest(sourcePath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	if m.Spec.Agent == nil {
		return fmt.Errorf("manifest has no agent spec")
	}
	if m.Spec.Agent.ToolExecutionPolicy == nil {
		m.Spec.Agent.ToolExecutionPolicy = make(map[string]core.ToolPolicy)
	}
	m.Spec.Agent.ToolExecutionPolicy[toolName] = core.ToolPolicy{Execute: core.AgentPermissionLevel(level)}
	// Also update in-memory registration so current session benefits immediately.
	if r.rt.Registration.Manifest.Spec.Agent != nil {
		if r.rt.Registration.Manifest.Spec.Agent.ToolExecutionPolicy == nil {
			r.rt.Registration.Manifest.Spec.Agent.ToolExecutionPolicy = make(map[string]core.ToolPolicy)
		}
		r.rt.Registration.Manifest.Spec.Agent.ToolExecutionPolicy[toolName] = core.ToolPolicy{Execute: core.AgentPermissionLevel(level)}
	}
	return fruntime.SaveAgentManifest(sourcePath, m)
}

func (r *runtimeAdapter) ListToolsInfo() []ToolInfo {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil
	}
	tools := r.rt.Tools.All()
	policies := r.rt.Tools.GetToolPolicies()
	infos := make([]ToolInfo, 0, len(tools))
	for _, t := range tools {
		name := t.Name()
		tags := t.Tags()
		pol := policies[name]
		level := fruntime.AgentPermissionLevel(pol.Execute)
		infos = append(infos, ToolInfo{
			Name:      name,
			Tags:      tags,
			Policy:    level,
			HasPolicy: level != "",
		})
	}
	return infos
}

func (r *runtimeAdapter) GetTagPolicies() map[string]fruntime.AgentPermissionLevel {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil
	}
	return r.rt.Tools.GetTagPolicies()
}

func (r *runtimeAdapter) SetToolPolicyLive(name string, level fruntime.AgentPermissionLevel) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return
	}
	r.rt.Tools.UpdateToolPolicy(name, core.ToolPolicy{Execute: core.AgentPermissionLevel(level)})
}

func (r *runtimeAdapter) SetTagPolicyLive(tag string, level fruntime.AgentPermissionLevel) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return
	}
	r.rt.Tools.UpdateTagPolicy(tag, level)
}

func (r *runtimeAdapter) SessionArtifacts() SessionArtifacts {
	if r == nil || r.rt == nil {
		return SessionArtifacts{}
	}
	return SessionArtifacts{
		TelemetryPath: r.rt.Config.TelemetryPath,
		LogPath:       r.rt.Config.LogPath,
	}
}

func (r *runtimeAdapter) PendingHITL() []*fruntime.PermissionRequest {
	if r == nil || r.rt == nil {
		return nil
	}
	return r.rt.PendingHITL()
}

func (r *runtimeAdapter) ApproveHITL(requestID, approver string, scope fruntime.GrantScope, duration time.Duration) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	return r.rt.ApproveHITL(requestID, approver, scope, duration)
}

func (r *runtimeAdapter) DenyHITL(requestID, reason string) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	return r.rt.DenyHITL(requestID, reason)
}

func (r *runtimeAdapter) SubscribeHITL() (<-chan fruntime.HITLEvent, func()) {
	if r == nil || r.rt == nil {
		return nil, func() {}
	}
	return r.rt.SubscribeHITL()
}
