package registry

import (
	"context"
	"fmt"
	"sync"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"
	"codeburg.org/lexbit/relurpify/capability/provider"
	runtime "codeburg.org/lexbit/relurpify/capability/runtime"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/model"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
)

// PermissionAware allows tools to receive the permission manager for fine-grained
// runtime checks (e.g. verifying file paths against allowlists).
type PermissionAware interface {
	SetPermissionManager(manager PermissionManagerHandle, agentID string)
}

// AgentSpecAware allows tools to consume the agent manifest runtime spec for
// additional policy enforcement (e.g. bash/file matrices).
type AgentSpecAware interface {
	SetAgentSpec(spec *agentspec.AgentRuntimeSpec, agentID string)
}

// SandboxScopeAware allows tools and capability handlers to receive the
// sandbox-enforced file scope.
type SandboxScopeAware interface {
	SetSandboxScope(scope *permissions.FileScopePolicy)
}

// CapabilityRegistry maintains framework-owned capability descriptors plus the
// narrowed local-tool runtime and temporary model-bridge shims used during the
// migration away from generic tool-shaped invocation.
type CapabilityRegistry struct {
	mu                  sync.RWMutex
	capabilities        map[string]descriptor.CapabilityDescriptor
	entries             map[string]*capabilityEntry
	capabilityNameIndex map[string][]string
	localToolNameIndex  map[string]string
	prechecks           []InvocationPrecheck
	postchecks          []PostInvocationHook
	permissionManager   PermissionManagerHandle
	registeredAgentID   string
	agentSpec           *agentspec.AgentRuntimeSpec
	sandboxScope        *permissions.FileScopePolicy
	runtimePolicy       *compiledRuntimePolicy
	allowedCapabilities []agentspec.CapabilitySelector
	allowedMatchers     []compiledSelector
	toolPolicies        map[string]agentspec.ToolPolicy
	capabilityPolicies  []agentspec.CapabilityPolicy
	exposurePolicies    []agentspec.CapabilityExposurePolicy
	globalPolicies      map[string]agentspec.AgentPermissionLevel
	guidanceBroker      RecoveryGuidanceBroker
	telemetry           fwtelemetry.Telemetry
	safety              *runtime.RuntimeSafetyController
	policyEngine        PolicyEngine
	modelProfile        *model.ModelProfile
	toolAdmission       *ToolAdmissionPolicy

	rollbackTokens  map[string]ports.RollbackToken
	rollbackMu      sync.Mutex
	metrics         *fwtelemetry.ToolCallMetrics
	delegate        *CapabilityRegistry
	toolIDAllowlist map[string]struct{}
}

// NewRegistry builds a capability registry instance.
func NewRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{
		capabilities:        make(map[string]descriptor.CapabilityDescriptor),
		entries:             make(map[string]*capabilityEntry),
		capabilityNameIndex: make(map[string][]string),
		localToolNameIndex:  make(map[string]string),
		toolPolicies:        make(map[string]agentspec.ToolPolicy),
		safety:              runtime.NewRuntimeSafetyController(),
		rollbackTokens:      make(map[string]ports.RollbackToken),
	}
}

// SetMetrics attaches a metrics collector to the registry. A nil value is a
// valid no-op (default behaviour before this method is called).
func (r *CapabilityRegistry) SetMetrics(metrics *fwtelemetry.ToolCallMetrics) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.metrics = metrics
	r.mu.Unlock()
}

// SetPolicyEngine wires a policy engine for capability evaluation.
func (r *CapabilityRegistry) SetPolicyEngine(engine PolicyEngine) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policyEngine = engine
}

// UseToolAdmission configures manifest-driven tool gating for legacy tool registration.
func (r *CapabilityRegistry) UseToolAdmission(policy *ToolAdmissionPolicy) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.toolAdmission = policy
	r.mu.Unlock()
}

