package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const contextFileMaxBytes = 8000

// DoctorReport is the runtime readiness report surfaced in the base shell.
type DoctorReport = runtimesvc.DoctorReport

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
	ExecuteInstruction(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any) (*execution.Result, error)
	ExecuteInstructionStream(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error)
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
	// LoadSandboxManifest returns the current workspace manifest spec.
	LoadSandboxManifest() (*config.ManifestSpec, error)
	// SaveSandboxManifest persists a sandbox manifest spec with backup.
	SaveSandboxManifest(m *config.ManifestSpec) (string, error)
	// SandboxBackend returns the active sandbox backend name.
	SandboxBackend() string
	// SaveSandboxBackend persists the active sandbox backend in workspace config.
	SaveSandboxBackend(backend string) (string, error)
	// ExecutionMode returns the current workspace execution posture.
	ExecutionMode() config.ExecutionMode
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
	InvokeCapability(ctx context.Context, name string, args map[string]any) (*ports.ToolResult, error)
	// Diagnostics returns a snapshot of runtime resource and agent state for
	// display in the session live subtab.
	Diagnostics() DiagnosticsInfo
	// BuildDoctorReport computes the workspace readiness report used by the
	// base-framework Doctor tab.
	BuildDoctorReport(ctx context.Context) DoctorReport
	// ReloadWorkspace rebuilds the runtime against a different workspace root.
	ReloadWorkspace(ctx context.Context, workspace string) error
	// InitializeWorkspaceFromTemplates materializes the bundled relurpify_cfg
	// tree into the active workspace.
	InitializeWorkspaceFromTemplates(overwrite bool) error
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

	// ResumeSession rehydrates a session from a workflow ID, returning
	// a non-nil error when the workflow cannot be resumed.
	ResumeSession(ctx context.Context, workflowID string) error
	// ResolveInteractionFrame writes a resolved interaction response back into
	// the live runtime envelope for the given task.
	ResolveInteractionFrame(ctx context.Context, taskID, frameID, choice, freetext string) error
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

func (r *runtimeAdapter) ExecuteInstruction(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any) (*execution.Result, error) {
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
		Workspace:     "",
		Model:         "",
		Agent:         "",
		Role:          "",
		Mode:          "",
		ExecutionMode: "",
		Strategy:      "",
		MaxTokens:     100000,
	}
	if r == nil || r.rt == nil {
		return info
	}
	cfg := r.rt.Config
	info.Workspace = cfg.Workspace
	info.Provider = cfg.InferenceProvider
	info.Model = cfg.InferenceModel
	info.Agent = cfg.AgentLabel()
	if r.rt.AgentWorkspace().ProfileResolution.Profile != nil {
		info.Profile = r.rt.AgentWorkspace().ProfileResolution.Profile.MatchPattern()
	}
	info.ProfileReason = r.rt.AgentWorkspace().ProfileResolution.Reason
	info.ProfileSource = r.rt.AgentWorkspace().ProfileResolution.SourcePath
	if be := r.rt.AgentWorkspace().Backend; be != nil {
		if mb, ok := be.(llm.ManagedBackend); ok {
			if health, err := mb.Health(context.Background()); err == nil && health != nil {
				info.BackendState = string(health.State)
			}
		}
	}

	if r.rt.AgentWorkspace().Registration != nil && r.rt.AgentWorkspace().Registration.ManifestSpec != nil {
		spec := r.rt.AgentWorkspace().Registration.ManifestSpec
		if spec.Agent != nil {
			if spec.Agent.Model.Provider != "" {
				info.Provider = spec.Agent.Model.Provider
			}
			if spec.Agent.Model.Name != "" {
				info.Model = spec.Agent.Model.Name
			}
			if spec.Agent.Mode != "" {
				info.Role = string(spec.Agent.Mode)
			}
			if spec.Agent.Context.MaxTokens > 0 {
				info.MaxTokens = spec.Agent.Context.MaxTokens
			}
		}
	}
	info.Mode, info.Strategy = describeAgentRuntime(r.rt.Agent)
	info.ExecutionMode = string(r.ExecutionMode())
	return info
}

