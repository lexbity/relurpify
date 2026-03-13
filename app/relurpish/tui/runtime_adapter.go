package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents"
	runtimesvc "github.com/lexcodex/relurpify/app/relurpish/runtime"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
)

const contextFileMaxBytes = 8000

// ToolInfo describes a registered local tool and its current policy for the Tools pane.
type ToolInfo struct {
	Name          string
	RuntimeFamily string
	Scope         string
	Tags          []string
	Labels        []string
	RiskClasses   []string
	EffectClasses []string
	TrustClass    string
	Exposure      string
	Policy        core.AgentPermissionLevel // per-tool override; "" means no override
	HasPolicy     bool
}

// CapabilityInfo exposes non-tool capability metadata to inspectable UI surfaces.
type CapabilityInfo struct {
	ID            string
	Kind          string
	Name          string
	Description   string
	Category      string
	RuntimeFamily string
	TrustClass    string
	ProviderID    string
	Scope         string
	Exposure      string
	Callable      bool
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
	ContractSummary() *ContractSummary
	CapabilityAdmissions() []CapabilityAdmissionInfo
	// SaveToolPolicy persists a per-tool execution policy to the agent manifest.
	// toolName is the bare tool name (e.g. "cli_mkdir"); level is typically AgentPermissionAllow.
	SaveToolPolicy(toolName string, level core.AgentPermissionLevel) error
	// ListToolsInfo returns the current local-tool list with per-tool policy overrides.
	ListToolsInfo() []ToolInfo
	// ListCapabilities returns all registered capabilities with runtime-family metadata.
	ListCapabilities() []CapabilityInfo
	ListPrompts() []PromptInfo
	ListResources(workflowRefs []string) []ResourceInfo
	// ListLiveProviders returns current runtime provider snapshots.
	ListLiveProviders() []LiveProviderInfo
	// ListLiveSessions returns current runtime provider-session snapshots.
	ListLiveSessions() []LiveProviderSessionInfo
	// ListApprovals returns current pending HITL approvals using the unified approval model.
	ListApprovals() []ApprovalInfo
	GetCapabilityDetail(id string) (*CapabilityDetail, error)
	GetPromptDetail(id string) (*PromptDetail, error)
	GetResourceDetail(idOrURI string) (*ResourceDetail, error)
	GetLiveProviderDetail(providerID string) (*LiveProviderDetail, error)
	GetLiveSessionDetail(sessionID string) (*LiveProviderSessionDetail, error)
	GetApprovalDetail(id string) (*ApprovalDetail, error)
	// GetClassPolicies returns the current capability-class permission policies.
	GetClassPolicies() map[string]core.AgentPermissionLevel
	// SetToolPolicyLive updates a per-tool execution policy in-memory (current session only).
	// Pass level="" to clear the override.
	SetToolPolicyLive(name string, level core.AgentPermissionLevel)
	// SetClassPolicyLive updates a class permission policy in-memory (current session only).
	// Pass level="" to clear the class policy.
	SetClassPolicyLive(class string, level core.AgentPermissionLevel)
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

func (r *runtimeAdapter) ContractSummary() *ContractSummary {
	if r == nil || r.rt == nil || r.rt.EffectiveContract == nil {
		return nil
	}
	summary := &ContractSummary{
		AgentID:         r.rt.EffectiveContract.AgentID,
		ManifestName:    r.rt.EffectiveContract.Sources.ManifestName,
		ManifestVersion: r.rt.EffectiveContract.Sources.ManifestVersion,
		Workspace:       r.rt.EffectiveContract.Sources.Workspace,
		AppliedSkills:   append([]string(nil), r.rt.EffectiveContract.Sources.AppliedSkills...),
		FailedSkills:    append([]string(nil), r.rt.EffectiveContract.Sources.FailedSkills...),
		AdmissionCount:  len(r.rt.CapabilityAdmissions),
	}
	if r.rt.Tools != nil {
		summary.CapabilityCount = len(r.rt.Tools.AllCapabilities())
	}
	for _, admission := range r.rt.CapabilityAdmissions {
		if !admission.Admitted {
			summary.RejectedCount++
		}
	}
	if r.rt.CompiledPolicy != nil {
		summary.PolicyRuleCount = len(r.rt.CompiledPolicy.Rules)
	}
	return summary
}

func (r *runtimeAdapter) CapabilityAdmissions() []CapabilityAdmissionInfo {
	if r == nil || r.rt == nil {
		return nil
	}
	out := make([]CapabilityAdmissionInfo, 0, len(r.rt.CapabilityAdmissions))
	for _, admission := range r.rt.CapabilityAdmissions {
		out = append(out, CapabilityAdmissionInfo{
			CapabilityID:   admission.CapabilityID,
			CapabilityName: admission.CapabilityName,
			Kind:           string(admission.Kind),
			Admitted:       admission.Admitted,
			Reason:         admission.Reason,
		})
	}
	return out
}

func describeAgentRuntime(agent graph.Agent) (string, string) {
	switch typed := agent.(type) {
	case *agents.ArchitectAgent:
		return "architect", "plan-execute"
	case *agents.ReActAgent:
		mode := strings.TrimSpace(typed.Mode)
		if mode == "" {
			mode = "react"
		}
		return mode, "react"
	case *agents.ReflectionAgent:
		mode, _ := describeAgentRuntime(typed.Delegate)
		if mode == "" {
			mode = "react"
		}
		return mode, "reflection"
	case *agents.PlannerAgent:
		return "plan", "plan-execute-verify"
	case *agents.EternalAgent:
		return "loop", "eternal"
	default:
		_ = typed
		return "", ""
	}
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
	facts, err := store.ListKnowledge(context.Background(), workflowID, memory.KnowledgeKindFact, false)
	if err != nil {
		return nil, err
	}
	issues, err := store.ListKnowledge(context.Background(), workflowID, memory.KnowledgeKindIssue, false)
	if err != nil {
		return nil, err
	}
	decisions, err := store.ListKnowledge(context.Background(), workflowID, memory.KnowledgeKindDecision, false)
	if err != nil {
		return nil, err
	}
	delegations, err := store.ListDelegations(context.Background(), workflowID, "")
	if err != nil {
		return nil, err
	}
	workflowArtifacts, err := store.ListWorkflowArtifacts(context.Background(), workflowID, "")
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
		Steps:             make([]WorkflowStepInfo, 0, len(steps)),
		Events:            make([]WorkflowEventInfo, 0, len(events)),
		Facts:             make([]WorkflowKnowledgeInfo, 0, len(facts)),
		Issues:            make([]WorkflowKnowledgeInfo, 0, len(issues)),
		Decisions:         make([]WorkflowKnowledgeInfo, 0, len(decisions)),
		Delegations:       make([]WorkflowDelegationInfo, 0, len(delegations)),
		WorkflowArtifacts: make([]WorkflowArtifactInfo, 0, len(workflowArtifacts)),
		ResourceDetails:   []WorkflowLinkedResourceInfo{},
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
	resourceRefs := map[string]struct{}{}
	runIDs := map[string]struct{}{}
	for _, delegation := range delegations {
		if strings.TrimSpace(delegation.RunID) != "" {
			runIDs[delegation.RunID] = struct{}{}
		}
		insertionAction := ""
		if delegation.Result != nil {
			insertionAction = string(delegation.Result.Insertion.Action)
			for _, ref := range delegation.Result.ResourceRefs {
				if strings.TrimSpace(ref) != "" {
					resourceRefs[ref] = struct{}{}
				}
			}
		}
		for _, ref := range delegation.Request.ResourceRefs {
			if strings.TrimSpace(ref) != "" {
				resourceRefs[ref] = struct{}{}
			}
		}
		info.Delegations = append(info.Delegations, WorkflowDelegationInfo{
			DelegationID:       delegation.DelegationID,
			RunID:              delegation.RunID,
			TaskID:             delegation.TaskID,
			State:              string(delegation.State),
			TargetCapabilityID: delegation.Request.TargetCapabilityID,
			TargetProviderID:   delegation.Request.TargetProviderID,
			TargetSessionID:    delegation.Request.TargetSessionID,
			TrustClass:         string(delegation.TrustClass),
			Recoverability:     string(delegation.Recoverability),
			Background:         delegation.Background,
			StartedAt:          delegation.StartedAt,
			UpdatedAt:          delegation.UpdatedAt,
			InsertionAction:    insertionAction,
			ResourceRefs:       append([]string(nil), delegation.Request.ResourceRefs...),
		})
		transitions, err := store.ListDelegationTransitions(context.Background(), delegation.DelegationID)
		if err != nil {
			return nil, err
		}
		for _, transition := range transitions {
			info.Transitions = append(info.Transitions, WorkflowDelegationTransitionInfo{
				DelegationID: transition.DelegationID,
				TransitionID: transition.TransitionID,
				RunID:        transition.RunID,
				FromState:    string(transition.FromState),
				ToState:      string(transition.ToState),
				CreatedAt:    transition.CreatedAt,
			})
		}
	}
	for _, artifact := range workflowArtifacts {
		if strings.TrimSpace(artifact.RunID) != "" {
			runIDs[artifact.RunID] = struct{}{}
		}
		info.WorkflowArtifacts = append(info.WorkflowArtifacts, WorkflowArtifactInfo{
			ArtifactID:  artifact.ArtifactID,
			RunID:       artifact.RunID,
			Kind:        artifact.Kind,
			ContentType: artifact.ContentType,
			SummaryText: artifact.SummaryText,
			CreatedAt:   artifact.CreatedAt,
		})
	}
	for _, runID := range sortedStringKeys(runIDs) {
		providers, err := store.ListProviderSnapshots(context.Background(), workflowID, runID)
		if err != nil {
			return nil, err
		}
		for _, provider := range providers {
			info.Providers = append(info.Providers, WorkflowProviderInfo{
				SnapshotID:     provider.SnapshotID,
				RunID:          provider.RunID,
				ProviderID:     provider.ProviderID,
				Kind:           string(provider.Descriptor.Kind),
				Recoverability: string(provider.Recoverability),
				Health:         provider.Health.Status,
				CapturedAt:     provider.CapturedAt,
			})
		}
		sessions, err := store.ListProviderSessionSnapshots(context.Background(), workflowID, runID)
		if err != nil {
			return nil, err
		}
		for _, session := range sessions {
			info.ProviderSessions = append(info.ProviderSessions, WorkflowProviderSessionInfo{
				SnapshotID:     session.SnapshotID,
				RunID:          session.RunID,
				SessionID:      session.Session.ID,
				ProviderID:     session.Session.ProviderID,
				Health:         session.Session.Health,
				Recoverability: string(session.Session.Recoverability),
				CapturedAt:     session.CapturedAt,
			})
		}
	}
	info.LinkedResources = sortedStringKeys(resourceRefs)
	info.ResourceDetails = describeWorkflowLinkedResources(info.LinkedResources)
	return info, nil
}

func (r *runtimeAdapter) CancelWorkflow(workflowID string) error {
	store, err := r.openWorkflowStore()
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.UpdateWorkflowStatus(context.Background(), workflowID, 0, memory.WorkflowRunStatusCanceled, "")
	return err
}

func (r *runtimeAdapter) openWorkflowStore() (*db.SQLiteWorkflowStateStore, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	path := config.New(r.rt.Config.Workspace).WorkflowStateFile()
	return db.NewSQLiteWorkflowStateStore(path)
}

func convertKnowledgeInfos(records []memory.KnowledgeRecord) []WorkflowKnowledgeInfo {
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

func sortedStringKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (r *runtimeAdapter) SaveToolPolicy(toolName string, level core.AgentPermissionLevel) error {
	if r == nil || r.rt == nil || r.rt.Registration == nil || r.rt.Registration.Manifest == nil {
		return fmt.Errorf("runtime unavailable")
	}
	sourcePath := r.rt.Registration.Manifest.SourcePath
	if sourcePath == "" {
		return fmt.Errorf("manifest source path not set")
	}
	// Reload from disk to avoid saving already-resolved permissions.
	m, err := manifest.LoadAgentManifest(sourcePath)
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
	return manifest.SaveAgentManifest(sourcePath, m)
}

func (r *runtimeAdapter) ListToolsInfo() []ToolInfo {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil
	}
	tools := r.rt.Tools.InspectableTools()
	capabilities := r.rt.Tools.AllCapabilities()
	capsByName := make(map[string]core.CapabilityDescriptor, len(capabilities))
	for _, capability := range capabilities {
		capsByName[capability.Name] = capability
	}
	policies := r.rt.Tools.GetToolPolicies()
	infos := make([]ToolInfo, 0, len(tools))
	for _, t := range tools {
		name := t.Name()
		tags := t.Tags()
		labels := append([]string{}, tags...)
		var riskClasses []string
		var effectClasses []string
		var trustClass string
		runtimeFamily := string(core.CapabilityRuntimeFamilyLocalTool)
		scope := string(core.CapabilityScopeBuiltin)
		exposure := core.CapabilityExposureCallable
		if capability, ok := capsByName[name]; ok {
			runtimeFamily = string(capability.RuntimeFamily)
			scope = string(capability.Source.Scope)
			for _, risk := range capability.RiskClasses {
				riskClasses = append(riskClasses, string(risk))
				labels = append(labels, string(risk))
			}
			for _, effect := range capability.EffectClasses {
				effectClasses = append(effectClasses, string(effect))
				labels = append(labels, string(effect))
			}
			trustClass = string(capability.TrustClass)
			if trustClass != "" {
				labels = append(labels, trustClass)
			}
			exposure = r.rt.Tools.EffectiveExposure(capability)
		}
		pol := policies[name]
		level := core.AgentPermissionLevel(pol.Execute)
		infos = append(infos, ToolInfo{
			Name:          name,
			RuntimeFamily: runtimeFamily,
			Scope:         scope,
			Tags:          tags,
			Labels:        dedupeLowerPreserveOrder(labels),
			RiskClasses:   dedupeLowerPreserveOrder(riskClasses),
			EffectClasses: dedupeLowerPreserveOrder(effectClasses),
			TrustClass:    strings.ToLower(strings.TrimSpace(trustClass)),
			Exposure:      string(exposure),
			Policy:        level,
			HasPolicy:     level != "",
		})
	}
	return infos
}

func (r *runtimeAdapter) ListCapabilities() []CapabilityInfo {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil
	}
	capabilities := r.rt.Tools.AllCapabilities()
	infos := make([]CapabilityInfo, 0, len(capabilities))
	for _, capability := range capabilities {
		infos = append(infos, CapabilityInfo{
			ID:            capability.ID,
			Kind:          string(capability.Kind),
			Name:          capability.Name,
			Description:   capability.Description,
			Category:      capability.Category,
			RuntimeFamily: string(capability.RuntimeFamily),
			TrustClass:    string(capability.TrustClass),
			ProviderID:    capability.Source.ProviderID,
			Scope:         string(capability.Source.Scope),
			Exposure:      string(r.rt.Tools.EffectiveExposure(capability)),
			Callable:      r.rt.Tools.EffectiveExposure(capability) == core.CapabilityExposureCallable,
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Kind == infos[j].Kind {
			return infos[i].Name < infos[j].Name
		}
		return infos[i].Kind < infos[j].Kind
	})
	return infos
}

func (r *runtimeAdapter) ListPrompts() []PromptInfo {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil
	}
	prompts := make([]PromptInfo, 0)
	for _, capability := range r.rt.Tools.AllCapabilities() {
		if capability.Kind != core.CapabilityKindPrompt {
			continue
		}
		exposure := r.rt.Tools.EffectiveExposure(capability)
		prompts = append(prompts, PromptInfo{
			Meta: InspectableMeta{
				ID:            capability.ID,
				Kind:          string(capability.Kind),
				Title:         capability.Name,
				RuntimeFamily: string(capability.RuntimeFamily),
				TrustClass:    string(capability.TrustClass),
				Scope:         string(capability.Source.Scope),
				Source:        fallbackSource(capability.Source.ProviderID, string(capability.Source.Scope)),
				State:         string(exposure),
			},
			PromptID:   capability.ID,
			ProviderID: capability.Source.ProviderID,
		})
	}
	sort.Slice(prompts, func(i, j int) bool { return prompts[i].Meta.Title < prompts[j].Meta.Title })
	return prompts
}

