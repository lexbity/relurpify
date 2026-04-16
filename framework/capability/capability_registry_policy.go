package capability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
)

// UsePermissionManager enables default-deny enforcement for all tools.
func (r *CapabilityRegistry) UsePermissionManager(agentID string, manager *PermissionManager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permissionManager = manager
	r.registeredAgentID = agentID
	r.syncPermissionAwareEntriesLocked()
	r.syncAgentSpecAwareEntriesLocked(r.agentSpec, agentID)
}

// UseAgentSpec wires per-tool policies and other manifest-driven knobs into the registry.
func (r *CapabilityRegistry) UseAgentSpec(agentID string, spec *AgentRuntimeSpec) {
	if spec == nil {
		return
	}
	r.mu.Lock()
	r.registeredAgentID = agentID
	r.agentSpec = spec
	r.refreshRuntimePolicyLocked()
	r.mu.Unlock()

	if spec.AllowedCapabilities != nil {
		r.setAllowedCapabilities(core.EffectiveAllowedCapabilitySelectors(spec), true)
	}
	r.setToolPolicies(spec.ToolExecutionPolicy)
	r.setCapabilityPolicies(spec.CapabilityPolicies)
	r.setExposurePolicies(effectiveExposurePolicies(spec))
	r.setClassPolicies(spec.GlobalPolicies)
	r.configureRuntimeSafety(spec.RuntimeSafety)

	r.mu.Lock()
	r.syncAgentSpecAwareEntriesLocked(spec, agentID)
	r.mu.Unlock()
}

// UseSandboxScope wires sandbox-enforced filesystem scope into file tools.
func (r *CapabilityRegistry) UseSandboxScope(scope *sandbox.FileScopePolicy) {
	if r == nil || scope == nil {
		return
	}
	r.mu.Lock()
	r.sandboxScope = scope
	r.syncSandboxScopeAwareEntriesLocked()
	r.mu.Unlock()
}

func (r *CapabilityRegistry) setAllowedCapabilities(allowed []core.CapabilitySelector, configured bool) {
	if r == nil || !configured {
		return
	}
	if len(allowed) == 0 {
		r.mu.Lock()
		r.allowedCapabilities = []core.CapabilitySelector{}
		r.allowedMatchers = nil
		r.capabilities = make(map[string]core.CapabilityDescriptor)
		r.entries = make(map[string]*capabilityEntry)
		r.capabilityNameIndex = make(map[string][]string)
		r.localToolNameIndex = make(map[string]string)
		r.mu.Unlock()
		return
	}
	r.mu.Lock()
	r.allowedCapabilities = core.CloneCapabilitySelectors(allowed)
	r.allowedMatchers = compileSelectors(allowed)
	r.mu.Unlock()
	r.RestrictToCapabilities(allowed)
}

func (r *CapabilityRegistry) setToolPolicies(policies map[string]ToolPolicy) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.toolPolicies = make(map[string]ToolPolicy, len(policies))
	for name, policy := range policies {
		r.toolPolicies[name] = policy
	}
	r.refreshRuntimePolicyLocked()
	r.mu.Unlock()
}

func (r *CapabilityRegistry) setCapabilityPolicies(policies []core.CapabilityPolicy) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.capabilityPolicies = append([]core.CapabilityPolicy{}, policies...)
	r.refreshRuntimePolicyLocked()
	r.mu.Unlock()
}

func (r *CapabilityRegistry) setExposurePolicies(policies []core.CapabilityExposurePolicy) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.exposurePolicies = append([]core.CapabilityExposurePolicy{}, policies...)
	r.refreshRuntimePolicyLocked()
	telemetry := r.telemetry
	resolved := r.snapshotResolvedExposureLocked()
	r.mu.Unlock()
	for _, item := range resolved {
		emitCapabilitySecurityEvent(telemetry, "capability_exposure_resolved", item.descriptor, item.exposure, "")
	}
}

func (r *CapabilityRegistry) AddExposurePolicies(policies []core.CapabilityExposurePolicy) {
	if r == nil || len(policies) == 0 {
		return
	}
	r.mu.Lock()
	r.exposurePolicies = append(r.exposurePolicies, policies...)
	r.refreshRuntimePolicyLocked()
	telemetry := r.telemetry
	resolved := r.snapshotResolvedExposureLocked()
	r.mu.Unlock()
	for _, item := range resolved {
		emitCapabilitySecurityEvent(telemetry, "capability_exposure_resolved", item.descriptor, item.exposure, "")
	}
}