func (r *runtimeAdapter) ContractSummary() *ContractSummary {
	if r == nil || r.rt == nil || r.rt.AgentWorkspace().EffectiveContract == nil {
		return nil
	}
	summary := &ContractSummary{
		AgentID:         r.rt.AgentWorkspace().EffectiveContract.AgentID,
		ManifestName:    r.rt.AgentWorkspace().EffectiveContract.Sources.ManifestName,
		ManifestVersion: r.rt.AgentWorkspace().EffectiveContract.Sources.ManifestVersion,
		Workspace:       r.rt.AgentWorkspace().EffectiveContract.Sources.Workspace,
		AppliedSkills:   nil,
		FailedSkills:    nil,
		AdmissionCount:  len(r.rt.AgentWorkspace().CapabilityAdmissions),
	}
	if r.rt.Tools != nil {
		summary.CapabilityCount = len(r.rt.Tools.AllCapabilities())
	}
	for _, admission := range r.rt.AgentWorkspace().CapabilityAdmissions {
		if !admission.Admitted {
			summary.RejectedCount++
		}
	}
	if r.rt.AgentWorkspace().CompiledPolicy != nil {
		summary.PolicyRuleCount = len(r.rt.AgentWorkspace().CompiledPolicy.Rules)
	}
	return summary
}