func (r *runtimeAdapter) ListResources(workflowRefs []string) []ResourceInfo {
	resources := make([]ResourceInfo, 0)
	if r != nil && r.rt != nil && r.rt.Tools != nil {
		for _, capability := range r.rt.Tools.AllCapabilities() {
			if capability.Kind != core.CapabilityKindResource {
				continue
			}
			exposure := r.rt.Tools.EffectiveExposure(capability)
			resources = append(resources, ResourceInfo{
				Meta: InspectableMeta{
					ID:            capability.ID,
					Kind:          string(capability.Kind),
					Title:         capability.Name,
					RuntimeFamily: string(capability.RuntimeFamily),
					TrustClass:    string(capability.TrustClass),
					Scope:         string(capability.Source.Scope),
					Source:        fallbackSource(capability.Source.ProviderID, string(capability.Source.Scope)),
					State:         string(exposure),
				},
				ResourceID: capability.ID,
				ProviderID: capability.Source.ProviderID,
			})
		}
	}
	seen := map[string]struct{}{}
	for i := range resources {
		seen[resources[i].ResourceID] = struct{}{}
	}
	for _, raw := range workflowRefs {
		ref, err := memory.ParseWorkflowResourceURI(raw)
		if err != nil {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		resources = append(resources, ResourceInfo{
			Meta: InspectableMeta{
				ID:         raw,
				Kind:       "workflow-resource",
				Title:      describeWorkflowResourceRef(ref),
				TrustClass: string(core.TrustClassWorkspaceTrusted),
				Scope:      ref.WorkflowID,
				Source:     "workflow",
				State:      string(ref.Tier),
			},
			ResourceID:       raw,
			WorkflowResource: true,
			WorkflowURI:      raw,
		})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Meta.Title < resources[j].Meta.Title })
	return resources
}