type resolvedExposure struct {
	descriptor core.CapabilityDescriptor
	exposure   core.CapabilityExposure
}

func (r *CapabilityRegistry) snapshotResolvedExposureLocked() []resolvedExposure {
	if r == nil {
		return nil
	}
	out := make([]resolvedExposure, 0, len(r.capabilities))
	for _, capability := range r.capabilities {
		out = append(out, resolvedExposure{
			descriptor: capability,
			exposure:   r.effectiveExposureLocked(capability),
		})
	}
	return out
}

func effectiveExposurePolicies(spec *AgentRuntimeSpec) []core.CapabilityExposurePolicy {
	if spec == nil {
		return nil
	}
	policies := append([]core.CapabilityExposurePolicy{}, spec.ExposurePolicies...)
	if spec.Browser != nil && spec.Browser.Enabled {
		policies = append(policies, core.CapabilityExposurePolicy{
			Selector: core.CapabilitySelector{
				Name:            "browser",
				RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyProvider},
			},
			Access: core.CapabilityExposureCallable,
		})
	}
	return policies
}

// EffectiveExposure resolves the effective visibility of a capability.
func (r *CapabilityRegistry) EffectiveExposure(desc core.CapabilityDescriptor) core.CapabilityExposure {
	if r == nil {
		return defaultCapabilityExposure(desc)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.effectiveExposureLocked(desc)
}

func (r *CapabilityRegistry) effectiveExposureLocked(desc core.CapabilityDescriptor) core.CapabilityExposure {
	result := defaultCapabilityExposure(desc)
	entry, ok := r.entries[desc.ID]
	if ok {
		result = defaultCapabilityExposureForEntry(desc, entry)
	}
	for _, policy := range r.currentRuntimePolicyLocked().compiledExposurePolicies {
		if ok {
			if !compiledSelectorMatches(policy.selector, entry.profile) {
				continue
			}
		} else if !compiledSelectorMatches(policy.selector, buildDescriptorProfile(desc)) {
			continue
		}
		result = policy.access
	}
	return result
}

func defaultCapabilityExposure(desc core.CapabilityDescriptor) core.CapabilityExposure {
	switch desc.RuntimeFamily {
	case core.CapabilityRuntimeFamilyLocalTool:
		return core.CapabilityExposureCallable
	case core.CapabilityRuntimeFamilyProvider:
		return core.CapabilityExposureInspectable
	default:
		switch desc.Kind {
		case core.CapabilityKindTool:
			switch desc.Source.Scope {
			case core.CapabilityScopeProvider, core.CapabilityScopeRemote:
				return core.CapabilityExposureInspectable
			default:
				return core.CapabilityExposureCallable
			}
		default:
			return core.CapabilityExposureInspectable
		}
	}
}

func defaultCapabilityExposureForEntry(desc core.CapabilityDescriptor, entry *capabilityEntry) core.CapabilityExposure {
	if entry == nil {
		return defaultCapabilityExposure(desc)
	}
	switch desc.RuntimeFamily {
	case core.CapabilityRuntimeFamilyLocalTool:
		return core.CapabilityExposureCallable
	case core.CapabilityRuntimeFamilyProvider:
		return core.CapabilityExposureInspectable
	case core.CapabilityRuntimeFamilyRelurpic:
		if _, ok := entry.handler.(core.InvocableCapabilityHandler); ok {
			return core.CapabilityExposureCallable
		}
		return core.CapabilityExposureInspectable
	default:
		return defaultCapabilityExposure(desc)
	}
}

func exposureRestrictiveness(access core.CapabilityExposure) int {
	switch access {
	case core.CapabilityExposureHidden:
		return 0
	case core.CapabilityExposureInspectable:
		return 1
	case core.CapabilityExposureCallable:
		return 2
	default:
		return 1
	}
}

func cloneToolPolicies(input map[string]ToolPolicy) map[string]ToolPolicy {
	if input == nil {
		return nil
	}
	out := make(map[string]ToolPolicy, len(input))
	for k, v := range input {
		out[k] = v
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

func (r *CapabilityRegistry) configureRuntimeSafety(spec *core.RuntimeSafetySpec) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.safety == nil {
		r.safety = newRuntimeSafetyController()
	}
	controller := r.safety
	r.mu.Unlock()
	controller.Configure(spec)
}

func cloneInsertionPolicies(input []core.CapabilityInsertionPolicy) []core.CapabilityInsertionPolicy {
	if len(input) == 0 {
		return nil
	}
	out := make([]core.CapabilityInsertionPolicy, len(input))
	copy(out, input)
	return out
}

func cloneProviderPolicies(input map[string]core.ProviderPolicy) map[string]core.ProviderPolicy {
	if input == nil {
		return nil
	}
	out := make(map[string]core.ProviderPolicy, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneRuntimeSafetySpec(input *core.RuntimeSafetySpec) *core.RuntimeSafetySpec {
	if input == nil {
		return nil
	}
	clone := *input
	return &clone
}

// setClassPolicies stores global capability-class policies and re-wraps all tools.
func (r *CapabilityRegistry) setClassPolicies(policies map[string]AgentPermissionLevel) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.globalPolicies = cloneGlobalPolicies(policies)
	r.refreshRuntimePolicyLocked()
	r.mu.Unlock()
}

// GetToolPolicies returns a snapshot of per-tool execution policies.
func (r *CapabilityRegistry) GetToolPolicies() map[string]ToolPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]ToolPolicy, len(r.currentRuntimePolicyLocked().toolPolicies))
	for k, v := range r.currentRuntimePolicyLocked().toolPolicies {
		out[k] = v
	}
	return out
}