// WithAllowlist returns a scoped view of this registry that restricts
// ModelCallableTools, CaptureExecutionCatalogSnapshot, and InvokeCapability
// to the given capability IDs. All other operations delegate to the base registry.
// An empty allowedIDs slice returns the receiver unchanged.
// Used by the thought thoughtrecipe executor to enforce per-step capability scoping.
func (r *CapabilityRegistry) WithAllowlist(allowedIDs []string) *CapabilityRegistry {
	if r == nil || len(allowedIDs) == 0 {
		return r
	}
	allowlist := make(map[string]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		if id != "" {
			allowlist[id] = struct{}{}
		}
	}
	if len(allowlist) == 0 {
		return r
	}
	return &CapabilityRegistry{
		delegate:        r,
		toolIDAllowlist: allowlist,
	}
}

// isAllowlisted reports whether the given capability ID is permitted by the
// active toolIDAllowlist. Always true when toolIDAllowlist is nil.
func (r *CapabilityRegistry) isAllowlisted(id string) bool {
	if r.toolIDAllowlist == nil {
		return true
	}
	_, ok := r.toolIDAllowlist[id]
	return ok
}

type capabilityEntry struct {
	descriptor descriptor.CapabilityDescriptor
	profile    descriptorProfile
	handler    handler.CapabilityHandler
	legacyTool ports.Tool
	providerID string
	sessionID  string
}

type RegistrationBatchItem struct {
	Descriptor       descriptor.CapabilityDescriptor
	InvocableHandler handler.InvocableCapabilityHandler
	PromptHandler    handler.PromptCapabilityHandler
	ResourceHandler  handler.ResourceCapabilityHandler
	LegacyTool       ports.Tool
}

type admissionEvent struct {
	descriptor descriptor.CapabilityDescriptor
	exposure   agentspec.CapabilityExposure
}

func (r *CapabilityRegistry) localToolEntryByNameLocked(name string) (*capabilityEntry, bool) {
	name = normalizeComparable(name)
	if name == "" {
		return nil, false
	}
	id, ok := r.localToolNameIndex[name]
	if !ok || id == "" {
		return nil, false
	}
	entry, ok := r.entries[id]
	return entry, ok && entry != nil && entry.legacyTool != nil
}