func (r *runtimeAdapter) GetCapabilityDetail(id string) (*CapabilityDetail, error) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("capability id required")
	}
	for _, capability := range r.rt.Tools.AllCapabilities() {
		if capability.ID != id {
			continue
		}
		exposure := r.rt.Tools.EffectiveExposure(capability)
		detail := &CapabilityDetail{
			Meta: InspectableMeta{
				ID:            capability.ID,
				Kind:          string(capability.Kind),
				Title:         capability.Name,
				RuntimeFamily: string(capability.RuntimeFamily),
				TrustClass:    string(capability.TrustClass),
				Scope:         string(capability.Source.Scope),
				Source:        fallbackSource(capability.Source.ProviderID, string(capability.Source.Scope)),
				State:         string(exposure),
			},
			Description:     capability.Description,
			Category:        capability.Category,
			Exposure:        string(exposure),
			Callable:        exposure == core.CapabilityExposureCallable,
			ProviderID:      capability.Source.ProviderID,
			SessionAffinity: capability.SessionAffinity,
			Availability:    capabilityAvailabilityLabel(capability.Availability),
			RiskClasses:     riskClassStrings(capability.RiskClasses),
			EffectClasses:   effectClassStrings(capability.EffectClasses),
			Tags:            append([]string(nil), capability.Tags...),
		}
		if capability.Coordination != nil {
			detail.CoordinationRole = string(capability.Coordination.Role)
			detail.CoordinationTaskTypes = append([]string(nil), capability.Coordination.TaskTypes...)
		}
		return detail, nil
	}
	return nil, fmt.Errorf("capability %s not found", id)
}