// GetClassPolicies returns a snapshot of capability-class permission policies.
func (r *CapabilityRegistry) GetClassPolicies() map[string]AgentPermissionLevel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneGlobalPolicies(r.currentRuntimePolicyLocked().globalPolicies)
}

// CapturePolicySnapshot returns the effective registry policy state.
func (r *CapabilityRegistry) CapturePolicySnapshot() *core.PolicySnapshot {
	if r == nil {
		return nil
	}
	if r.delegate != nil {
		return r.delegate.CapturePolicySnapshot()
	}
	now := time.Now().UTC()
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.capturePolicySnapshotLocked(now)
}

func (r *CapabilityRegistry) capturePolicySnapshotLocked(now time.Time) *core.PolicySnapshot {
	if r == nil {
		return nil
	}
	policy := r.currentRuntimePolicyLocked()
	snapshot := &core.PolicySnapshot{
		ID:                 fmt.Sprintf("policy-%d", now.UnixNano()),
		CapturedAt:         now,
		AgentID:            r.registeredAgentID,
		ToolPolicies:       make(map[string]ToolPolicy, len(policy.toolPolicies)),
		CapabilityPolicies: append([]core.CapabilityPolicy{}, policy.capabilityPolicies...),
		ExposurePolicies:   append([]core.CapabilityExposurePolicy{}, policy.exposurePolicies...),
		GlobalPolicies:     cloneGlobalPolicies(policy.globalPolicies),
	}
	if policy != nil {
		snapshot.InsertionPolicies = cloneInsertionPolicies(policy.insertionPolicies)
		snapshot.ProviderPolicies = cloneProviderPolicies(policy.providerPolicies)
		snapshot.RuntimeSafety = cloneRuntimeSafetySpec(policy.runtimeSafety)
	}
	if r.safety != nil {
		snapshot.Revocations = r.safety.RevocationSnapshot()
	}
	for name, policy := range r.currentRuntimePolicyLocked().toolPolicies {
		snapshot.ToolPolicies[name] = policy
	}
	return snapshot
}

func clonePolicySnapshot(input *core.PolicySnapshot) *core.PolicySnapshot {
	if input == nil {
		return nil
	}
	return &core.PolicySnapshot{
		ID:                 input.ID,
		CapturedAt:         input.CapturedAt,
		AgentID:            input.AgentID,
		ToolPolicies:       cloneToolPolicies(input.ToolPolicies),
		CapabilityPolicies: append([]core.CapabilityPolicy{}, input.CapabilityPolicies...),
		ExposurePolicies:   append([]core.CapabilityExposurePolicy{}, input.ExposurePolicies...),
		InsertionPolicies:  cloneInsertionPolicies(input.InsertionPolicies),
		GlobalPolicies:     cloneGlobalPolicies(input.GlobalPolicies),
		ProviderPolicies:   cloneProviderPolicies(input.ProviderPolicies),
		RuntimeSafety:      cloneRuntimeSafetySpec(input.RuntimeSafety),
		Revocations: core.RevocationSnapshot{
			Capabilities: cloneSnapshotStringMap(input.Revocations.Capabilities),
			Providers:    cloneSnapshotStringMap(input.Revocations.Providers),
			Sessions:     cloneSnapshotStringMap(input.Revocations.Sessions),
		},
	}
}

func cloneSnapshotStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (r *CapabilityRegistry) RevokeCapability(id, reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.safety == nil {
		r.safety = newRuntimeSafetyController()
	}
	controller := r.safety
	r.mu.Unlock()
	controller.RevokeCapability(id, reason)
}

func (r *CapabilityRegistry) RevokeProvider(id, reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.safety == nil {
		r.safety = newRuntimeSafetyController()
	}
	controller := r.safety
	r.mu.Unlock()
	controller.RevokeProvider(id, reason)
}

func (r *CapabilityRegistry) RevokeSession(id, reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.safety == nil {
		r.safety = newRuntimeSafetyController()
	}
	controller := r.safety
	r.mu.Unlock()
	controller.RevokeSession(id, reason)
}

func (r *CapabilityRegistry) RecordSessionSubprocess(sessionID string, count int) error {
	if r == nil || sessionID == "" || count <= 0 {
		return nil
	}
	r.mu.Lock()
	if r.safety == nil {
		r.safety = newRuntimeSafetyController()
	}
	controller := r.safety
	r.mu.Unlock()
	return controller.RecordSessionSubprocess(sessionID, count)
}

func (r *CapabilityRegistry) RecordSessionNetworkRequest(sessionID string, count int) error {
	if r == nil || sessionID == "" || count <= 0 {
		return nil
	}
	r.mu.Lock()
	if r.safety == nil {
		r.safety = newRuntimeSafetyController()
	}
	controller := r.safety
	r.mu.Unlock()
	return controller.RecordSessionNetworkRequest(sessionID, count)
}

// UpdateToolPolicy updates a single tool's execution policy in-memory.
func (r *CapabilityRegistry) UpdateToolPolicy(name string, policy ToolPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.toolPolicies == nil {
		r.toolPolicies = make(map[string]ToolPolicy)
	}
	r.toolPolicies[name] = policy
	r.refreshRuntimePolicyLocked()
}

// UpdateClassPolicy updates a single capability-class policy in-memory.
func (r *CapabilityRegistry) UpdateClassPolicy(class string, level AgentPermissionLevel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.globalPolicies == nil {
		r.globalPolicies = make(map[string]AgentPermissionLevel)
	}
	if level == "" {
		delete(r.globalPolicies, class)
	} else {
		r.globalPolicies[class] = level
	}
	r.refreshRuntimePolicyLocked()
}

