// Package capability implements the central capability registry for the agent framework.
package capability

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
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

// CapabilityRegistry maintains framework-owned capability descriptors plus the
// narrowed local-tool runtime and temporary model-bridge shims used during the
// migration away from generic tool-shaped invocation.
type CapabilityRegistry struct {
	mu                  sync.RWMutex
	tools               map[string]Tool
	capabilities        map[string]core.CapabilityDescriptor
	entries             map[string]*capabilityEntry
	permissionManager   *PermissionManager
	registeredAgentID   string
	agentSpec           *AgentRuntimeSpec
	allowedCapabilities []core.CapabilitySelector
	toolPolicies        map[string]ToolPolicy
	capabilityPolicies  []core.CapabilityPolicy
	exposurePolicies    []core.CapabilityExposurePolicy
	globalPolicies      map[string]AgentPermissionLevel
	telemetry           Telemetry
	safety              *runtimeSafetyController
	policyEngine        authorization.PolicyEngine
	nodeProviders       map[string]core.NodeProvider
}

// NewCapabilityRegistry builds a capability registry instance.
func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{
		tools:        make(map[string]Tool),
		capabilities: make(map[string]core.CapabilityDescriptor),
		entries:      make(map[string]*capabilityEntry),
		toolPolicies: make(map[string]ToolPolicy),
		safety:       newRuntimeSafetyController(),
	}
}

type capabilityEntry struct {
	descriptor core.CapabilityDescriptor
	handler    core.CapabilityHandler
	legacyTool Tool
	providerID string
	sessionID  string
}

// RegisterCapability adds a non-tool capability descriptor to the shared registry.
func (r *CapabilityRegistry) RegisterCapability(descriptor core.CapabilityDescriptor) error {
	if r == nil {
		return fmt.Errorf("registry unavailable")
	}
	descriptor = core.NormalizeCapabilityDescriptor(descriptor)
	if descriptor.ID == "" {
		return fmt.Errorf("capability id required")
	}
	if err := validateCoordinationDescriptor(descriptor); err != nil {
		return err
	}
	r.mu.Lock()
	if _, ok := r.capabilities[descriptor.ID]; ok {
		r.mu.Unlock()
		return fmt.Errorf("capability %s already registered", descriptor.ID)
	}
	if !matchesAnyCapabilitySelector(r.allowedCapabilities, descriptor) {
		r.mu.Unlock()
		return nil
	}
	r.capabilities[descriptor.ID] = descriptor
	r.entries[descriptor.ID] = &capabilityEntry{
		descriptor: descriptor,
		providerID: descriptor.Source.ProviderID,
		sessionID:  descriptor.Source.SessionID,
	}
	telemetry := r.telemetry
	exposure := r.effectiveExposureLocked(descriptor)
	r.mu.Unlock()
	emitCapabilitySecurityEvent(telemetry, "capability_admitted", descriptor, exposure, "")
	return nil
}

// RegisterInvocableCapability registers a runtime-backed invocable capability.
func (r *CapabilityRegistry) RegisterInvocableCapability(handler core.InvocableCapabilityHandler) error {
	if r == nil {
		return fmt.Errorf("registry unavailable")
	}
	if handler == nil {
		return fmt.Errorf("capability handler required")
	}
	desc := core.NormalizeCapabilityDescriptor(handler.Descriptor(context.Background(), nil))
	if desc.ID == "" {
		return fmt.Errorf("capability id required")
	}
	if err := validateCoordinationDescriptor(desc); err != nil {
		return err
	}
	r.mu.Lock()
	if !matchesAnyCapabilitySelector(r.allowedCapabilities, desc) {
		r.mu.Unlock()
		return nil
	}
	if _, ok := r.entries[desc.ID]; ok {
		r.mu.Unlock()
		return fmt.Errorf("capability %s already registered", desc.ID)
	}
	r.capabilities[desc.ID] = desc
	r.entries[desc.ID] = &capabilityEntry{
		descriptor: desc,
		handler:    r.wrapCapabilityHandler(handler),
		providerID: desc.Source.ProviderID,
		sessionID:  desc.Source.SessionID,
	}
	telemetry := r.telemetry
	exposure := r.effectiveExposureLocked(desc)
	r.mu.Unlock()
	emitCapabilitySecurityEvent(telemetry, "capability_admitted", desc, exposure, "")
	return nil
}