func (r *runtimeAdapter) GetPromptDetail(id string) (*PromptDetail, error) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("prompt id required")
	}
	for _, capability := range r.rt.Tools.AllCapabilities() {
		if capability.ID != id || capability.Kind != core.CapabilityKindPrompt {
			continue
		}
		rendered, err := r.rt.Tools.RenderPrompt(context.Background(), core.NewContext(), capability.ID, nil)
		if err != nil {
			return nil, err
		}
		exposure := r.rt.Tools.EffectiveExposure(capability)
		detail := &PromptDetail{
			Meta: InspectableMeta{
				ID:            capability.ID,
				Kind:          string(capability.Kind),
				Title:         capability.Name,
				RuntimeFamily: string(capability.RuntimeFamily),
				TrustClass:    string(capability.TrustClass),
				Scope:         string(capability.Source.Scope),
				Source:        fallbackSource(capability.Source.ProviderID, string(capability.Source.Scope)),
				State:         string(exposure),
			},
			PromptID:    capability.ID,
			ProviderID:  capability.Source.ProviderID,
			Description: capability.Description,
			Messages:    make([]StructuredPromptMessage, 0, len(rendered.Messages)),
			Metadata:    summarizeAnyMetadata(rendered.Metadata),
		}
		for _, message := range rendered.Messages {
			converted := StructuredPromptMessage{Role: message.Role}
			for _, block := range message.Content {
				converted.Content = append(converted.Content, structuredBlockFromCore(block))
			}
			detail.Messages = append(detail.Messages, converted)
		}
		return detail, nil
	}
	return nil, fmt.Errorf("prompt %s not found", id)
}

