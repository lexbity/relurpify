package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

const contextFileMaxBytes = 8000

// ToolInfo describes a registered local tool and its current policy for the config pane.
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
	Policy        agentspec.AgentPermissionLevel // per-tool override; "" means no override
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
	HITLServiceIface
	ExecuteInstruction(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any) (*core.Result, error)
	ExecuteInstructionStream(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any, callback func(string)) (*core.Result, error)
	AvailableAgents() []string
	SwitchAgent(name string) error
	SessionInfo() SessionInfo
	ResolveContextFiles(ctx context.Context, files []string) ContextFileResolution
	SessionArtifacts() SessionArtifacts
	InferenceModels(ctx context.Context) ([]string, error)
	RecordingMode() string
	SetRecordingMode(mode string) error
	SaveModel(model string) error
	ContractSummary() *ContractSummary
	CapabilityAdmissions() []CapabilityAdmissionInfo
	// SaveToolPolicy persists a per-tool execution policy to the agent manifest.
	// toolName is the bare tool name (e.g. "cli_mkdir"); level is typically AgentPermissionAllow.
	SaveToolPolicy(toolName string, level agentspec.AgentPermissionLevel) error
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
	GetClassPolicies() map[string]agentspec.AgentPermissionLevel
	// SetToolPolicyLive updates a per-tool execution policy in-memory (current session only).
	// Pass level="" to clear the override.
	SetToolPolicyLive(name string, level agentspec.AgentPermissionLevel)
	// SetClassPolicyLive updates a class permission policy in-memory (current session only).
	// Pass level="" to clear the class policy.
	SetClassPolicyLive(class string, level agentspec.AgentPermissionLevel)
	ListWorkflows(limit int) ([]WorkflowInfo, error)
	GetWorkflow(workflowID string) (*WorkflowDetails, error)
	CancelWorkflow(workflowID string) error
	// InvokeCapability invokes a registered capability by name through the
	// capability registry, applying the same policy, HITL, audit, and sandbox
	// enforcement that applies to agent tool calls.
	InvokeCapability(ctx context.Context, name string, args map[string]any) (*contracts.ToolResult, error)
	// Diagnostics returns a snapshot of runtime resource and agent state for
	// display in the session live subtab.
	Diagnostics() DiagnosticsInfo
	// ApplyChatPolicy hints to the runtime that the user has switched to a
	// chat subtab with a specific execution policy. Implementations may update
	// the agent mode, tool enablement, or context strategy accordingly.
	// The TUI continues regardless of whether this call returns an error.
	ApplyChatPolicy(subtab SubTabID) error
	// Service management
	ListServices() []ServiceInfo
	StopService(id string) error
	RestartService(ctx context.Context, id string) error
	RestartAllServices(ctx context.Context) error
	// Context file management
	AddFileToContext(path string) error
	DropFileFromContext(path string) error
	// ActiveWorkflowID returns the current active workflow ID (empty if none).
	ActiveWorkflowID() string
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
	info.Provider = cfg.InferenceProvider
	info.Model = cfg.InferenceModel
	info.Agent = cfg.AgentLabel()
	if r.rt.ProfileResolution.Profile != nil {
		info.Profile = r.rt.ProfileResolution.Profile.MatchPattern()
	}
	info.ProfileReason = r.rt.ProfileResolution.Reason
	info.ProfileSource = r.rt.ProfileResolution.SourcePath
	if r.rt.Backend != nil {
		if health, err := r.rt.Backend.Health(context.Background()); err == nil && health != nil {
			info.BackendState = string(health.State)
		}
	}

	if r.rt.Registration != nil && r.rt.Registration.Manifest != nil {
		manifest := r.rt.Registration.Manifest
		info.Agent = manifest.Metadata.Name
		if manifest.Spec.Agent != nil {
			if manifest.Spec.Agent.Model.Provider != "" {
				info.Provider = manifest.Spec.Agent.Model.Provider
			}
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

func describeAgentRuntime(agent agentgraph.WorkflowExecutor) (string, string) {
	switch typed := agent.(type) {
	case *euclo.Agent:
		return "euclo", "route-dispatch"
	default:
		_ = typed
		return "", ""
	}
}

func (r *runtimeAdapter) ResolveContextFiles(ctx context.Context, files []string) ContextFileResolution {
	paths := normalizePaths(files)
	res := ContextFileResolution{
		Allowed:  make([]string, 0, len(paths)),
		Contents: make([]ContextFileContent, 0, len(paths)),
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
			if err := perm.CheckFileAccess(ctx, r.rt.Registration.ID, contracts.FileSystemRead, abs); err != nil {
				res.Denied[path] = err.Error()
				continue
			}
		}
		result, err := r.InvokeCapability(ctx, "file_read", map[string]any{"path": abs})
		if err != nil {
			res.Denied[path] = err.Error()
			continue
		}
		if result == nil {
			res.Denied[path] = "file_read returned no result"
			continue
		}
		if !result.Success {
			msg := strings.TrimSpace(result.Error)
			if msg == "" {
				msg = "file_read failed"
			}
			res.Denied[path] = msg
			continue
		}
		content, _ := result.Data["content"].(string)
		if content == "" {
			res.Denied[path] = "file_read returned no content"
			continue
		}
		truncated := false
		if len(content) > contextFileMaxBytes {
			content = content[:contextFileMaxBytes]
			truncated = true
		}
		res.Allowed = append(res.Allowed, abs)
		res.Contents = append(res.Contents, ContextFileContent{
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

func (r *runtimeAdapter) InferenceModels(ctx context.Context) ([]string, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	var models []string
	if r.rt.Backend != nil {
		backendModels, err := r.rt.Backend.ListModels(ctx)
		if err != nil {
			return nil, err
		}
		for _, model := range backendModels {
			models = append(models, model.Name)
		}
		return models, nil
	}
	backend, err := llm.New(llm.ProviderConfigFromRuntimeConfig(r.rt.Config))
	if err != nil {
		return nil, err
	}
	defer backend.Close()
	backendModels, err := backend.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	for _, model := range backendModels {
		models = append(models, model.Name)
	}
	return models, nil
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
	provider := strings.TrimSpace(r.rt.Config.InferenceProvider)
	if provider == "" {
		provider = strings.TrimSpace(r.SessionInfo().Provider)
	}
	wsCfg.Provider = provider
	wsCfg.Model = model
	if backend := strings.TrimSpace(r.rt.Config.SandboxBackend); backend != "" {
		wsCfg.SandboxBackend = backend
	}
	wsCfg.LastUpdated = time.Now().Unix()
	return runtimesvc.SaveWorkspaceConfig(cfgPath, wsCfg)
}

func (r *runtimeAdapter) ListWorkflows(limit int) ([]WorkflowInfo, error) {
	// TODO: Reimplement without SQLiteWorkflowStateStore dependency
	// per the agentlifecycle workflow-store removal plan
	return nil, nil
}

func (r *runtimeAdapter) GetWorkflow(workflowID string) (*WorkflowDetails, error) {
	// TODO: Reimplement without SQLiteWorkflowStateStore dependency
	// per the agentlifecycle workflow-store removal plan
	return nil, fmt.Errorf("workflow details not available during migration")
}

func (r *runtimeAdapter) CancelWorkflow(workflowID string) error {
	// TODO: Reimplement without SQLiteWorkflowStateStore dependency
	// per the agentlifecycle workflow-store removal plan
	return fmt.Errorf("workflow cancellation not available during migration")
}

func (r *runtimeAdapter) InvokeCapability(ctx context.Context, name string, args map[string]any) (*contracts.ToolResult, error) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil, fmt.Errorf("capability registry unavailable")
	}
	env := contextdata.NewEnvelope("", "")
	return r.rt.Tools.InvokeCapability(ctx, env, name, args)
}

func (r *runtimeAdapter) getWorkflowResourceDetail(uri string) (*ResourceDetail, error) {
	// TODO: Reimplement without SQLiteWorkflowStateStore dependency
	// per the agentlifecycle workflow-store removal plan
	return nil, fmt.Errorf("workflow resource details not available during migration")
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

func (r *runtimeAdapter) ListCapabilities() []CapabilityInfo {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil
	}
	caps := r.rt.Tools.AllCapabilities()
	out := make([]CapabilityInfo, 0, len(caps))
	for _, cap := range caps {
		out = append(out, CapabilityInfo{
			Name:        cap.Name,
			Description: cap.Description,
			Kind:        string(cap.Kind),
		})
	}
	return out
}

func (r *runtimeAdapter) ListPrompts() []PromptInfo {
	return nil
}

func (r *runtimeAdapter) ListResources(workflowRefs []string) []ResourceInfo {
	_ = workflowRefs
	return nil
}

func (r *runtimeAdapter) GetCapabilityDetail(id string) (*CapabilityDetail, error) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("capability id required")
	}
	for _, cap := range r.ListCapabilities() {
		if cap.ID != id && cap.Name != id {
			continue
		}
		return &CapabilityDetail{
			Meta: InspectableMeta{
				ID:    cap.ID,
				Kind:  cap.Kind,
				Title: cap.Name,
			},
			Description:     cap.Description,
			Category:        cap.Category,
			Exposure:        cap.Exposure,
			Callable:        cap.Callable,
			ProviderID:      cap.ProviderID,
			SessionAffinity: cap.Scope,
		}, nil
	}
	return nil, fmt.Errorf("capability %s not found", id)
}

func (r *runtimeAdapter) GetPromptDetail(id string) (*PromptDetail, error) {
	return nil, fmt.Errorf("prompt details not available")
}

func (r *runtimeAdapter) GetResourceDetail(idOrURI string) (*ResourceDetail, error) {
	return nil, fmt.Errorf("resource details not available")
}

func (r *runtimeAdapter) ListToolsInfo() []ToolInfo {
	// TODO: Implement tool info listing
	return nil
}

func (r *runtimeAdapter) ListLiveProviders() []LiveProviderInfo {
	// TODO: Implement live provider listing
	return nil
}

func (r *runtimeAdapter) GetLiveProviderDetail(providerID string) (*LiveProviderDetail, error) {
	return nil, fmt.Errorf("live provider details not available")
}

func (r *runtimeAdapter) GetLiveSessionDetail(sessionID string) (*LiveProviderSessionDetail, error) {
	return nil, fmt.Errorf("live session details not available")
}

func (r *runtimeAdapter) ListLiveSessions() []LiveProviderSessionInfo {
	// TODO: Implement live session listing
	return nil
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

func (r *runtimeAdapter) GetClassPolicies() map[string]agentspec.AgentPermissionLevel {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil
	}
	return r.rt.Tools.GetClassPolicies()
}

func (r *runtimeAdapter) SetToolPolicyLive(name string, level agentspec.AgentPermissionLevel) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return
	}
	r.rt.Tools.UpdateToolPolicy(name, agentspec.ToolPolicy{Execute: agentspec.AgentPermissionLevel(level)})
}