// RegisterPromptCapability registers a runtime-backed prompt capability.
func (r *CapabilityRegistry) RegisterPromptCapability(handler core.PromptCapabilityHandler) error {
	if r == nil {
		return fmt.Errorf("registry unavailable")
	}
	if handler == nil {
		return fmt.Errorf("prompt handler required")
	}
	desc := core.NormalizeCapabilityDescriptor(handler.Descriptor(context.Background(), nil))
	if desc.ID == "" {
		return fmt.Errorf("capability id required")
	}
	if err := validateCoordinationDescriptor(desc); err != nil {
		return err
	}
	r.mu.Lock()
	if !matchesAnyCapabilitySelector(r.allowedCapabilities, desc) {
		r.mu.Unlock()
		return nil
	}
	if _, ok := r.entries[desc.ID]; ok {
		r.mu.Unlock()
		return fmt.Errorf("capability %s already registered", desc.ID)
	}
	r.capabilities[desc.ID] = desc
	r.entries[desc.ID] = &capabilityEntry{
		descriptor: desc,
		handler:    r.wrapCapabilityHandler(handler),
		providerID: desc.Source.ProviderID,
		sessionID:  desc.Source.SessionID,
	}
	telemetry := r.telemetry
	exposure := r.effectiveExposureLocked(desc)
	r.mu.Unlock()
	emitCapabilitySecurityEvent(telemetry, "capability_admitted", desc, exposure, "")
	return nil
}

// RegisterResourceCapability registers a runtime-backed resource capability.
func (r *CapabilityRegistry) RegisterResourceCapability(handler core.ResourceCapabilityHandler) error {
	if r == nil {
		return fmt.Errorf("registry unavailable")
	}
	if handler == nil {
		return fmt.Errorf("resource handler required")
	}
	desc := core.NormalizeCapabilityDescriptor(handler.Descriptor(context.Background(), nil))
	if desc.ID == "" {
		return fmt.Errorf("capability id required")
	}
	if err := validateCoordinationDescriptor(desc); err != nil {
		return err
	}
	r.mu.Lock()
	if !matchesAnyCapabilitySelector(r.allowedCapabilities, desc) {
		r.mu.Unlock()
		return nil
	}
	if _, ok := r.entries[desc.ID]; ok {
		r.mu.Unlock()
		return fmt.Errorf("capability %s already registered", desc.ID)
	}
	r.capabilities[desc.ID] = desc
	r.entries[desc.ID] = &capabilityEntry{
		descriptor: desc,
		handler:    r.wrapCapabilityHandler(handler),
		providerID: desc.Source.ProviderID,
		sessionID:  desc.Source.SessionID,
	}
	telemetry := r.telemetry
	exposure := r.effectiveExposureLocked(desc)
	r.mu.Unlock()
	emitCapabilitySecurityEvent(telemetry, "capability_admitted", desc, exposure, "")
	return nil
}

// ProviderCapabilityRegistrar returns a registrar that normalizes provider-
// backed capabilities against provider metadata and agent policy before
// registration.
func (r *CapabilityRegistry) ProviderCapabilityRegistrar(provider core.ProviderDescriptor, policy core.ProviderPolicy) (core.CapabilityRegistrar, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	if err := provider.Validate(); err != nil {
		return nil, err
	}
	if err := core.ValidateProviderPolicy(policy); err != nil {
		return nil, err
	}
	return providerCapabilityRegistrar{
		registry: r,
		provider: provider,
		policy:   policy,
	}, nil
}

type providerCapabilityRegistrar struct {
	registry *CapabilityRegistry
	provider core.ProviderDescriptor
	policy   core.ProviderPolicy
}

func (r providerCapabilityRegistrar) RegisterCapability(descriptor core.CapabilityDescriptor) error {
	normalized, err := core.NormalizeProviderCapability(descriptor, r.provider, r.policy)
	if err != nil {
		return err
	}
	return r.registry.RegisterCapability(normalized)
}

// Register adds a tool to the registry.
func (r *CapabilityRegistry) Register(tool Tool) error {
	return r.RegisterLegacyTool(tool)
}