func (r *runtimeAdapter) GetResourceDetail(idOrURI string) (*ResourceDetail, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	idOrURI = strings.TrimSpace(idOrURI)
	if idOrURI == "" {
		return nil, fmt.Errorf("resource id required")
	}
	if strings.HasPrefix(idOrURI, "workflow://") {
		return r.getWorkflowResourceDetail(idOrURI)
	}
	if r.rt.Tools == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	for _, capability := range r.rt.Tools.AllCapabilities() {
		if capability.ID != idOrURI || capability.Kind != core.CapabilityKindResource {
			continue
		}
		read, err := r.rt.Tools.ReadResource(context.Background(), core.NewContext(), capability.ID)
		if err != nil {
			return nil, err
		}
		exposure := r.rt.Tools.EffectiveExposure(capability)
		detail := &ResourceDetail{
			Meta: InspectableMeta{
				ID:            capability.ID,
				Kind:          string(capability.Kind),
				Title:         capability.Name,
				RuntimeFamily: string(capability.RuntimeFamily),
				TrustClass:    string(capability.TrustClass),
				Scope:         string(capability.Source.Scope),
				Source:        fallbackSource(capability.Source.ProviderID, string(capability.Source.Scope)),
				State:         string(exposure),
			},
			ResourceID:  capability.ID,
			ProviderID:  capability.Source.ProviderID,
			Description: capability.Description,
			Metadata:    summarizeAnyMetadata(read.Metadata),
		}
		for _, block := range read.Contents {
			detail.Contents = append(detail.Contents, structuredBlockFromCore(block))
		}
		return detail, nil
	}
	return nil, fmt.Errorf("resource %s not found", idOrURI)
}