func effectiveClassPolicy(tool Tool, policies map[string]AgentPermissionLevel) AgentPermissionLevel {
	var result AgentPermissionLevel
	for _, label := range capabilityPolicyLabels(tool) {
		level, ok := policies[label]
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
func (r *CapabilityRegistry) UseTelemetry(telemetry Telemetry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.telemetry = telemetry
}

// RestrictTo removes tools not present in the allowed set.
func (r *CapabilityRegistry) RestrictTo(allowed []string) {
	if len(allowed) == 0 {
		return
	}
	selectors := make([]core.CapabilitySelector, 0, len(allowed))
	for _, name := range allowed {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		selectors = append(selectors, core.CapabilitySelector{Name: name, Kind: core.CapabilityKindTool})
	}
	r.RestrictToCapabilities(selectors)
}

// RestrictToCapabilities removes tools and capabilities not matched by the selector set.
func (r *CapabilityRegistry) RestrictToCapabilities(allowed []core.CapabilitySelector) {
	if len(allowed) == 0 {
		return
	}
	compiledAllowed := compileSelectors(allowed)
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, capability := range r.capabilities {
		if !matchesAnyCompiledCapabilitySelector(compiledAllowed, buildDescriptorProfile(capability)) {
			delete(r.capabilities, id)
			delete(r.entries, id)
		}
	}
	r.rebuildIndexesLocked()
}

func matchesAnyCapabilitySelector(selectors []core.CapabilitySelector, desc core.CapabilityDescriptor) bool {
	return matchesAnyCompiledCapabilitySelector(compileSelectors(selectors), buildDescriptorProfile(desc))
}

func matchesAnyCompiledCapabilitySelector(selectors []compiledSelector, profile descriptorProfile) bool {
	if len(selectors) == 0 {
		return true
	}
	for _, selector := range selectors {
		if compiledSelectorMatches(selector, profile) {
			return true
		}
	}
	return false
}

func capabilityPolicyLabels(tool Tool) []string {
	if tool == nil {
		return nil
	}
	labels := make(map[string]struct{})
	desc := core.ToolDescriptor(context.Background(), nil, tool)
	for _, class := range desc.RiskClasses {
		labels[strings.ToLower(strings.TrimSpace(string(class)))] = struct{}{}
	}
	for _, class := range desc.EffectClasses {
		labels[strings.ToLower(strings.TrimSpace(string(class)))] = struct{}{}
	}
	if desc.TrustClass != "" {
		labels[strings.ToLower(strings.TrimSpace(string(desc.TrustClass)))] = struct{}{}
	}
	for _, tag := range tool.Tags() {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		labels[tag] = struct{}{}
	}
	out := make([]string, 0, len(labels))
	for label := range labels {
		out = append(out, label)
	}
	return out
}

func capabilityPolicyLabelsForDescriptor(desc core.CapabilityDescriptor) []string {
	labels := make(map[string]struct{})
	for _, class := range desc.RiskClasses {
		labels[strings.ToLower(strings.TrimSpace(string(class)))] = struct{}{}
	}
	for _, class := range desc.EffectClasses {
		labels[strings.ToLower(strings.TrimSpace(string(class)))] = struct{}{}
	}
	if desc.TrustClass != "" {
		labels[strings.ToLower(strings.TrimSpace(string(desc.TrustClass)))] = struct{}{}
	}
	if desc.RuntimeFamily != "" {
		labels[strings.ToLower(strings.TrimSpace(string(desc.RuntimeFamily)))] = struct{}{}
	}
	out := make([]string, 0, len(labels))
	for label := range labels {
		out = append(out, label)
	}
	return out
}

func effectiveCapabilityPolicy(tool Tool, policies []core.CapabilityPolicy) AgentPermissionLevel {
	if tool == nil || len(policies) == 0 {
		return ""
	}
	desc := core.ToolDescriptor(context.Background(), nil, tool)
	var result AgentPermissionLevel
	for _, policy := range policies {
		if !core.SelectorMatchesDescriptor(policy.Selector, desc) {
			continue
		}
		switch {
		case policy.Execute == AgentPermissionDeny:
			return AgentPermissionDeny
		case policy.Execute == AgentPermissionAsk && result != AgentPermissionDeny:
			result = AgentPermissionAsk
		case policy.Execute == AgentPermissionAllow && result == "":
			result = AgentPermissionAllow
		}
	}
	return result
}

func effectiveCapabilityPolicyForDescriptor(desc core.CapabilityDescriptor, policies []core.CapabilityPolicy) AgentPermissionLevel {
	return effectiveCompiledCapabilityPolicyForProfile(buildDescriptorProfile(desc), compileCapabilityPolicies(policies))
}

func effectiveCompiledCapabilityPolicyForProfile(profile descriptorProfile, policies []compiledCapabilityPolicy) AgentPermissionLevel {
	if len(policies) == 0 {
		return ""
	}
	var result AgentPermissionLevel
	for _, policy := range policies {
		if !compiledSelectorMatches(policy.selector, profile) {
			continue
		}
		switch {
		case policy.execute == AgentPermissionDeny:
			return AgentPermissionDeny
		case policy.execute == AgentPermissionAsk && result != AgentPermissionDeny:
			result = AgentPermissionAsk
		case policy.execute == AgentPermissionAllow && result == "":
			result = AgentPermissionAllow
		}
	}
	return result
}

func effectiveClassPolicyForDescriptor(desc core.CapabilityDescriptor, policies map[string]AgentPermissionLevel) AgentPermissionLevel {
	return effectiveClassPolicyForProfile(buildDescriptorProfile(desc), policies)
}

func effectiveClassPolicyForProfile(profile descriptorProfile, policies map[string]AgentPermissionLevel) AgentPermissionLevel {
	var result AgentPermissionLevel
	for _, label := range profile.classLabels {
		level, ok := policies[label]
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
