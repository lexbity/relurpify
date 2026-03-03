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

	runtimesvc "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/core"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
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
		Mode:      "",
		MaxTokens: 100000,
	}
	if r == nil || r.rt == nil {
		return info
	}
	cfg := r.rt.Config
	info.Workspace = cfg.Workspace
	info.Model = cfg.OllamaModel
	info.Agent = cfg.AgentLabel()
	info.Mode = string(core.AgentModePrimary)

	if r.rt.Registration != nil && r.rt.Registration.Manifest != nil {
		manifest := r.rt.Registration.Manifest
		info.Agent = manifest.Metadata.Name
		if manifest.Spec.Agent != nil {
			if manifest.Spec.Agent.Model.Name != "" {
				info.Model = manifest.Spec.Agent.Model.Name
			}
			if manifest.Spec.Agent.Mode != "" {
				info.Mode = string(manifest.Spec.Agent.Mode)
			}
			if manifest.Spec.Agent.Context.MaxTokens > 0 {
				info.MaxTokens = manifest.Spec.Agent.Context.MaxTokens
			}
		}
	}
	return info
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