func (r *runtimeAdapter) ListLiveProviders() []LiveProviderInfo {
	if r == nil || r.rt == nil {
		return nil
	}
	providers, _, err := r.rt.CaptureProviderSnapshots(context.Background())
	if err != nil {
		return nil
	}
	infos := make([]LiveProviderInfo, 0, len(providers))
	for _, provider := range providers {
		infos = append(infos, LiveProviderInfo{
			Meta: InspectableMeta{
				ID:         provider.ProviderID,
				Kind:       string(provider.Descriptor.Kind),
				Title:      provider.ProviderID,
				TrustClass: string(provider.Descriptor.TrustBaseline),
				Source:     provider.Descriptor.ConfiguredSource,
				State:      provider.Health.Status,
				CapturedAt: provider.CapturedAt,
			},
			ProviderID:     provider.ProviderID,
			Kind:           string(provider.Descriptor.Kind),
			TrustBaseline:  string(provider.Descriptor.TrustBaseline),
			Recoverability: string(provider.Recoverability),
			ConfiguredFrom: provider.Descriptor.ConfiguredSource,
			CapabilityIDs:  append([]string(nil), provider.CapabilityIDs...),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ProviderID < infos[j].ProviderID
	})
	return infos
}

func (r *runtimeAdapter) GetLiveProviderDetail(providerID string) (*LiveProviderDetail, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, fmt.Errorf("provider id required")
	}
	providers, _, err := r.rt.CaptureProviderSnapshots(context.Background())
	if err != nil {
		return nil, err
	}
	for _, provider := range providers {
		if provider.ProviderID != providerID {
			continue
		}
		return &LiveProviderDetail{
			Meta: InspectableMeta{
				ID:         provider.ProviderID,
				Kind:       string(provider.Descriptor.Kind),
				Title:      provider.ProviderID,
				TrustClass: string(provider.Descriptor.TrustBaseline),
				Source:     provider.Descriptor.ConfiguredSource,
				State:      provider.Health.Status,
				CapturedAt: provider.CapturedAt,
			},
			ProviderID:     provider.ProviderID,
			Kind:           string(provider.Descriptor.Kind),
			TrustBaseline:  string(provider.Descriptor.TrustBaseline),
			Recoverability: string(provider.Recoverability),
			ConfiguredFrom: provider.Descriptor.ConfiguredSource,
			CapabilityIDs:  append([]string(nil), provider.CapabilityIDs...),
			Metadata:       summarizeAnyMetadata(provider.Metadata),
		}, nil
	}
	return nil, fmt.Errorf("provider %s not found", providerID)
}

func (r *runtimeAdapter) ListLiveSessions() []LiveProviderSessionInfo {
	if r == nil || r.rt == nil {
		return nil
	}
	_, sessions, err := r.rt.CaptureProviderSnapshots(context.Background())
	if err != nil {
		return nil
	}
	infos := make([]LiveProviderSessionInfo, 0, len(sessions))
	for _, session := range sessions {
		infos = append(infos, LiveProviderSessionInfo{
			Meta: InspectableMeta{
				ID:         session.Session.ID,
				Kind:       "session",
				Title:      session.Session.ID,
				TrustClass: string(session.Session.TrustClass),
				Scope:      session.Session.WorkflowID,
				Source:     session.Session.ProviderID,
				State:      session.Session.Health,
				CapturedAt: session.CapturedAt,
			},
			SessionID:       session.Session.ID,
			ProviderID:      session.Session.ProviderID,
			WorkflowID:      session.Session.WorkflowID,
			TaskID:          session.Session.TaskID,
			TrustClass:      string(session.Session.TrustClass),
			Recoverability:  string(session.Session.Recoverability),
			CapabilityIDs:   append([]string(nil), session.Session.CapabilityIDs...),
			LastActivityAt:  session.Session.LastActivityAt,
			MetadataSummary: summarizeMetadata(session.Session.Metadata),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].ProviderID == infos[j].ProviderID {
			return infos[i].SessionID < infos[j].SessionID
		}
		return infos[i].ProviderID < infos[j].ProviderID
	})
	return infos
}