// RegisterLegacyTool adds a legacy core.Tool implementation to the registry by
// adapting it into a tool-kind capability entry.
func (r *CapabilityRegistry) RegisterLegacyTool(tool Tool) error {
	desc := core.NormalizeCapabilityDescriptor(core.ToolDescriptor(context.Background(), nil, tool))
	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyLocalTool {
		return fmt.Errorf("legacy tool registration only supports local-tool runtime family; %s is %s", desc.ID, desc.RuntimeFamily)
	}
	r.mu.Lock()
	if _, exists := r.tools[tool.Name()]; exists {
		r.mu.Unlock()
		return fmt.Errorf("tool %s already registered", tool.Name())
	}
	if _, exists := r.capabilities[desc.ID]; exists {
		r.mu.Unlock()
		return fmt.Errorf("capability %s already registered", desc.ID)
	}
	if !matchesAnyCapabilitySelector(r.allowedCapabilities, desc) {
		r.mu.Unlock()
		return nil
	}
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
	wrapped := r.wrapTool(tool)
	adapter := legacyToolHandler{tool: wrapped}
	desc = core.NormalizeCapabilityDescriptor(adapter.Descriptor(context.Background(), nil))
	r.tools[tool.Name()] = wrapped
	r.capabilities[desc.ID] = desc
	r.entries[desc.ID] = &capabilityEntry{
		descriptor: desc,
		handler:    adapter,
		legacyTool: wrapped,
		providerID: desc.Source.ProviderID,
		sessionID:  desc.Source.SessionID,
	}
	telemetry := r.telemetry
	exposure := r.effectiveExposureLocked(desc)
	r.mu.Unlock()
	emitCapabilitySecurityEvent(telemetry, "capability_admitted", desc, exposure, "")
	return nil
}

// Get fetches a tool by name.
func (r *CapabilityRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// HasCapability reports whether a capability is registered by ID or public name.
func (r *CapabilityRegistry) HasCapability(idOrName string) bool {
	_, ok := r.GetCapability(idOrName)
	return ok
}

// All returns tools exposed as callable to the active agent.
func (r *CapabilityRegistry) All() []Tool {
	return r.CallableTools()
}

// CallableTools returns only tools exposed as callable to agents.
func (r *CapabilityRegistry) CallableTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		desc := core.ToolDescriptor(context.Background(), nil, unwrapTool(t))
		if desc.RuntimeFamily != core.CapabilityRuntimeFamilyLocalTool {
			continue
		}
		if r.effectiveExposureLocked(desc) != core.CapabilityExposureCallable {
			continue
		}
		res = append(res, t)
	}
	return res
}

// InspectableTools returns tools visible for operator inspection.
func (r *CapabilityRegistry) InspectableTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		desc := core.ToolDescriptor(context.Background(), nil, unwrapTool(t))
		if desc.RuntimeFamily != core.CapabilityRuntimeFamilyLocalTool {
			continue
		}
		if r.effectiveExposureLocked(desc) == core.CapabilityExposureHidden {
			continue
		}
		res = append(res, t)
	}
	return res
}

// ModelCallableTools returns LLM-facing callable tools. Local tools are
// returned directly, while callable provider/Relurpic capabilities are exposed
// through compatibility shims until the model interface becomes capability-native.
func (r *CapabilityRegistry) ModelCallableTools() []Tool {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]Tool, 0, len(r.entries))
	for _, t := range r.tools {
		desc := core.ToolDescriptor(context.Background(), nil, unwrapTool(t))
		if desc.RuntimeFamily != core.CapabilityRuntimeFamilyLocalTool {
			continue
		}
		if r.effectiveExposureLocked(desc) != core.CapabilityExposureCallable {
			continue
		}
		res = append(res, t)
	}
	for _, entry := range r.entries {
		if entry == nil || entry.handler == nil {
			continue
		}
		if entry.descriptor.RuntimeFamily == core.CapabilityRuntimeFamilyLocalTool {
			continue
		}
		if _, ok := entry.handler.(core.InvocableCapabilityHandler); !ok {
			continue
		}
		if r.effectiveExposureLocked(entry.descriptor) != core.CapabilityExposureCallable {
			continue
		}
		res = append(res, capabilityToolShim{registry: r, descriptor: entry.descriptor})
	}
	return res
}

