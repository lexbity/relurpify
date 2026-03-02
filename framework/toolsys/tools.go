package toolsys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PermissionAware allows tools to receive the permission manager for fine-grained
// runtime checks (e.g. verifying file paths against allowlists).
type PermissionAware interface {
	SetPermissionManager(manager *PermissionManager, agentID string)
}

// AgentSpecAware allows tools to consume the agent manifest runtime spec for
// additional policy enforcement (e.g. bash/file matrices).
type AgentSpecAware interface {
	SetAgentSpec(spec *AgentRuntimeSpec, agentID string)
}

// ToolRegistry maintains tools and ensures metadata lookups are fast. Agents
// typically keep a shared registry instance so dynamic planners can discover
// the available affordances at runtime.
type ToolRegistry struct {
	mu                sync.RWMutex
	tools             map[string]Tool
	permissionManager *PermissionManager
	registeredAgentID string
	agentSpec         *AgentRuntimeSpec
	allowedTools      map[string]struct{}
	toolPolicies      map[string]ToolPolicy
	globalPolicies    map[string]AgentPermissionLevel
	telemetry         Telemetry
}

// NewToolRegistry builds a registry instance.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:        make(map[string]Tool),
		toolPolicies: make(map[string]ToolPolicy),
	}
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[tool.Name()]; exists {
		return fmt.Errorf("tool %s already registered", tool.Name())
	}
	if r.allowedTools != nil {
		if _, ok := r.allowedTools[tool.Name()]; !ok {
			return nil
		}
	}
	// If we already have a manager, inject it immediately
	if r.permissionManager != nil {
		if aware, ok := tool.(PermissionAware); ok {
			aware.SetPermissionManager(r.permissionManager, r.registeredAgentID)
		}
	}
	if r.agentSpec != nil {
		if aware, ok := tool.(AgentSpecAware); ok {
			aware.SetAgentSpec(r.agentSpec, r.registeredAgentID)
		}
	}
	r.tools[tool.Name()] = r.wrapTool(tool)
	return nil
}

// Get fetches a tool by name.
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// All returns all registered tools.
func (r *ToolRegistry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		res = append(res, t)
	}
	return res
}

// CloneFiltered returns a new registry that contains the same tool wrappers and
// registry policies, but only keeps tools that match the predicate. This is
// useful when a higher-level agent wants to expose a subset of tools (e.g.,
// mode-specific scopes) without losing telemetry/permission wiring.
func (r *ToolRegistry) CloneFiltered(keep func(Tool) bool) *ToolRegistry {
	if r == nil {
		return NewToolRegistry()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	clone := &ToolRegistry{
		tools:             make(map[string]Tool),
		permissionManager: r.permissionManager,
		registeredAgentID: r.registeredAgentID,
		agentSpec:         r.agentSpec,
		allowedTools:      cloneAllowedTools(r.allowedTools),
		telemetry:         r.telemetry,
		toolPolicies:      make(map[string]ToolPolicy, len(r.toolPolicies)),
		globalPolicies:    cloneGlobalPolicies(r.globalPolicies),
	}
	for name, pol := range r.toolPolicies {
		clone.toolPolicies[name] = pol
	}
	for name, tool := range r.tools {
		if keep != nil && !keep(tool) {
			continue
		}
		clone.tools[name] = cloneTool(tool)
	}
	return clone
}

func cloneTool(tool Tool) Tool {
	if tool == nil {
		return nil
	}
	if t, ok := tool.(*instrumentedTool); ok {
		// Create a fresh wrapper so per-mode registries don't mutate each other's
		// telemetry/permission pointers.
		return &instrumentedTool{
			Tool:           t.Tool,
			manager:        t.manager,
			agentID:        t.agentID,
			telemetry:      t.telemetry,
			policy:         t.policy,
			hasPolicy:      t.hasPolicy,
			globalPolicies: t.globalPolicies,
		}
	}
	return tool
}

// UsePermissionManager enables default-deny enforcement for all tools.
func (r *ToolRegistry) UsePermissionManager(agentID string, manager *PermissionManager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permissionManager = manager
	r.registeredAgentID = agentID
	for name, tool := range r.tools {
		var inner Tool = tool
		if instrumented, ok := tool.(*instrumentedTool); ok {
			inner = instrumented.Tool
			instrumented.manager = manager
			instrumented.agentID = agentID
		}
		if aware, ok := inner.(PermissionAware); ok {
			aware.SetPermissionManager(manager, agentID)
		}
		if aware, ok := inner.(AgentSpecAware); ok && r.agentSpec != nil {
			aware.SetAgentSpec(r.agentSpec, agentID)
		}
		r.tools[name] = r.wrapTool(inner)
	}
}

// UseAgentSpec wires per-tool policies and other manifest-driven knobs into
// the registry and any tools that opt in.
func (r *ToolRegistry) UseAgentSpec(agentID string, spec *AgentRuntimeSpec) {
	if spec == nil {
		return
	}
	r.mu.Lock()
	r.registeredAgentID = agentID
	r.agentSpec = spec
	r.mu.Unlock()

	if spec.AllowedTools != nil {
		r.setAllowedTools(spec.AllowedTools, true)
	}
	r.setToolPolicies(spec.ToolExecutionPolicy)
	r.setTagPolicies(spec.GlobalPolicies)

	r.mu.Lock()
	for name, tool := range r.tools {
		var inner Tool = tool
		if instrumented, ok := tool.(*instrumentedTool); ok {
			inner = instrumented.Tool
		}
		if aware, ok := inner.(AgentSpecAware); ok {
			aware.SetAgentSpec(spec, agentID)
		}
		r.tools[name] = r.wrapTool(inner)
	}
	r.mu.Unlock()
}

func (r *ToolRegistry) setAllowedTools(allowed []string, configured bool) {
	if r == nil {
		return
	}
	if !configured {
		return
	}
	if len(allowed) == 0 {
		r.mu.Lock()
		r.allowedTools = map[string]struct{}{}
		r.tools = make(map[string]Tool)
		r.mu.Unlock()
		return
	}
	set := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	r.mu.Lock()
	r.allowedTools = set
	r.mu.Unlock()
	r.RestrictTo(allowed)
}

func (r *ToolRegistry) setToolPolicies(policies map[string]ToolPolicy) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.toolPolicies = make(map[string]ToolPolicy, len(policies))
	for name, policy := range policies {
		r.toolPolicies[name] = policy
	}
	for name, tool := range r.tools {
		var inner Tool = tool
		if instrumented, ok := tool.(*instrumentedTool); ok {
			inner = instrumented.Tool
			instrumented.policy = r.toolPolicies[inner.Name()]
			instrumented.hasPolicy = r.agentSpec != nil
		}
		r.tools[name] = r.wrapTool(inner)
	}
	r.mu.Unlock()
}