func (r *runtimeAdapter) SetClassPolicyLive(class string, level agentspec.AgentPermissionLevel) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return
	}
	r.rt.Tools.UpdateClassPolicy(class, agentspec.AgentPermissionLevel(level))
}

func (r *runtimeAdapter) SaveToolPolicy(toolName string, level agentspec.AgentPermissionLevel) error {
	_ = toolName
	_ = level
	return nil
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
			Role:    string(ref.Role),
			RunID:   ref.RunID,
			StepID:  ref.StepID,
			Summary: describeWorkflowResourceRef(ref),
		})
	}
	return out
}

func describeWorkflowResourceRef(ref memory.WorkflowResourceURI) string {
	parts := []string{ref.WorkflowID}
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

func (r *runtimeAdapter) Diagnostics() DiagnosticsInfo {
	if r == nil || r.rt == nil {
		return DiagnosticsInfo{}
	}
	d := DiagnosticsInfo{}

	// Capabilities.
	if r.rt.Tools != nil {
		d.CapabilitiesTotal = len(r.rt.Tools.AllCapabilities())
	}

	// Pending approvals and live providers.
	d.PendingApprovals = len(r.ListApprovals())
	d.LiveProviders = len(r.ListLiveProviders())

	// Active workflows from store.
	// TODO: Reimplement without WorkflowStore dependency
	// per the agentlifecycle workflow-store removal plan

	// Agent mode and profile from session info.
	info := r.SessionInfo()
	d.ActiveMode = info.Mode
	d.ActiveProfile = info.Profile
	d.ProfileReason = info.ProfileReason
	d.ProfileSource = info.ProfileSource
	if r.rt.Registration != nil && r.rt.Registration.ManifestSnapshot != nil {
		d.ManifestFingerprint = fmt.Sprintf("%x", r.rt.Registration.ManifestSnapshot.Fingerprint)
	}
	if r.rt.Config.Workspace != "" {
		d.ProtectedPaths = manifest.New(r.rt.Config.Workspace).GovernanceRoots(
			r.rt.Config.ManifestPath,
			r.rt.Config.ConfigPath,
		)
	}
	if r.rt.Registration != nil && r.rt.Registration.Manifest != nil {
		d.ManifestPolicy = manifestPolicySummary(r.rt.Registration.Manifest)
		d.DeprecationNotices = append([]string(nil), r.rt.Registration.Manifest.Spec.CompatibilityWarnings...)
	}

	return d
}

func manifestPolicySummary(m *manifest.AgentManifest) string {
	if m == nil {
		return ""
	}
	parts := []string{}
	if m.Spec.Policy != nil {
		policy := m.Spec.Policy
		permCount := len(policy.Permissions.FileSystem) + len(policy.Permissions.Executables) + len(policy.Permissions.Network)
		if permCount > 0 {
			parts = append(parts, fmt.Sprintf("policy-perms=%d", permCount))
		}
		if len(policy.Policies) > 0 {
			parts = append(parts, fmt.Sprintf("policy-rules=%d", len(policy.Policies)))
		}
		if policy.Defaults != nil {
			if policy.Defaults.Permissions != nil {
				defaultPerms := policy.Defaults.Permissions
				parts = append(parts, fmt.Sprintf("defaults=%d/%d/%d", len(defaultPerms.FileSystem), len(defaultPerms.Executables), len(defaultPerms.Network)))
			}
		}
	}
	if m.Spec.Agent != nil {
		parts = append(parts, fmt.Sprintf("tool-calling=%s", m.Spec.Agent.ResolveToolCallingIntent()))
	}
	return strings.Join(parts, ", ")
}

func (r *runtimeAdapter) ApplyChatPolicy(subtab SubTabID) error {
	if r == nil || r.rt == nil {
		return nil
	}
	// The policy is a TUI hint; no runtime enforcement needed beyond
	// propagating the mode hint via metadata on the next ExecuteInstruction
	// call (which happens via buildMetadata in ChatPane). Nothing to do here.
	return nil
}

// Service management methods
func (r *runtimeAdapter) ListServices() []ServiceInfo {
	if r == nil || r.rt == nil || r.rt.ServiceManager == nil {
		return nil
	}
	ids := r.rt.ServiceManager.ListIDs()
	infos := make([]ServiceInfo, 0, len(ids))
	for _, id := range ids {
		infos = append(infos, ServiceInfo{
			ID:     id,
			Status: ServiceStatusRunning, // registered services are considered running
		})
	}
	return infos
}

func (r *runtimeAdapter) StopService(id string) error {
	if r == nil || r.rt == nil || r.rt.ServiceManager == nil {
		return fmt.Errorf("runtime unavailable")
	}
	svc := r.rt.ServiceManager.Get(id)
	if svc == nil {
		return fmt.Errorf("service %s not found", id)
	}
	return svc.Stop()
}

func (r *runtimeAdapter) RestartService(ctx context.Context, id string) error {
	if r == nil || r.rt == nil || r.rt.ServiceManager == nil {
		return fmt.Errorf("runtime unavailable")
	}
	svc := r.rt.ServiceManager.Get(id)
	if svc == nil {
		return fmt.Errorf("service %s not found", id)
	}
	if err := svc.Stop(); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return svc.Start(ctx)
}

func (r *runtimeAdapter) RestartAllServices(ctx context.Context) error {
	if r == nil || r.rt == nil || r.rt.ServiceManager == nil {
		return fmt.Errorf("runtime unavailable")
	}
	if err := r.rt.ServiceManager.StopAll(); err != nil {
		return fmt.Errorf("stop all: %w", err)
	}
	return r.rt.ServiceManager.StartAll(ctx)
}

// Context file management
func (r *runtimeAdapter) AddFileToContext(path string) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	return nil
}