// GetModelTool resolves an LLM-facing callable tool by name. This includes
// compatibility shims for non-local callable capabilities.
func (r *CapabilityRegistry) GetModelTool(name string) (Tool, bool) {
	for _, tool := range r.ModelCallableTools() {
		if strings.EqualFold(strings.TrimSpace(tool.Name()), strings.TrimSpace(name)) {
			return tool, true
		}
	}
	return nil, false
}

// GetCapability resolves a tool by either capability ID or public name.
func (r *CapabilityRegistry) GetCapability(idOrName string) (CapabilityDescriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if capability, ok := r.capabilities[idOrName]; ok {
		return capability, true
	}
	for _, capability := range r.capabilities {
		if capability.ID == idOrName || capability.Name == idOrName {
			return capability, true
		}
	}
	return CapabilityDescriptor{}, false
}

// GetCoordinationTarget returns a non-hidden capability that is explicitly
// marked as a coordination target.
func (r *CapabilityRegistry) GetCoordinationTarget(idOrName string) (CapabilityDescriptor, bool) {
	if r == nil {
		return CapabilityDescriptor{}, false
	}
	desc, ok := r.GetCapability(idOrName)
	if !ok || desc.Coordination == nil || !desc.Coordination.Target {
		return CapabilityDescriptor{}, false
	}
	if r.EffectiveExposure(desc) == core.CapabilityExposureHidden {
		return CapabilityDescriptor{}, false
	}
	return desc, true
}

// AllCapabilities returns non-hidden capability descriptors.
func (r *CapabilityRegistry) AllCapabilities() []CapabilityDescriptor {
	return r.InspectableCapabilities()
}

// CallableCapabilities returns descriptors exposed as callable to agents.
func (r *CapabilityRegistry) CallableCapabilities() []CapabilityDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]CapabilityDescriptor, 0, len(r.capabilities))
	for _, capability := range r.capabilities {
		if r.effectiveExposureLocked(capability) != core.CapabilityExposureCallable {
			continue
		}
		res = append(res, capability)
	}
	return res
}

// CoordinationTargets returns admitted, non-hidden coordination target
// capabilities that match all provided selectors.
func (r *CapabilityRegistry) CoordinationTargets(selectors ...core.CapabilitySelector) []CapabilityDescriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CapabilityDescriptor, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry == nil || entry.descriptor.Coordination == nil || !entry.descriptor.Coordination.Target {
			continue
		}
		if r.effectiveExposureLocked(entry.descriptor) == core.CapabilityExposureHidden {
			continue
		}
		matched := true
		for _, selector := range selectors {
			if !core.SelectorMatchesDescriptor(selector, entry.descriptor) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, entry.descriptor)
		}
	}
	return out
}

// InspectableCapabilities returns non-hidden capability descriptors.
func (r *CapabilityRegistry) InspectableCapabilities() []CapabilityDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]CapabilityDescriptor, 0, len(r.capabilities))
	for _, capability := range r.capabilities {
		if r.effectiveExposureLocked(capability) == core.CapabilityExposureHidden {
			continue
		}
		res = append(res, capability)
	}
	return res
}