func cloneAllowedTools(input map[string]struct{}) map[string]struct{} {
	if input == nil {
		return nil
	}
	out := make(map[string]struct{}, len(input))
	for name := range input {
		out[name] = struct{}{}
	}
	return out
}

func cloneGlobalPolicies(input map[string]AgentPermissionLevel) map[string]AgentPermissionLevel {
	if input == nil {
		return nil
	}
	out := make(map[string]AgentPermissionLevel, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

// setTagPolicies stores global tag-based policies and re-wraps all tools.
func (r *ToolRegistry) setTagPolicies(policies map[string]AgentPermissionLevel) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.globalPolicies = cloneGlobalPolicies(policies)
	for name, tool := range r.tools {
		var inner Tool = tool
		if instrumented, ok := tool.(*instrumentedTool); ok {
			inner = instrumented.Tool
		}
		r.tools[name] = r.wrapTool(inner)
	}
	r.mu.Unlock()
}

// effectiveTagPolicy resolves the most restrictive permission level from
// all matching tags. Order: deny > ask > allow > "".
func effectiveTagPolicy(tags []string, policies map[string]AgentPermissionLevel) AgentPermissionLevel {
	var result AgentPermissionLevel
	for _, tag := range tags {
		level, ok := policies[tag]
		if !ok {
			continue
		}
		switch {
		case level == AgentPermissionDeny:
			return AgentPermissionDeny
		case level == AgentPermissionAsk && result != AgentPermissionDeny:
			result = AgentPermissionAsk
		case level == AgentPermissionAllow && result == "":
			result = AgentPermissionAllow
		}
	}
	return result
}

// UseTelemetry wires a telemetry sink for all tool executions.
func (r *ToolRegistry) UseTelemetry(telemetry Telemetry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.telemetry = telemetry
	for name, tool := range r.tools {
		var inner Tool = tool
		if instrumented, ok := tool.(*instrumentedTool); ok {
			inner = instrumented.Tool
			instrumented.telemetry = telemetry
		}
		r.tools[name] = r.wrapTool(inner)
	}
}

// RestrictTo removes tools not present in the allowed set.
func (r *ToolRegistry) RestrictTo(allowed []string) {
	if len(allowed) == 0 {
		return
	}
	set := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for name := range r.tools {
		if _, ok := set[name]; !ok {
			delete(r.tools, name)
		}
	}
}

// wrapTool decorates a tool with the instrumentation wrapper so permissions
// and telemetry remain consistent regardless of who calls the tool.
func (r *ToolRegistry) wrapTool(tool Tool) Tool {
	if tool == nil {
		return nil
	}
	if existing, ok := tool.(*instrumentedTool); ok {
		existing.manager = r.permissionManager
		existing.agentID = r.registeredAgentID
		existing.telemetry = r.telemetry
		existing.policy = r.toolPolicies[existing.Tool.Name()]
		existing.hasPolicy = r.agentSpec != nil
		existing.globalPolicies = r.globalPolicies
		return existing
	}
	return &instrumentedTool{
		Tool:           tool,
		manager:        r.permissionManager,
		agentID:        r.registeredAgentID,
		telemetry:      r.telemetry,
		policy:         r.toolPolicies[tool.Name()],
		hasPolicy:      r.agentSpec != nil,
		globalPolicies: r.globalPolicies,
	}
}

type instrumentedTool struct {
	Tool
	manager        *PermissionManager
	agentID        string
	telemetry      Telemetry
	policy         ToolPolicy
	hasPolicy      bool
	globalPolicies map[string]AgentPermissionLevel
}

// Execute authorizes the wrapped tool before delegating to the original
// implementation to ensure permission checks happen even for direct callers.
func (t *instrumentedTool) Execute(ctx context.Context, state *Context, args map[string]interface{}) (*ToolResult, error) {
	if t.hasPolicy {
		switch t.policy.Execute {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("tool %s blocked: execution denied by policy", t.Tool.Name())
		case AgentPermissionAsk:
			if t.manager == nil {
				return nil, fmt.Errorf("tool %s blocked: approval required but permission manager missing", t.Tool.Name())
			}
			if err := t.manager.RequireApproval(ctx, t.agentID, PermissionDescriptor{
				Type:         PermissionTypeHITL,
				Action:       fmt.Sprintf("tool_exec:%s", t.Tool.Name()),
				Resource:     t.agentID,
				RequiresHITL: true,
			}, "tool execution approval", GrantScopeOneTime, RiskLevelMedium, 0); err != nil {
				return nil, err
			}
		}
	}
	if len(t.globalPolicies) > 0 {
		effective := effectiveTagPolicy(t.Tool.Tags(), t.globalPolicies)
		switch effective {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("tool %s blocked: execution denied by tag policy", t.Tool.Name())
		case AgentPermissionAsk:
			if t.manager == nil {
				return nil, fmt.Errorf("tool %s blocked: approval required but permission manager missing", t.Tool.Name())
			}
			if err := t.manager.RequireApproval(ctx, t.agentID, PermissionDescriptor{
				Type:         PermissionTypeHITL,
				Action:       fmt.Sprintf("tool_exec:%s", t.Tool.Name()),
				Resource:     t.agentID,
				RequiresHITL: true,
			}, "tag policy approval", GrantScopeOneTime, RiskLevelMedium, 0); err != nil {
				return nil, err
			}
		}
	}
	if t.manager != nil {
		if err := t.manager.AuthorizeTool(ctx, t.agentID, t.Tool, args); err != nil {
			var denied *PermissionDeniedError
			if errors.As(err, &denied) {
				return nil, fmt.Errorf("tool %s blocked: %w", t.Tool.Name(), err)
			}
			return nil, err
		}
	}
	if t.telemetry != nil {
		t.telemetry.Emit(Event{
			Type:      EventToolCall,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("tool %s invoked", t.Tool.Name()),
			Metadata: map[string]interface{}{
				"tool":     t.Tool.Name(),
				"agent_id": t.agentID,
				"args":     summarizeArgs(args),
			},
		})
	}
	result, err := t.Tool.Execute(ctx, state, args)
	if err != nil {
		var denied *PermissionDeniedError
		if errors.As(err, &denied) {
			err = fmt.Errorf("tool %s blocked: %w", t.Tool.Name(), err)
		}
	}
	if t.telemetry != nil {
		metadata := map[string]interface{}{
			"tool":     t.Tool.Name(),
			"agent_id": t.agentID,
		}
		if result != nil {
			metadata["success"] = result.Success
			if result.Error != "" {
				metadata["tool_error"] = result.Error
			}
		}
		if err != nil {
			metadata["error"] = err.Error()
		}
		t.telemetry.Emit(Event{
			Type:      EventToolResult,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("tool %s completed", t.Tool.Name()),
			Metadata:  metadata,
		})
	}
	return result, err
}

func summarizeArgs(args map[string]interface{}) interface{} {
	if len(args) == 0 {
		return nil
	}
	return fmt.Sprintf("%v", args)
}