func (r *runtimeAdapter) GetLiveSessionDetail(sessionID string) (*LiveProviderSessionDetail, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id required")
	}
	_, sessions, err := r.rt.CaptureProviderSnapshots(context.Background())
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		if session.Session.ID != sessionID {
			continue
		}
		return &LiveProviderSessionDetail{
			Meta: InspectableMeta{
				ID:         session.Session.ID,
				Kind:       "session",
				Title:      session.Session.ID,
				TrustClass: string(session.Session.TrustClass),
				Scope:      session.Session.WorkflowID,
				Source:     session.Session.ProviderID,
				State:      session.Session.Health,
				CapturedAt: session.CapturedAt,
			},
			SessionID:       session.Session.ID,
			ProviderID:      session.Session.ProviderID,
			WorkflowID:      session.Session.WorkflowID,
			TaskID:          session.Session.TaskID,
			Recoverability:  string(session.Session.Recoverability),
			CapabilityIDs:   append([]string(nil), session.Session.CapabilityIDs...),
			LastActivityAt:  session.Session.LastActivityAt,
			MetadataSummary: summarizeMetadata(session.Session.Metadata),
		}, nil
	}
	return nil, fmt.Errorf("session %s not found", sessionID)
}

func (r *runtimeAdapter) getWorkflowResourceDetail(uri string) (*ResourceDetail, error) {
	store, err := r.openWorkflowStore()
	if err != nil {
		return nil, err
	}
	defer store.Close()
	ref, err := memory.ParseWorkflowResourceURI(uri)
	if err != nil {
		return nil, err
	}
	service := memory.WorkflowProjectionService{Store: store}
	read, err := service.Project(context.Background(), ref)
	if err != nil {
		return nil, err
	}
	detail := &ResourceDetail{
		Meta: InspectableMeta{
			ID:         uri,
			Kind:       "workflow-resource",
			Title:      describeWorkflowResourceRef(ref),
			TrustClass: string(core.TrustClassWorkspaceTrusted),
			Scope:      ref.WorkflowID,
			Source:     "workflow",
			State:      string(ref.Tier),
		},
		ResourceID:       uri,
		WorkflowResource: true,
		WorkflowURI:      uri,
		Description:      fmt.Sprintf("%s workflow projection resource", ref.Tier),
		Metadata:         summarizeAnyMetadata(read.Metadata),
	}
	for _, block := range read.Contents {
		detail.Contents = append(detail.Contents, structuredBlockFromCore(block))
	}
	return detail, nil
}

func (r *runtimeAdapter) ListApprovals() []ApprovalInfo {
	if r == nil || r.rt == nil {
		return nil
	}
	requests := r.rt.PendingHITL()
	infos := make([]ApprovalInfo, 0, len(requests))
	for _, request := range requests {
		if request == nil {
			continue
		}
		infos = append(infos, ApprovalInfo{
			Meta: InspectableMeta{
				ID:         request.ID,
				Kind:       inferApprovalKind(*request),
				Title:      request.Permission.Action,
				Source:     request.Permission.Resource,
				State:      request.State,
				CapturedAt: request.RequestedAt.Format(time.RFC3339),
			},
			ID:             request.ID,
			Kind:           inferApprovalKind(*request),
			PermissionType: string(request.Permission.Type),
			Action:         request.Permission.Action,
			Resource:       request.Permission.Resource,
			Risk:           string(request.Risk),
			Scope:          string(request.Scope),
			Justification:  request.Justification,
			RequestedAt:    request.RequestedAt,
			Metadata:       cloneStringMap(request.Permission.Metadata),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].RequestedAt.Equal(infos[j].RequestedAt) {
			return infos[i].ID < infos[j].ID
		}
		return infos[i].RequestedAt.Before(infos[j].RequestedAt)
	})
	return infos
}