func (r *runtimeAdapter) DropFileFromContext(path string) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	return nil
}

func (r *runtimeAdapter) QueryPatternProposals(scope string) ([]PatternProposalInfo, error) {
	_ = scope
	return nil, nil
}

func (r *runtimeAdapter) QueryConfirmedPatterns(scope string) ([]PatternRecordInfo, error) {
	_ = scope
	return nil, nil
}

func (r *runtimeAdapter) QueryIntentGaps(filePath, scope string) ([]IntentGapInfo, error) {
	if r == nil || r.rt == nil {
		return nil, nil
	}
	// TODO: Reimplement retrieval drift/anchor queries without DB() dependency
	// These operations previously used WorkflowStore.DB() which is being removed
	// per the agentlifecycle workflow-store removal plan
	return nil, nil
}

func (r *runtimeAdapter) QueryTensions(scope string) ([]TensionInfo, error) {
	if r == nil || r.rt == nil {
		return nil, nil
	}
	// TODO: Reimplement without WorkflowStore dependency
	// per the agentlifecycle workflow-store removal plan
	return nil, nil
}

func (r *runtimeAdapter) LoadLivePlan(workflowID string) (*LivePlanInfo, error) {
	_ = workflowID
	return nil, nil
}

func (r *runtimeAdapter) AddPlanNote(stepRef string, body string) error {
	_ = stepRef
	_ = body
	return nil
}