// CloneFiltered returns a new registry that contains the same tool wrappers and
// registry policies, but only keeps tools that match the predicate.
func (r *CapabilityRegistry) CloneFiltered(keep func(Tool) bool) *CapabilityRegistry {
	if r == nil {
		return NewCapabilityRegistry()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	clone := &CapabilityRegistry{
		tools:               make(map[string]Tool),
		capabilities:        make(map[string]core.CapabilityDescriptor),
		entries:             make(map[string]*capabilityEntry),
		permissionManager:   r.permissionManager,
		registeredAgentID:   r.registeredAgentID,
		agentSpec:           r.agentSpec,
		allowedCapabilities: cloneCapabilitySelectors(r.allowedCapabilities),
		telemetry:           r.telemetry,
		safety:              r.safety,
		toolPolicies:        make(map[string]ToolPolicy, len(r.toolPolicies)),
		capabilityPolicies:  append([]core.CapabilityPolicy{}, r.capabilityPolicies...),
		exposurePolicies:    append([]core.CapabilityExposurePolicy{}, r.exposurePolicies...),
		globalPolicies:      cloneGlobalPolicies(r.globalPolicies),
	}
	for name, pol := range r.toolPolicies {
		clone.toolPolicies[name] = pol
	}
	for id, capability := range r.capabilities {
		if capability.Kind == core.CapabilityKindTool {
			continue
		}
		clone.capabilities[id] = capability
		if entry, ok := r.entries[id]; ok {
			clonedEntry := *entry
			clone.entries[id] = &clonedEntry
		}
	}
	for name, tool := range r.tools {
		if keep != nil && !keep(tool) {
			continue
		}
		clonedTool := cloneTool(tool)
		clone.tools[name] = clonedTool
		desc := core.NormalizeCapabilityDescriptor(core.ToolDescriptor(context.Background(), nil, unwrapTool(clonedTool)))
		clone.capabilities[desc.ID] = desc
		if entry, ok := r.entries[desc.ID]; ok {
			clonedEntry := *entry
			clonedEntry.descriptor = desc
			clonedEntry.legacyTool = clonedTool
			if clonedEntry.handler == nil {
				clonedEntry.handler = legacyToolHandler{tool: clonedTool}
			}
			clone.entries[desc.ID] = &clonedEntry
			continue
		}
		clone.entries[desc.ID] = &capabilityEntry{
			descriptor: desc,
			handler:    legacyToolHandler{tool: clonedTool},
			legacyTool: clonedTool,
			providerID: desc.Source.ProviderID,
			sessionID:  desc.Source.SessionID,
		}
	}
	return clone
}

func cloneTool(tool Tool) Tool {
	if tool == nil {
		return nil
	}
	if t, ok := tool.(*instrumentedTool); ok {
		return &instrumentedTool{
			Tool:               t.Tool,
			manager:            t.manager,
			agentID:            t.agentID,
			telemetry:          t.telemetry,
			policy:             t.policy,
			capabilityPolicies: append([]core.CapabilityPolicy{}, t.capabilityPolicies...),
			hasPolicy:          t.hasPolicy,
			globalPolicies:     t.globalPolicies,
			safety:             t.safety,
		}
	}
	return tool
}

// InvokeCapability executes an invocable capability by capability ID or public name.
func (r *CapabilityRegistry) InvokeCapability(ctx context.Context, state *Context, idOrName string, args map[string]interface{}) (*ToolResult, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	entry, err := r.capabilityEntry(idOrName)
	if err != nil {
		return nil, err
	}
	invocable, ok := entry.handler.(core.InvocableCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability %s is not invocable", entry.descriptor.ID)
	}
	if aware, ok := entry.handler.(core.AvailabilityAwareCapabilityHandler); ok {
		if availability := aware.Availability(ctx, state); !availability.Available {
			reason := strings.TrimSpace(availability.Reason)
			if reason == "" {
				reason = "capability unavailable"
			}
			return nil, fmt.Errorf("capability %s blocked: %s", entry.descriptor.ID, reason)
		}
	}
	r.mu.RLock()
	policyEngine := r.policyEngine
	agentID := r.registeredAgentID
	r.mu.RUnlock()
	if policyEngine != nil {
		decision, err := policyEngine.Evaluate(ctx, core.PolicyRequest{
			Target:         core.PolicyTargetCapability,
			Actor:          core.EventActor{Kind: "agent", ID: agentID},
			CapabilityID:   entry.descriptor.ID,
			CapabilityName: entry.descriptor.Name,
			CapabilityKind: entry.descriptor.Kind,
			RuntimeFamily:  entry.descriptor.RuntimeFamily,
			ProviderKind:   providerKindForDescriptor(entry.descriptor),
			TrustClass:     entry.descriptor.TrustClass,
			RiskClasses:    append([]core.RiskClass{}, entry.descriptor.RiskClasses...),
			EffectClasses:  append([]core.EffectClass{}, entry.descriptor.EffectClasses...),
		})
		if err != nil {
			return nil, err
		}
		switch decision.Effect {
		case "deny":
			return nil, fmt.Errorf("capability %s blocked: %s", entry.descriptor.ID, decision.Reason)
		case "require_approval":
			if r.permissionManager == nil {
				return nil, fmt.Errorf("capability %s blocked: approval required but permission manager unavailable", entry.descriptor.ID)
			}
			if err := r.permissionManager.RequireApproval(ctx, agentID, core.PermissionDescriptor{
				Type:         core.PermissionTypeCapability,
				Action:       "capability:" + entry.descriptor.Name,
				Resource:     entry.descriptor.ID,
				RequiresHITL: true,
			}, "capability policy approval", authorization.GrantScopeSession, authorization.RiskLevelMedium, 0); err != nil {
				return nil, err
			}
		}
	}
	return invocable.Invoke(ctx, state, args)
}

// RenderPrompt executes a runtime-backed prompt capability by capability ID or public name.
func (r *CapabilityRegistry) RenderPrompt(ctx context.Context, state *Context, idOrName string, args map[string]interface{}) (*core.PromptRenderResult, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	entry, err := r.capabilityEntry(idOrName)
	if err != nil {
		return nil, err
	}
	promptHandler, ok := entry.handler.(core.PromptCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability %s is not a prompt handler", entry.descriptor.ID)
	}
	if aware, ok := entry.handler.(core.AvailabilityAwareCapabilityHandler); ok {
		if availability := aware.Availability(ctx, state); !availability.Available {
			reason := strings.TrimSpace(availability.Reason)
			if reason == "" {
				reason = "capability unavailable"
			}
			return nil, fmt.Errorf("capability %s blocked: %s", entry.descriptor.ID, reason)
		}
	}
	return promptHandler.RenderPrompt(ctx, state, args)
}

// ReadResource executes a runtime-backed resource capability by capability ID or public name.
func (r *CapabilityRegistry) ReadResource(ctx context.Context, state *Context, idOrName string) (*core.ResourceReadResult, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	entry, err := r.capabilityEntry(idOrName)
	if err != nil {
		return nil, err
	}
	resourceHandler, ok := entry.handler.(core.ResourceCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability %s is not a resource handler", entry.descriptor.ID)
	}
	if aware, ok := entry.handler.(core.AvailabilityAwareCapabilityHandler); ok {
		if availability := aware.Availability(ctx, state); !availability.Available {
			reason := strings.TrimSpace(availability.Reason)
			if reason == "" {
				reason = "capability unavailable"
			}
			return nil, fmt.Errorf("capability %s blocked: %s", entry.descriptor.ID, reason)
		}
	}
	return resourceHandler.ReadResource(ctx, state)
}

func (r *CapabilityRegistry) capabilityEntry(idOrName string) (*capabilityEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if entry, ok := r.entries[idOrName]; ok {
		return entry, nil
	}
	for _, entry := range r.entries {
		if entry.descriptor.ID == idOrName || entry.descriptor.Name == idOrName {
			return entry, nil
		}
	}
	return nil, fmt.Errorf("capability %s not found", idOrName)
}

// CapabilityAvailable reports whether a registered capability is currently available for invocation.
func (r *CapabilityRegistry) CapabilityAvailable(ctx context.Context, state *Context, idOrName string) bool {
	if r == nil {
		return false
	}
	entry, err := r.capabilityEntry(idOrName)
	if err != nil || entry == nil {
		return false
	}
	aware, ok := entry.handler.(core.AvailabilityAwareCapabilityHandler)
	if !ok {
		return true
	}
	return aware.Availability(ctx, state).Available
}

// InvocableCapabilities returns non-hidden capability descriptors with an invocable runtime handler.
func (r *CapabilityRegistry) InvocableCapabilities() []CapabilityDescriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]CapabilityDescriptor, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry == nil || entry.handler == nil {
			continue
		}
		if _, ok := entry.handler.(core.InvocableCapabilityHandler); !ok {
			continue
		}
		if r.effectiveExposureLocked(entry.descriptor) == core.CapabilityExposureHidden {
			continue
		}
		res = append(res, entry.descriptor)
	}
	return res
}

func validateCoordinationDescriptor(desc core.CapabilityDescriptor) error {
	if err := core.ValidateCoordinationTargetMetadata(desc.Coordination); err != nil {
		return fmt.Errorf("coordination metadata invalid for %s: %w", desc.ID, err)
	}
	return nil
}

func providerKindForDescriptor(desc core.CapabilityDescriptor) core.ProviderKind {
	switch desc.Source.Scope {
	case core.CapabilityScopeProvider, core.CapabilityScopeRemote:
		return core.ProviderKindNodeDevice
	default:
		return core.ProviderKindBuiltin
	}
}