func (r *runtimeAdapter) CapabilityAdmissions() []CapabilityAdmissionInfo {
	if r == nil || r.rt == nil {
		return nil
	}
	out := make([]CapabilityAdmissionInfo, 0, len(r.rt.AgentWorkspace().CapabilityAdmissions))
	for _, admission := range r.rt.AgentWorkspace().CapabilityAdmissions {
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

type runtimeProfileProvider interface {
	RuntimeProfile() (mode, strategy string)
}

func describeAgentRuntime(agent agentgraph.WorkflowExecutor) (string, string) {
	if typed, ok := agent.(runtimeProfileProvider); ok {
		return typed.RuntimeProfile()
	}
	return "", ""
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
	perm := r.rt.AgentWorkspace().Registration.Permissions

	for _, path := range paths {
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(workspace, abs)
		}
		abs = filepath.Clean(abs)

		if perm != nil {
			if err := perm.CheckFileAccess(ctx, r.rt.AgentWorkspace().Registration.ID, permissions.FileSystemRead, abs); err != nil {
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

func (r *runtimeAdapter) ExecuteInstructionStream(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error) {
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
	if be := r.rt.AgentWorkspace().Backend; be != nil {
		if mb, ok := be.(llm.ManagedBackend); ok {
			backendModels, err := mb.ListModels(ctx)
			if err != nil {
				return nil, err
			}
			for _, model := range backendModels {
				models = append(models, model.Name)
			}
			return models, nil
		}
	}
	backend, err := llm.New(llm.ProviderConfigFromRuntimeConfig(r.rt.Config), r.rt.ProviderSecrets())
	if err != nil {
		return nil, err
	}
	defer func() { _ = backend.Close() }()
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
	workspace := strings.TrimSpace(r.rt.Config.Workspace)
	if workspace == "" {
		return fmt.Errorf("workspace not set")
	}
	path := config.New(workspace).RuntimeProvidersFile()
	profile, err := config.LoadRuntimeProviderConfig(path)
	if err != nil {
		profile = config.RuntimeProviderConfig{}
	}
	if profile.Provider == "" {
		profile.Provider = strings.TrimSpace(r.rt.Config.InferenceProvider)
	}
	if profile.Provider == "" {
		profile.Provider = strings.TrimSpace(r.SessionInfo().Provider)
	}
	profile.Endpoint = strings.TrimSpace(r.rt.Config.InferenceEndpoint)
	profile.Model = strings.TrimSpace(model)
	profile.NativeToolCalling = r.rt.Config.InferenceNativeToolCalling
	if profile.Timeout == "" {
		profile.Timeout = "30s"
	}
	profile.LastUpdated = time.Now().Unix()
	if _, err := config.SaveRuntimeProviderConfigWithBackup(path, profile); err != nil {
		return err
	}
	r.rt.Config.InferenceProvider = profile.Provider
	r.rt.Config.InferenceEndpoint = profile.Endpoint
	r.rt.Config.InferenceModel = model
	r.rt.Config.InferenceNativeToolCalling = profile.NativeToolCalling
	r.rt.WorkspaceConfig.Provider = profile.Provider
	r.rt.WorkspaceConfig.Model = model
	return nil
}

func (r *runtimeAdapter) ListWorkflows(limit int) ([]WorkflowInfo, error) {
	return nil, nil
}

func (r *runtimeAdapter) GetWorkflow(workflowID string) (*WorkflowDetails, error) {
	return nil, fmt.Errorf("workflow details not available during migration")
}

func (r *runtimeAdapter) CancelWorkflow(workflowID string) error {
	return fmt.Errorf("workflow cancellation not available during migration")
}

func (r *runtimeAdapter) InvokeCapability(ctx context.Context, name string, args map[string]any) (*ports.ToolResult, error) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return nil, fmt.Errorf("capability registry unavailable")
	}
	env := contextdata.NewEnvelope("", "")
	return r.rt.Tools.InvokeCapability(ctx, env.State(), name, args)
}

func (r *runtimeAdapter) getWorkflowResourceDetail(uri string) (*ResourceDetail, error) {
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

func (r *runtimeAdapter) PromptRegistry() prompt.Registry {
	if r == nil || r.rt == nil || r.rt.AgentWorkspace() == nil {
		return nil
	}
	return r.rt.AgentWorkspace().Environment.PromptRegistry
}

func (r *runtimeAdapter) ListPrompts() []PromptInfo {
	reg := r.PromptRegistry()
	if reg == nil {
		return nil
	}
	cfgs := reg.All()
	out := make([]PromptInfo, 0, len(cfgs))
	for _, cfg := range cfgs {
		if cfg == nil {
			continue
		}
		vars := make([]string, 0, len(cfg.Variables))
		for name := range cfg.Variables {
			vars = append(vars, name)
		}
		sort.Strings(vars)
		out = append(out, PromptInfo{
			Meta: InspectableMeta{
				ID:     cfg.ID,
				Kind:   "prompt",
				Title:  cfg.ID,
				Source: cfg.SourcePath,
				State:  strings.Join(cfg.Tags, ", "),
			},
			PromptID:    cfg.ID,
			ProviderID:  "local",
			Tags:        append([]string(nil), cfg.Tags...),
			Variables:   vars,
			Description: strings.TrimSpace(cfg.Body),
		})
	}
	return out
}

func (r *runtimeAdapter) ListResources(workflowRefs []string) []ResourceInfo {
	if len(workflowRefs) == 0 {
		prompts := r.ListPrompts()
		out := make([]ResourceInfo, 0, len(prompts))
		for _, promptInfo := range prompts {
			out = append(out, ResourceInfo{
				Meta: InspectableMeta{
					ID:     promptInfo.PromptID,
					Kind:   "prompt",
					Title:  promptInfo.Meta.Title,
					Source: promptInfo.Meta.Source,
					State:  promptInfo.Meta.State,
				},
				ResourceID: promptInfo.PromptID,
			})
		}
		return out
	}
	out := make([]ResourceInfo, 0, len(workflowRefs))
	for _, ref := range workflowRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		out = append(out, ResourceInfo{
			Meta: InspectableMeta{
				ID:     ref,
				Kind:   "workflow-resource",
				Title:  ref,
				Source: ref,
				State:  "linked",
			},
			ResourceID:       ref,
			WorkflowResource: true,
			WorkflowURI:      ref,
		})
	}
	return out
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
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("prompt id required")
	}
	reg := r.PromptRegistry()
	if reg == nil {
		return nil, fmt.Errorf("prompt registry unavailable")
	}
	cfg, ok := reg.Get(id)
	if !ok || cfg == nil {
		return nil, fmt.Errorf("prompt %s not found", id)
	}
	vars := make([]string, 0, len(cfg.Variables))
	for name := range cfg.Variables {
		vars = append(vars, name)
	}
	sort.Strings(vars)
	metadata := []string{}
	if len(cfg.Tags) > 0 {
		metadata = append(metadata, "tags: "+strings.Join(cfg.Tags, ", "))
	}
	if len(vars) > 0 {
		metadata = append(metadata, "variables: "+strings.Join(vars, ", "))
	}
	if cfg.SourcePath != "" {
		metadata = append(metadata, "source: "+cfg.SourcePath)
	}
	body := strings.TrimSpace(cfg.Body)
	if body == "" {
		body = "(empty prompt body)"
	}
	return &PromptDetail{
		Meta: InspectableMeta{
			ID:     cfg.ID,
			Kind:   "prompt",
			Title:  cfg.ID,
			Source: cfg.SourcePath,
			State:  strings.Join(cfg.Tags, ", "),
		},
		PromptID:    cfg.ID,
		ProviderID:  "local",
		Description: strings.TrimSpace(cfg.Body),
		Messages: []StructuredPromptMessage{{
			Role: "system",
			Content: []StructuredContentBlock{{
				Type:       "text",
				Summary:    "prompt body",
				Body:       body,
				Provenance: map[string]string{"source": cfg.SourcePath},
			}},
		}},
		Metadata: metadata,
	}, nil
}

func (r *runtimeAdapter) GetResourceDetail(idOrURI string) (*ResourceDetail, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	idOrURI = strings.TrimSpace(idOrURI)
	if idOrURI == "" {
		return nil, fmt.Errorf("resource id required")
	}
	if detail, err := r.getWorkflowResourceDetail(idOrURI); err == nil && detail != nil {
		return detail, nil
	}
	reg := r.PromptRegistry()
	if reg == nil {
		return nil, fmt.Errorf("resource details not available")
	}
	cfg, ok := reg.Get(idOrURI)
	if !ok || cfg == nil {
		return nil, fmt.Errorf("resource %s not found", idOrURI)
	}
	var names []string
	for name := range cfg.Variables {
		names = append(names, name)
	}
	sort.Strings(names)
	metadata := []string{}
	if len(cfg.Tags) > 0 {
		metadata = append(metadata, "tags: "+strings.Join(cfg.Tags, ", "))
	}
	if len(names) > 0 {
		metadata = append(metadata, "variables: "+strings.Join(names, ", "))
	}
	if cfg.SourcePath != "" {
		metadata = append(metadata, "source: "+cfg.SourcePath)
	}
	body := strings.TrimSpace(cfg.Body)
	if body == "" {
		body = "(empty prompt body)"
	}
	return &ResourceDetail{
		Meta: InspectableMeta{
			ID:     cfg.ID,
			Kind:   "prompt-resource",
			Title:  cfg.ID,
			Source: cfg.SourcePath,
			State:  "ready",
		},
		ResourceID:  cfg.ID,
		ProviderID:  "local",
		Description: strings.TrimSpace(cfg.Body),
		Contents: []StructuredContentBlock{{
			Type:       "text",
			Summary:    "prompt body",
			Body:       body,
			Provenance: map[string]string{"source": cfg.SourcePath},
		}},
		Metadata: metadata,
	}, nil
}

func (r *runtimeAdapter) ListToolsInfo() []ToolInfo {
	return nil
}

func (r *runtimeAdapter) ListLiveProviders() []LiveProviderInfo {
	return nil
}

func (r *runtimeAdapter) GetLiveProviderDetail(providerID string) (*LiveProviderDetail, error) {
	return nil, fmt.Errorf("live provider details not available")
}

func (r *runtimeAdapter) GetLiveSessionDetail(sessionID string) (*LiveProviderSessionDetail, error) {
	return nil, fmt.Errorf("live session details not available")
}

func (r *runtimeAdapter) ListLiveSessions() []LiveProviderSessionInfo {
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
	r.rt.Tools.UpdateToolPolicy(name, agentspec.ToolPolicy{Execute: level})
}

func (r *runtimeAdapter) SetClassPolicyLive(class string, level agentspec.AgentPermissionLevel) {
	if r == nil || r.rt == nil || r.rt.Tools == nil {
		return
	}
	r.rt.Tools.UpdateClassPolicy(class, level)
}

func (r *runtimeAdapter) SaveToolPolicy(toolName string, level agentspec.AgentPermissionLevel) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	manifestPath := r.rt.Config.ManifestPath
	if manifestPath == "" {
		return fmt.Errorf("manifest path not set")
	}
	spec := r.rt.AgentWorkspace().Registration.ManifestSpec
	if spec == nil {
		spec = &config.ManifestSpec{}
	}
	if spec.Agent == nil {
		spec.Agent = &agentspec.AgentRuntimeSpec{}
	}
	if spec.Agent.ToolExecutionPolicy == nil {
		spec.Agent.ToolExecutionPolicy = make(map[string]agentspec.ToolPolicy)
	}
	spec.Agent.ToolExecutionPolicy[strings.TrimSpace(toolName)] = agentspec.ToolPolicy{Execute: level}
	if _, err := runtimesvc.SaveManifestSpecWithBackup(manifestPath, spec); err != nil {
		return err
	}
	return nil
}

func (r *runtimeAdapter) LoadSandboxManifest() (*config.ManifestSpec, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	if r.rt.AgentWorkspace().Registration != nil && r.rt.AgentWorkspace().Registration.ManifestSpec != nil {
		return r.rt.AgentWorkspace().Registration.ManifestSpec, nil
	}
	if r.rt.Config.ManifestPath == "" {
		return nil, fmt.Errorf("manifest unavailable")
	}
	docSnapshot, err := config.LoadDocument(r.rt.Config.ManifestPath)
	if err != nil {
		return nil, err
	}
	spec := &config.ManifestSpec{}
	if node, ok := docSnapshot.Document.Section("agent"); ok {
		var agentSpec agentspec.AgentRuntimeSpec
		if err := node.Decode(&agentSpec); err == nil {
			spec.Agent = &agentSpec
		}
	}
	return spec, nil
}

func (r *runtimeAdapter) SaveSandboxManifest(spec *config.ManifestSpec) (string, error) {
	if r == nil || r.rt == nil {
		return "", fmt.Errorf("runtime unavailable")
	}
	if spec == nil {
		return "", fmt.Errorf("manifest required")
	}
	path := strings.TrimSpace(r.rt.Config.ManifestPath)
	if path == "" {
		return "", fmt.Errorf("manifest path not set")
	}
	return runtimesvc.SaveManifestSpecWithBackup(path, spec)
}

func (r *runtimeAdapter) SandboxBackend() string {
	if r == nil || r.rt == nil {
		return ""
	}
	return strings.TrimSpace(r.rt.Config.SandboxBackend)
}

func (r *runtimeAdapter) ExecutionMode() config.ExecutionMode {
	if r == nil || r.rt == nil {
		return config.ExecutionModeStaged
	}
	mode := config.NormalizeExecutionMode(r.rt.WorkspaceConfig.ExecutionMode)
	if mode == config.ExecutionModeStaged && strings.TrimSpace(r.rt.WorkspaceConfig.ExecutionMode) == "" {
		return config.ExecutionModeStaged
	}
	return mode
}

func (r *runtimeAdapter) SaveSandboxBackend(backend string) (string, error) {
	if r == nil || r.rt == nil {
		return "", fmt.Errorf("runtime unavailable")
	}
	backend = strings.ToLower(strings.TrimSpace(backend))
	switch backend {
	case "gvisor", "docker":
	default:
		return "", fmt.Errorf("unsupported sandbox backend %q", backend)
	}
	r.rt.Config.SandboxBackend = backend
	r.rt.WorkspaceConfig.SandboxBackend = backend
	path := r.rt.Config.ConfigPath
	if path == "" {
		return "", fmt.Errorf("config path not set")
	}
	return config.SaveRuntimeWorkspaceConfigWithBackup(path, config.RuntimeWorkspaceConfig{
		Model:               r.rt.Config.InferenceModel,
		Provider:            r.rt.Config.InferenceProvider,
		SandboxBackend:      backend,
		ExecutionMode:       string(r.ExecutionMode()),
		Agents:              append([]string(nil), r.rt.WorkspaceConfig.Agents...),
		AllowedCapabilities: append([]config.RuntimeCapabilitySelector(nil), r.rt.WorkspaceConfig.AllowedCapabilities...),
		Nexus:               r.rt.WorkspaceConfig.Nexus,
		NodeRegistration:    r.rt.WorkspaceConfig.NodeRegistration,
		LastUpdated:         time.Now().Unix(),
	})
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

func (r *runtimeAdapter) ApproveHITL(requestID, approver string, scope policy.GrantScope, duration time.Duration) error {
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

	// Agent mode and profile from session info.
	info := r.SessionInfo()
	d.ActiveMode = info.Mode
	d.ActiveProfile = info.Profile
	d.ProfileReason = info.ProfileReason
	d.ProfileSource = info.ProfileSource
	if r.rt.AgentWorkspace().Registration != nil && r.rt.AgentWorkspace().Registration.ManifestSnapshot != nil {
		d.ManifestFingerprint = fmt.Sprintf("%x", r.rt.AgentWorkspace().Registration.ManifestSnapshot.Fingerprint)
	}
	if r.rt.Config.Workspace != "" {
		d.ProtectedPaths = config.New(r.rt.Config.Workspace).GovernanceRoots(
			r.rt.Config.ManifestPath,
			r.rt.Config.ConfigPath,
		)
	}
	if r.rt.AgentWorkspace().Registration != nil && r.rt.AgentWorkspace().Registration.ManifestSpec != nil {
		d.ManifestPolicy = manifestPolicySummary(r.rt.AgentWorkspace().Registration.ManifestSpec)
		d.DeprecationNotices = append([]string(nil), r.rt.AgentWorkspace().Registration.ManifestSpec.CompatibilityWarnings...)
	}

	return d
}

func manifestPolicySummary(spec *config.ManifestSpec) string {
	if spec == nil {
		return ""
	}
	parts := []string{}
	if spec.Policy != nil {
		policy := spec.Policy
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
	if spec.Agent != nil {
		parts = append(parts, fmt.Sprintf("tool-calling=%s", spec.Agent.ResolveToolCallingIntent()))
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
	if r == nil || r.rt == nil || r.rt.AgentWorkspace() == nil || r.rt.AgentWorkspace().ServiceManager == nil {
		return nil
	}
	snapshots := r.rt.AgentWorkspace().ServiceManager.Snapshot()
	infos := make([]ServiceInfo, 0, len(snapshots))
	for _, snapshot := range snapshots {
		status := ServiceStatusStopped
		switch snapshot.Status {
		case "running":
			status = ServiceStatusRunning
		case "stopped":
			status = ServiceStatusStopped
		case "error":
			status = ServiceStatusError
		}
		infos = append(infos, ServiceInfo{
			ID:     snapshot.ID,
			Status: status,
			Source: snapshot.Source,
			Owner:  snapshot.Owner,
			Notes:  append([]string(nil), snapshot.Notes...),
		})
	}
	return infos
}

func (r *runtimeAdapter) StopService(id string) error {
	if r == nil || r.rt == nil || r.rt.AgentWorkspace() == nil || r.rt.AgentWorkspace().ServiceManager == nil {
		return fmt.Errorf("runtime unavailable")
	}
	return r.rt.AgentWorkspace().ServiceManager.Stop(id)
}

func (r *runtimeAdapter) RestartService(ctx context.Context, id string) error {
	if r == nil || r.rt == nil || r.rt.AgentWorkspace() == nil || r.rt.AgentWorkspace().ServiceManager == nil {
		return fmt.Errorf("runtime unavailable")
	}
	return r.rt.AgentWorkspace().ServiceManager.Restart(id, ctx)
}

func (r *runtimeAdapter) RestartAllServices(ctx context.Context) error {
	if r == nil || r.rt == nil || r.rt.AgentWorkspace() == nil || r.rt.AgentWorkspace().ServiceManager == nil {
		return fmt.Errorf("runtime unavailable")
	}
	if err := r.rt.AgentWorkspace().ServiceManager.StopAll(); err != nil {
		return fmt.Errorf("stop all: %w", err)
	}
	return r.rt.AgentWorkspace().ServiceManager.StartAll(ctx)
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
		return nil, errors.New("runtime not initialized")
	}
	return nil, errors.New("QueryIntentGaps not implemented")
}

func (r *runtimeAdapter) QueryTensions(scope string) ([]TensionInfo, error) {
	if r == nil || r.rt == nil {
		return nil, errors.New("runtime not initialized")
	}
	return nil, errors.New("QueryTensions not implemented")
}

func (r *runtimeAdapter) LoadLivePlan(workflowID string) (*LivePlanInfo, error) {
	_ = workflowID
	return nil, errors.New("LoadLivePlan not implemented")
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
	return info, nil
}

func (r *runtimeAdapter) GetLatestTrace() (TraceInfo, error) {
	return TraceInfo{}, nil
}

// ActiveWorkflowID satisfies RuntimeAdapter.
func (r *runtimeAdapter) ActiveWorkflowID() string { return r.activeWorkflowID() }

func (r *runtimeAdapter) ResumeSession(ctx context.Context, workflowID string) error {
	if r == nil || r.rt == nil {
		return nil
	}
	_, err := r.rt.ResumeSession(ctx, workflowID)
	return err
}

func (r *runtimeAdapter) ResolveInteractionFrame(ctx context.Context, taskID, frameID, choice, freetext string) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	return r.rt.ResolveInteractionFrame(ctx, taskID, frameID, choice, freetext)
}

func (r *runtimeAdapter) BuildDoctorReport(ctx context.Context) DoctorReport {
	if r == nil || r.rt == nil {
		return DoctorReport{}
	}
	return runtimesvc.BuildDoctorReport(ctx, r.rt.Config, r.rt.Secrets())
}

func (r *runtimeAdapter) ReloadWorkspace(ctx context.Context, workspace string) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	newRT, err := runtimesvc.ReloadRuntimeForWorkspace(ctx, r.rt, workspace)
	if err != nil {
		return err
	}
	r.rt = newRT
	return nil
}

func (r *runtimeAdapter) InitializeWorkspaceFromTemplates(overwrite bool) error {
	if r == nil || r.rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	return runtimesvc.InitializeWorkspaceFromTemplates(r.rt.Config, overwrite)
}

func (r *runtimeAdapter) activeWorkflowID() string {
	return ""
}