func (r *runtimeAdapter) GetApprovalDetail(id string) (*ApprovalDetail, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("approval id required")
	}
	for _, request := range r.rt.PendingHITL() {
		if request == nil || request.ID != id {
			continue
		}
		return &ApprovalDetail{
			Meta: InspectableMeta{
				ID:         request.ID,
				Kind:       inferApprovalKind(*request),
				Title:      request.Permission.Action,
				Source:     request.Permission.Resource,
				State:      request.State,
				CapturedAt: request.RequestedAt.Format(time.RFC3339),
			},
			ID:             request.ID,
			Kind:           inferApprovalKind(*request),
			PermissionType: string(request.Permission.Type),
			Action:         request.Permission.Action,
			Resource:       request.Permission.Resource,
			Risk:           string(request.Risk),
			Scope:          string(request.Scope),
			Justification:  request.Justification,
			RequestedAt:    request.RequestedAt,
			Metadata:       cloneStringMap(request.Permission.Metadata),
		}, nil
	}
	return nil, fmt.Errorf("approval %s not found", id)
}

func (r *runtimeAdapter) GetClassPolicies() map[string]core.AgentPermissionLevel {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil
	}
	return r.rt.Tools.GetClassPolicies()
}

func (r *runtimeAdapter) SetToolPolicyLive(name string, level core.AgentPermissionLevel) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return
	}
	r.rt.Tools.UpdateToolPolicy(name, core.ToolPolicy{Execute: core.AgentPermissionLevel(level)})
}

func (r *runtimeAdapter) SetClassPolicyLive(class string, level core.AgentPermissionLevel) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return
	}
	r.rt.Tools.UpdateClassPolicy(class, core.AgentPermissionLevel(level))
}

func dedupeLowerPreserveOrder(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func summarizeMetadata(metadata map[string]interface{}) []string {
	if len(metadata) == 0 {
		return nil
	}
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, fmt.Sprintf("%s=%v", key, metadata[key]))
	}
	return out
}

func summarizeAnyMetadata(metadata map[string]any) []string {
	if len(metadata) == 0 {
		return nil
	}
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, fmt.Sprintf("%s=%v", key, metadata[key]))
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func inferApprovalKind(request fauthorization.PermissionRequest) string {
	action := strings.TrimSpace(request.Permission.Action)
	switch {
	case strings.HasPrefix(action, "provider:"):
		return "provider_operation"
	case strings.Contains(action, "insert"):
		return "insertion"
	case strings.Contains(action, "activate"), strings.Contains(action, "admission"):
		return "admission"
	default:
		return "execution"
	}
}

func capabilityAvailabilityLabel(spec core.AvailabilitySpec) string {
	if spec.Available {
		return "available"
	}
	if strings.TrimSpace(spec.Reason) != "" {
		return "unavailable: " + strings.TrimSpace(spec.Reason)
	}
	return "unavailable"
}

func riskClassStrings(values []core.RiskClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func effectClassStrings(values []core.EffectClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func describeWorkflowLinkedResources(refs []string) []WorkflowLinkedResourceInfo {
	if len(refs) == 0 {
		return nil
	}
	out := make([]WorkflowLinkedResourceInfo, 0, len(refs))
	for _, raw := range refs {
		ref, err := memory.ParseWorkflowResourceURI(raw)
		if err != nil {
			out = append(out, WorkflowLinkedResourceInfo{URI: raw, Summary: raw})
			continue
		}
		out = append(out, WorkflowLinkedResourceInfo{
			URI:     raw,
			Tier:    string(ref.Tier),
			Role:    string(ref.Role),
			RunID:   ref.RunID,
			StepID:  ref.StepID,
			Summary: describeWorkflowResourceRef(ref),
		})
	}
	return out
}

func describeWorkflowResourceRef(ref memory.WorkflowResourceRef) string {
	parts := []string{ref.WorkflowID, string(ref.Tier)}
	if ref.Role != "" {
		parts = append(parts, string(ref.Role))
	}
	if ref.StepID != "" {
		parts = append(parts, ref.StepID)
	}
	if ref.RunID != "" {
		parts = append(parts, ref.RunID)
	}
	return strings.Join(parts, " / ")
}

func fallbackSource(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return strings.TrimSpace(fallback)
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

func (r *runtimeAdapter) PendingHITL() []*fauthorization.PermissionRequest {
	if r == nil || r.rt == nil {
		return nil
	}
	return r.rt.PendingHITL()
}

func (r *runtimeAdapter) ApproveHITL(requestID, approver string, scope fauthorization.GrantScope, duration time.Duration) error {
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

func (r *runtimeAdapter) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	if r == nil || r.rt == nil {
		return nil, func() {}
	}
	return r.rt.SubscribeHITL()
}