func (r *CapabilityRegistry) localToolEntriesLocked() []*capabilityEntry {
	if r == nil {
		return nil
	}
	out := make([]*capabilityEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry == nil || entry.legacyTool == nil {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (r *CapabilityRegistry) rewrapLegacyEntryLocked(entry *capabilityEntry) {
	if r == nil || entry == nil || entry.legacyTool == nil {
		return
	}
	var inner ports.Tool = entry.legacyTool
	if instrumented, ok := entry.legacyTool.(*instrumentedTool); ok {
		inner = instrumented.Tool
	}
	entry.legacyTool = r.wrapTool(inner)
	entry.handler = legacyToolHandler{tool: entry.legacyTool}
}

func (r *CapabilityRegistry) syncPermissionAwareEntriesLocked() {
	if r == nil {
		return
	}
	for _, entry := range r.entries {
		if entry == nil {
			continue
		}
		if entry.legacyTool != nil {
			if aware, ok := unwrapTool(entry.legacyTool).(PermissionAware); ok {
				aware.SetPermissionManager(r.permissionManager, r.registeredAgentID)
			}
			continue
		}
		if entry.handler == nil {
			continue
		}
		if aware, ok := unwrapCapabilityHandler(entry.handler).(PermissionAware); ok {
			aware.SetPermissionManager(r.permissionManager, r.registeredAgentID)
		}
	}
}

func (r *CapabilityRegistry) syncAgentSpecAwareEntriesLocked(spec *agentspec.AgentRuntimeSpec, agentID string) {
	if r == nil || spec == nil {
		return
	}
	for _, entry := range r.entries {
		if entry == nil {
			continue
		}
		if entry.legacyTool != nil {
			if aware, ok := unwrapTool(entry.legacyTool).(AgentSpecAware); ok {
				aware.SetAgentSpec(spec, agentID)
			}
			continue
		}
		if entry.handler == nil {
			continue
		}
		if aware, ok := unwrapCapabilityHandler(entry.handler).(AgentSpecAware); ok {
			aware.SetAgentSpec(spec, agentID)
		}
	}
}

func (r *CapabilityRegistry) syncSandboxScopeAwareEntriesLocked() {
	if r == nil || r.sandboxScope == nil {
		return
	}
	for _, entry := range r.entries {
		if entry == nil {
			continue
		}
		if entry.legacyTool != nil {
			if aware, ok := unwrapTool(entry.legacyTool).(SandboxScopeAware); ok {
				aware.SetSandboxScope(r.sandboxScope)
			}
			continue
		}
		if entry.handler == nil {
			continue
		}
		if aware, ok := unwrapCapabilityHandler(entry.handler).(SandboxScopeAware); ok {
			aware.SetSandboxScope(r.sandboxScope)
		}
	}
}

func (r *CapabilityRegistry) rebuildIndexesLocked() {
	if r == nil {
		return
	}
	perfstats.IncCapabilityRegistryRebuild()
	r.capabilityNameIndex = make(map[string][]string, len(r.entries))
	r.localToolNameIndex = make(map[string]string)
	for id, entry := range r.entries {
		if entry == nil {
			continue
		}
		name := normalizeComparable(entry.descriptor.Name)
		if name != "" {
			r.capabilityNameIndex[name] = append(r.capabilityNameIndex[name], id)
		}
		if entry.legacyTool != nil {
			toolName := normalizeComparable(entry.legacyTool.Name())
			if toolName != "" {
				r.localToolNameIndex[toolName] = id
			}
		}
	}
}

func (r *CapabilityRegistry) indexEntryLocked(id string, entry *capabilityEntry) {
	if r == nil || entry == nil || id == "" {
		return
	}
	name := normalizeComparable(entry.descriptor.Name)
	if name != "" {
		r.capabilityNameIndex[name] = append(r.capabilityNameIndex[name], id)
	}
	if entry.legacyTool != nil {
		toolName := normalizeComparable(entry.legacyTool.Name())
		if toolName != "" {
			r.localToolNameIndex[toolName] = id
		}
	}
}

func (r *CapabilityRegistry) registerEntryLocked(desc descriptor.CapabilityDescriptor, entry *capabilityEntry) {
	if r == nil || entry == nil {
		return
	}
	if entry.legacyTool != nil && r.sandboxScope != nil {
		if aware, ok := unwrapTool(entry.legacyTool).(SandboxScopeAware); ok {
			aware.SetSandboxScope(r.sandboxScope)
		}
	}
	if entry.handler != nil && r.sandboxScope != nil {
		if aware, ok := unwrapCapabilityHandler(entry.handler).(SandboxScopeAware); ok {
			aware.SetSandboxScope(r.sandboxScope)
		}
	}
	r.capabilities[desc.ID] = desc
	r.entries[desc.ID] = entry
	r.indexEntryLocked(desc.ID, entry)
}

// RegisterCapability adds a non-tool capability descriptor to the shared registry.
func (r *CapabilityRegistry) RegisterCapability(descriptor descriptor.CapabilityDescriptor) error {
	return r.RegisterBatch([]RegistrationBatchItem{{Descriptor: descriptor}})
}

// RegisterInvocableCapability registers a runtime-backed invocable
func (r *CapabilityRegistry) RegisterInvocableCapability(handler handler.InvocableCapabilityHandler) error {
	return r.RegisterBatch([]RegistrationBatchItem{{InvocableHandler: handler}})
}

// RegisterPromptCapability registers a runtime-backed prompt
func (r *CapabilityRegistry) RegisterPromptCapability(handler handler.PromptCapabilityHandler) error {
	return r.RegisterBatch([]RegistrationBatchItem{{PromptHandler: handler}})
}

// RegisterResourceCapability registers a runtime-backed resource
func (r *CapabilityRegistry) RegisterResourceCapability(handler handler.ResourceCapabilityHandler) error {
	return r.RegisterBatch([]RegistrationBatchItem{{ResourceHandler: handler}})
}

// ProviderCapabilityRegistrar returns a registrar that normalizes provider-
// backed capabilities against provider metadata and agent policy before
// registration.
func (r *CapabilityRegistry) ProviderCapabilityRegistrar(provider provider.ProviderDescriptor, policy agentspec.ProviderPolicy) (provider.CapabilityRegistrar, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	if err := provider.Validate(); err != nil {
		return nil, err
	}
	if err := agentspec.ValidateProviderPolicy(policy); err != nil {
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
	provider provider.ProviderDescriptor
	policy   agentspec.ProviderPolicy
}

func (r providerCapabilityRegistrar) RegisterCapability(descriptor descriptor.CapabilityDescriptor) error {
	normalized, err := provider.NormalizeProviderCapability(descriptor, r.provider, r.policy)
	if err != nil {
		return err
	}
	return r.registry.RegisterCapability(normalized)
}

func (r providerCapabilityRegistrar) RegisterCapabilitiesBatch(descriptors []descriptor.CapabilityDescriptor) error {
	items := make([]RegistrationBatchItem, 0, len(descriptors))
	for _, descriptor := range descriptors {
		normalized, err := provider.NormalizeProviderCapability(descriptor, r.provider, r.policy)
		if err != nil {
			return err
		}
		items = append(items, RegistrationBatchItem{Descriptor: normalized})
	}
	return r.registry.RegisterBatch(items)
}

// Register adds a tool to the registry.
func (r *CapabilityRegistry) Register(tool ports.Tool) error {
	return r.RegisterLegacyTool(tool)
}

// RegisterLegacyTool adds a legacy ports.Tool implementation to the registry by
// adapting it into a tool-kind capability entry.
func (r *CapabilityRegistry) RegisterLegacyTool(tool ports.Tool) error {
	return r.RegisterBatch([]RegistrationBatchItem{{LegacyTool: tool}})
}

func (r *CapabilityRegistry) RegisterCapabilitiesBatch(descriptors []descriptor.CapabilityDescriptor) error {
	items := make([]RegistrationBatchItem, 0, len(descriptors))
	for _, descriptor := range descriptors {
		items = append(items, RegistrationBatchItem{Descriptor: descriptor})
	}
	return r.RegisterBatch(items)
}

func (r *CapabilityRegistry) RegisterBatch(items []RegistrationBatchItem) error {
	if r == nil {
		return fmt.Errorf("registry unavailable")
	}
	if len(items) == 0 {
		return nil
	}
	r.mu.Lock()
	telemetry := r.telemetry
	events := make([]admissionEvent, 0, len(items))
	seenIDs := make(map[string]struct{}, len(items))
	seenToolNames := make(map[string]struct{}, len(items))
	for _, item := range items {
		desc, entry, err := r.prepareBatchEntryLocked(item, seenIDs, seenToolNames)
		if err != nil {
			r.mu.Unlock()
			return err
		}
		if entry == nil {
			continue
		}
		r.registerEntryLocked(desc, entry)
		events = append(events, admissionEvent{
			descriptor: desc,
			exposure:   r.effectiveExposureLocked(desc),
		})
	}
	r.mu.Unlock()
	for _, event := range events {
		emitCapabilitySecurityEvent(telemetry, "capability_admitted", event.descriptor, event.exposure, "")
	}
	return nil
}

func (r *CapabilityRegistry) prepareBatchEntryLocked(item RegistrationBatchItem, seenIDs, seenToolNames map[string]struct{}) (descriptor.CapabilityDescriptor, *capabilityEntry, error) {
	switch {
	case item.LegacyTool != nil:
		return r.prepareLegacyToolBatchEntryLocked(item.LegacyTool, seenIDs, seenToolNames)
	case item.InvocableHandler != nil:
		return r.prepareHandlerBatchEntryLocked(item.InvocableHandler, seenIDs)
	case item.PromptHandler != nil:
		return r.prepareHandlerBatchEntryLocked(item.PromptHandler, seenIDs)
	case item.ResourceHandler != nil:
		return r.prepareHandlerBatchEntryLocked(item.ResourceHandler, seenIDs)
	case item.Descriptor.ID != "" || item.Descriptor.Name != "":
		return r.prepareDescriptorBatchEntryLocked(item.Descriptor, seenIDs)
	default:
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("batch item missing registration payload")
	}
}

func (r *CapabilityRegistry) prepareDescriptorBatchEntryLocked(desc descriptor.CapabilityDescriptor, seenIDs map[string]struct{}) (descriptor.CapabilityDescriptor, *capabilityEntry, error) {
	desc = descriptor.NormalizeCapabilityDescriptor(desc)
	if desc.ID == "" {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("capability id required")
	}
	if err := validateCoordinationDescriptor(desc); err != nil {
		return descriptor.CapabilityDescriptor{}, nil, err
	}
	if _, ok := r.capabilities[desc.ID]; ok {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("capability %s already registered", desc.ID)
	}
	if _, ok := seenIDs[desc.ID]; ok {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("capability %s already registered", desc.ID)
	}
	seenIDs[desc.ID] = struct{}{}
	profile := buildDescriptorProfile(desc)
	if !matchesAnyCompiledCapabilitySelector(r.allowedMatchers, profile) {
		return descriptor.CapabilityDescriptor{}, nil, nil
	}
	return desc, &capabilityEntry{
		descriptor: desc,
		profile:    profile,
		providerID: desc.Source.ProviderID,
		sessionID:  desc.Source.SessionID,
	}, nil
}

func (r *CapabilityRegistry) prepareHandlerBatchEntryLocked(handler handler.CapabilityHandler, seenIDs map[string]struct{}) (descriptor.CapabilityDescriptor, *capabilityEntry, error) {
	if handler == nil {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("capability handler required")
	}
	desc := descriptor.NormalizeCapabilityDescriptor(handler.Descriptor(context.Background(), nil))
	if desc.ID == "" {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("capability id required")
	}
	if err := validateCoordinationDescriptor(desc); err != nil {
		return descriptor.CapabilityDescriptor{}, nil, err
	}
	if _, ok := r.entries[desc.ID]; ok {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("capability %s already registered", desc.ID)
	}
	if _, ok := seenIDs[desc.ID]; ok {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("capability %s already registered", desc.ID)
	}
	seenIDs[desc.ID] = struct{}{}
	profile := buildDescriptorProfile(desc)
	if !matchesAnyCompiledCapabilitySelector(r.allowedMatchers, profile) {
		return descriptor.CapabilityDescriptor{}, nil, nil
	}
	return desc, &capabilityEntry{
		descriptor: desc,
		profile:    profile,
		handler:    r.wrapCapabilityHandlerPrepared(handler, desc, profile),
		providerID: desc.Source.ProviderID,
		sessionID:  desc.Source.SessionID,
	}, nil
}

func (r *CapabilityRegistry) prepareLegacyToolBatchEntryLocked(tool ports.Tool, seenIDs, seenToolNames map[string]struct{}) (descriptor.CapabilityDescriptor, *capabilityEntry, error) {
	if tool == nil {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("tool required")
	}
	if r.toolAdmission != nil {
		allowed, err := r.toolAdmission.Admit(tool)
		if err != nil {
			return descriptor.CapabilityDescriptor{}, nil, err
		}
		if !allowed {
			return descriptor.CapabilityDescriptor{}, nil, nil
		}
	}
	desc := descriptor.NormalizeCapabilityDescriptor(descriptor.ToolDescriptor(context.Background(), tool))
	if desc.RuntimeFamily != agentspec.CapabilityRuntimeFamilyLocalTool {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("legacy tool registration only supports local-tool runtime family; %s is %s", desc.ID, desc.RuntimeFamily)
	}
	normalizedName := normalizeComparable(tool.Name())
	if _, exists := r.localToolEntryByNameLocked(tool.Name()); exists {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("tool %s already registered", tool.Name())
	}
	if _, exists := seenToolNames[normalizedName]; exists {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("tool %s already registered", tool.Name())
	}
	if _, exists := r.capabilities[desc.ID]; exists {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("capability %s already registered", desc.ID)
	}
	if _, exists := seenIDs[desc.ID]; exists {
		return descriptor.CapabilityDescriptor{}, nil, fmt.Errorf("capability %s already registered", desc.ID)
	}
	seenIDs[desc.ID] = struct{}{}
	profile := buildDescriptorProfile(desc)
	seenToolNames[normalizedName] = struct{}{}
	if !matchesAnyCompiledCapabilitySelector(r.allowedMatchers, profile) {
		return descriptor.CapabilityDescriptor{}, nil, nil
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
	return desc, &capabilityEntry{
		descriptor: desc,
		profile:    profile,
		handler:    adapter,
		legacyTool: wrapped,
		providerID: desc.Source.ProviderID,
		sessionID:  desc.Source.SessionID,
	}, nil
}

// Get fetches a tool by name.
func (r *CapabilityRegistry) Get(name string) (ports.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.localToolEntryByNameLocked(name)
	if !ok || entry == nil || entry.legacyTool == nil {
		return nil, false
	}
	return entry.legacyTool, true
}

// HasCapability reports whether a capability is registered by ID or public name.
func (r *CapabilityRegistry) HasCapability(idOrName string) bool {
	_, ok := r.GetCapability(idOrName)
	return ok
}

// All returns tools exposed as callable to the active agent.
func (r *CapabilityRegistry) All() []ports.Tool {
	return r.CallableTools()
}

// CallableTools returns only tools exposed as callable to agents.
func (r *CapabilityRegistry) CallableTools() []ports.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := r.localToolEntriesLocked()
	res := make([]ports.Tool, 0, len(entries))
	for _, entry := range entries {
		if r.effectiveExposureLocked(entry.descriptor) != agentspec.CapabilityExposureCallable {
			continue
		}
		if !toolAvailableForPrompt(entry.legacyTool) {
			continue
		}
		res = append(res, entry.legacyTool)
	}
	return res
}

// InspectableTools returns tools visible for operator inspection.
func (r *CapabilityRegistry) InspectableTools() []ports.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := r.localToolEntriesLocked()
	res := make([]ports.Tool, 0, len(entries))
	for _, entry := range entries {
		if r.effectiveExposureLocked(entry.descriptor) == agentspec.CapabilityExposureHidden {
			continue
		}
		res = append(res, entry.legacyTool)
	}
	return res
}

// GetCapability resolves a tool by either capability ID or public name.
func (r *CapabilityRegistry) GetCapability(idOrName string) (descriptor.CapabilityDescriptor, bool) {
	if r == nil {
		return descriptor.CapabilityDescriptor{}, false
	}
	if r.delegate != nil {
		return r.delegate.GetCapability(idOrName)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	idOrName = r.normalizeToolNameLocked(idOrName)
	if capability, ok := r.capabilities[idOrName]; ok {
		return capability, true
	}
	if ids := r.capabilityNameIndex[normalizeComparable(idOrName)]; len(ids) > 0 {
		for _, id := range ids {
			if capability, ok := r.capabilities[id]; ok {
				return capability, true
			}
		}
	}
	return descriptor.CapabilityDescriptor{}, false
}

// GetCoordinationTarget returns a non-hidden capability that is explicitly
// marked as a coordination target.
func (r *CapabilityRegistry) GetCoordinationTarget(idOrName string) (policy.DelegationTarget, bool) {
	if r == nil {
		return descriptor.CapabilityDescriptor{}, false
	}
	desc, ok := r.GetCapability(idOrName)
	if !ok || desc.Coordination == nil || !desc.Coordination.Target {
		return descriptor.CapabilityDescriptor{}, false
	}
	if r.EffectiveExposure(desc) == agentspec.CapabilityExposureHidden {
		return descriptor.CapabilityDescriptor{}, false
	}
	return desc, true
}

// AllCapabilities returns non-hidden capability descriptors.
func (r *CapabilityRegistry) AllCapabilities() []descriptor.CapabilityDescriptor {
	return r.InspectableCapabilities()
}

// CallableCapabilities returns descriptors exposed as callable to agents.
func (r *CapabilityRegistry) CallableCapabilities() []descriptor.CapabilityDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]descriptor.CapabilityDescriptor, 0, len(r.capabilities))
	for _, capability := range r.capabilities {
		if r.effectiveExposureLocked(capability) != agentspec.CapabilityExposureCallable {
			continue
		}
		res = append(res, capability)
	}
	return res
}

// CoordinationTargets returns admitted, non-hidden coordination target
// capabilities that match all provided selectors.
func (r *CapabilityRegistry) CoordinationTargets(selectors ...agentspec.CapabilitySelector) []policy.DelegationTarget {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]policy.DelegationTarget, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry == nil || entry.descriptor.Coordination == nil || !entry.descriptor.Coordination.Target {
			continue
		}
		if r.effectiveExposureLocked(entry.descriptor) == agentspec.CapabilityExposureHidden {
			continue
		}
		matched := true
		for _, selector := range selectors {
			if !compiledSelectorMatches(compileSelector(selector), entry.profile) {
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
func (r *CapabilityRegistry) InspectableCapabilities() []descriptor.CapabilityDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]descriptor.CapabilityDescriptor, 0, len(r.capabilities))
	for _, capability := range r.capabilities {
		if r.effectiveExposureLocked(capability) == agentspec.CapabilityExposureHidden {
			continue
		}
		res = append(res, capability)
	}
	return res
}