func (r *runtimeAdapter) GetPlanDiff(workflowID string) (PlanDiffInfo, error) {
	info := PlanDiffInfo{WorkflowID: workflowID}
	plan, err := r.LoadLivePlan(workflowID)
	if err != nil || plan == nil {
		return info, err
	}
	info.WorkflowID = plan.WorkflowID
	info.Steps = append([]PlanStepInfo(nil), plan.Steps...)
	// TODO: Reimplement retrieval drift queries without DB() dependency
	// These operations previously used WorkflowStore.DB() which is being removed
	// per the agentlifecycle workflow-store removal plan
	return info, nil
}

func (r *runtimeAdapter) GetLatestTrace() (TraceInfo, error) {
	return TraceInfo{}, nil
}

// ActiveWorkflowID satisfies RuntimeAdapter.
func (r *runtimeAdapter) ActiveWorkflowID() string { return r.activeWorkflowID() }

func (r *runtimeAdapter) activeWorkflowID() string {
	return ""
}

func (r *runtimeAdapter) workspaceRoot() string {
	if r == nil || r.rt == nil || strings.TrimSpace(r.rt.Config.Workspace) == "" {
		return "."
	}
	return r.rt.Config.Workspace
}

func normalizeGoPackageArg(pkg string) string {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return "./..."
	}
	return pkg
}

func splitNonEmptyLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func combineToolOutput(result *contracts.ToolResult) []byte {
	if result == nil {
		return nil
	}
	stdout, _ := result.Data["stdout"].(string)
	stderr, _ := result.Data["stderr"].(string)
	switch {
	case stdout == "":
		return []byte(stderr)
	case stderr == "":
		return []byte(stdout)
	default:
		return []byte(stdout + "\n" + stderr)
	}
}

func (r *runtimeAdapter) planNotes(workflowID string) map[string][]string {
	out := map[string][]string{}
	if r == nil || r.rt == nil || workflowID == "" {
		return out
	}
	// TODO: Reimplement without WorkflowStore dependency
	// per the agentlifecycle workflow-store removal plan
	return out
}

func matchesCorpusScope(scope, corpusScope string, instances []patterns.PatternInstance) bool {
	scope = normalizeScope(scope)
	corpusScope = normalizeScope(corpusScope)
	if scope == "" || corpusScope == "" {
		return true
	}
	if strings.Contains(corpusScope, scope) || strings.Contains(scope, corpusScope) {
		return true
	}
	for _, instance := range instances {
		if strings.Contains(normalizeScope(instance.FilePath), scope) {
			return true
		}
	}
	return false
}

func normalizeScope(scope string) string {
	scope = strings.TrimSpace(scope)
	scope = strings.TrimPrefix(scope, "./")
	return filepath.Clean(scope)
}

func parseDriftSeverity(detail string) string {
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "severity:critical"):
		return "critical"
	case strings.Contains(lower, "severity:significant"):
		return "significant"
	default:
		return "minor"
	}
}

func mapPlanAnchors(ids []string) []AnchorRef {
	out := make([]AnchorRef, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		out = append(out, AnchorRef{Name: id, Status: "active"})
	}
	return out
}

func convertAnchors(anchorIDs []string) []AnchorRef {
	out := make([]AnchorRef, 0, len(anchorIDs))
	for _, id := range anchorIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		out = append(out, AnchorRef{
			Name:   id,
			Class:  "technical", // Default class
			Status: "active",
		})
	}
	return out
}