// CloneFiltered returns a new registry that contains the same tool wrappers and
// registry policies, but only keeps tools that match the predicate.
func (r *CapabilityRegistry) CloneFiltered(keep func(ports.Tool) bool) *CapabilityRegistry {
	if r == nil {
		return NewRegistry()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	clone := &CapabilityRegistry{
		capabilities:        make(map[string]descriptor.CapabilityDescriptor),
		entries:             make(map[string]*capabilityEntry),
		capabilityNameIndex: make(map[string][]string),
		localToolNameIndex:  make(map[string]string),
		prechecks:           append([]InvocationPrecheck{}, r.prechecks...),
		permissionManager:   r.permissionManager,
		registeredAgentID:   r.registeredAgentID,
		agentSpec:           r.agentSpec,
		runtimePolicy:       r.currentRuntimePolicyLocked(),
		allowedCapabilities: CloneCapabilitySelectors(r.allowedCapabilities),
		allowedMatchers:     append([]compiledSelector{}, r.allowedMatchers...),
		telemetry:           r.telemetry,
		safety:              r.safety,
		toolPolicies:        make(map[string]agentspec.ToolPolicy, len(r.toolPolicies)),
		capabilityPolicies:  append([]agentspec.CapabilityPolicy{}, r.capabilityPolicies...),
		exposurePolicies:    append([]agentspec.CapabilityExposurePolicy{}, r.exposurePolicies...),
		globalPolicies:      cloneGlobalPolicies(r.globalPolicies),
	}
	for name, pol := range r.toolPolicies {
		clone.toolPolicies[name] = pol
	}
	clone.refreshRuntimePolicyLocked()
	for id, capability := range r.capabilities {
		if capability.Kind == agentspec.CapabilityKindTool {
			continue
		}
		clone.capabilities[id] = capability
		if entry, ok := r.entries[id]; ok {
			clonedEntry := *entry
			if clonedEntry.handler != nil {
				clonedEntry.handler = clone.wrapCapabilityHandler(unwrapCapabilityHandler(clonedEntry.handler))
			}
			clone.entries[id] = &clonedEntry
		}
	}
	for _, entry := range r.entries {
		if entry == nil || entry.legacyTool == nil {
			continue
		}
		if keep != nil && !keep(entry.legacyTool) {
			continue
		}
		clonedTool := cloneTool(entry.legacyTool, clone)
		desc := descriptor.NormalizeCapabilityDescriptor(descriptor.ToolDescriptor(context.Background(), unwrapTool(clonedTool)))
		clone.capabilities[desc.ID] = desc
		clonedEntry := *entry
		clonedEntry.descriptor = desc
		clonedEntry.profile = buildDescriptorProfile(desc)
		clonedEntry.legacyTool = clonedTool
		clonedEntry.handler = legacyToolHandler{tool: clonedTool}
		clone.entries[desc.ID] = &clonedEntry
	}
	clone.rebuildIndexesLocked()
	return clone
}

func cloneTool(tool ports.Tool, registry *CapabilityRegistry) ports.Tool {
	if tool == nil {
		return nil
	}
	if t, ok := tool.(*instrumentedTool); ok {
		return &instrumentedTool{
			Tool:     t.Tool,
			registry: registry,
		}
	}
	return tool
}

func validateCoordinationDescriptor(desc descriptor.CapabilityDescriptor) error {
	if err := descriptor.ValidateCoordinationTargetMetadata(desc.Coordination); err != nil {
		return fmt.Errorf("coordination metadata invalid for %s: %w", desc.ID, err)
	}
	return nil
}
