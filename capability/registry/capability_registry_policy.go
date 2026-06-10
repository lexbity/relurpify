package registry

import (
	"context"
	"fmt"
	"strings"
	"time"

	capresult "codeburg.org/lexbit/relurpify/capability/result"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"
	runtime "codeburg.org/lexbit/relurpify/capability/runtime"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/safety"
	"codeburg.org/lexbit/relurpify/capability/classification"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/risk"
	"codeburg.org/lexbit/relurpify/model"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// UsePermissionManager enables default-deny enforcement for all tools.
func (r *CapabilityRegistry) UsePermissionManager(agentID string, manager PermissionManagerHandle) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permissionManager = manager
	r.registeredAgentID = agentID
	r.syncPermissionAwareEntriesLocked()
	r.syncAgentSpecAwareEntriesLocked(r.agentSpec, agentID)
}

// UseAgentSpec wires per-tool policies and other manifest-driven knobs into the registry.
func (r *CapabilityRegistry) UseAgentSpec(agentID string, spec *agentspec.AgentRuntimeSpec) {
	if spec == nil {
		return
	}
	r.mu.Lock()
	r.registeredAgentID = agentID
	r.agentSpec = spec
	r.refreshRuntimePolicyLocked()
	r.mu.Unlock()

	if spec.AllowedCapabilities != nil {
		r.setAllowedCapabilities(agentspec.EffectiveAllowedCapabilitySelectors(spec), true)
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
func (r *CapabilityRegistry) UseSandboxScope(scope *permissions.FileScopePolicy) {
	if r == nil || scope == nil {
		return
	}
	r.mu.Lock()
	r.sandboxScope = scope
	r.syncSandboxScopeAwareEntriesLocked()
	r.mu.Unlock()
}

func (r *CapabilityRegistry) setAllowedCapabilities(allowed []agentspec.CapabilitySelector, configured bool) {
	if r == nil || !configured {
		return
	}
	if len(allowed) == 0 {
		r.mu.Lock()
		r.allowedCapabilities = []agentspec.CapabilitySelector{}
		r.allowedMatchers = nil
		r.capabilities = make(map[string]descriptor.CapabilityDescriptor)
		r.entries = make(map[string]*capabilityEntry)
		r.capabilityNameIndex = make(map[string][]string)
		r.localToolNameIndex = make(map[string]string)
		r.mu.Unlock()
		return
	}
	r.mu.Lock()
	r.allowedCapabilities = CloneCapabilitySelectors(allowed)
	r.allowedMatchers = compileSelectors(allowed)
	r.mu.Unlock()
	r.RestrictToCapabilities(allowed)
}

func (r *CapabilityRegistry) setToolPolicies(policies map[string]agentspec.ToolPolicy) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.toolPolicies = make(map[string]agentspec.ToolPolicy, len(policies))
	for name, policy := range policies {
		r.toolPolicies[name] = policy
	}
	r.refreshRuntimePolicyLocked()
	r.mu.Unlock()
}

func (r *CapabilityRegistry) setCapabilityPolicies(policies []agentspec.CapabilityPolicy) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.capabilityPolicies = append([]agentspec.CapabilityPolicy{}, policies...)
	r.refreshRuntimePolicyLocked()
	r.mu.Unlock()
}

func (r *CapabilityRegistry) setExposurePolicies(policies []agentspec.CapabilityExposurePolicy) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.exposurePolicies = append([]agentspec.CapabilityExposurePolicy{}, policies...)
	r.refreshRuntimePolicyLocked()
	telemetry := r.telemetry
	resolved := r.snapshotResolvedExposureLocked()
	r.mu.Unlock()
	for _, item := range resolved {
		emitCapabilitySecurityEvent(telemetry, "capability_exposure_resolved", item.descriptor, item.exposure, "")
	}
}

func (r *CapabilityRegistry) AddExposurePolicies(policies []agentspec.CapabilityExposurePolicy) {
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
	descriptor descriptor.CapabilityDescriptor
	exposure   agentspec.CapabilityExposure
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

func effectiveExposurePolicies(spec *agentspec.AgentRuntimeSpec) []agentspec.CapabilityExposurePolicy {
	if spec == nil {
		return nil
	}
	policies := append([]agentspec.CapabilityExposurePolicy{}, spec.ExposurePolicies...)
	if spec.Browser != nil && spec.Browser.Enabled {
		policies = append(policies, agentspec.CapabilityExposurePolicy{
			Selector: agentspec.CapabilitySelector{
				Name:            "browser",
				RuntimeFamilies: []agentspec.CapabilityRuntimeFamily{agentspec.CapabilityRuntimeFamilyProvider},
			},
			Access: agentspec.CapabilityExposureCallable,
		})
	}
	return policies
}

// EffectiveExposure resolves the effective visibility of a
func (r *CapabilityRegistry) EffectiveExposure(desc descriptor.CapabilityDescriptor) agentspec.CapabilityExposure {
	if r == nil {
		return defaultCapabilityExposure(desc)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.effectiveExposureLocked(desc)
}

func (r *CapabilityRegistry) effectiveExposureLocked(desc descriptor.CapabilityDescriptor) agentspec.CapabilityExposure {
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

func defaultCapabilityExposure(desc descriptor.CapabilityDescriptor) agentspec.CapabilityExposure {
	switch desc.RuntimeFamily {
	case agentspec.CapabilityRuntimeFamilyLocalTool:
		return agentspec.CapabilityExposureCallable
	case agentspec.CapabilityRuntimeFamilyProvider:
		return agentspec.CapabilityExposureInspectable
	default:
		switch desc.Kind {
		case agentspec.CapabilityKindTool:
			switch desc.Source.Scope {
			case classification.CapabilityScopeProvider, classification.CapabilityScopeRemote:
				return agentspec.CapabilityExposureInspectable
			default:
				return agentspec.CapabilityExposureCallable
			}
		default:
			return agentspec.CapabilityExposureInspectable
		}
	}
}

func defaultCapabilityExposureForEntry(desc descriptor.CapabilityDescriptor, entry *capabilityEntry) agentspec.CapabilityExposure {
	if entry == nil {
		return defaultCapabilityExposure(desc)
	}
	switch desc.RuntimeFamily {
	case agentspec.CapabilityRuntimeFamilyLocalTool:
		return agentspec.CapabilityExposureCallable
	case agentspec.CapabilityRuntimeFamilyProvider:
		return agentspec.CapabilityExposureInspectable
	case agentspec.CapabilityRuntimeFamilyRelurpic:
		if _, ok := entry.handler.(handler.InvocableCapabilityHandler); ok {
			return agentspec.CapabilityExposureCallable
		}
		return agentspec.CapabilityExposureInspectable
	default:
		return defaultCapabilityExposure(desc)
	}
}

func exposureRestrictiveness(access agentspec.CapabilityExposure) int {
	switch access {
	case agentspec.CapabilityExposureHidden:
		return 0
	case agentspec.CapabilityExposureInspectable:
		return 1
	case agentspec.CapabilityExposureCallable:
		return 2
	default:
		return 1
	}
}

func cloneToolPolicies(input map[string]agentspec.ToolPolicy) map[string]agentspec.ToolPolicy {
	if input == nil {
		return nil
	}
	out := make(map[string]agentspec.ToolPolicy, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneGlobalPolicies(input map[string]agentspec.AgentPermissionLevel) map[string]agentspec.AgentPermissionLevel {
	if input == nil {
		return nil
	}
	out := make(map[string]agentspec.AgentPermissionLevel, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func (r *CapabilityRegistry) configureRuntimeSafety(spec *safety.RuntimeSafetySpec) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.safety == nil {
		r.safety = runtime.NewRuntimeSafetyController()
	}
	controller := r.safety
	r.mu.Unlock()
	controller.Configure(spec)
}

func cloneInsertionPolicies(input []agentspec.CapabilityInsertionPolicy) []agentspec.CapabilityInsertionPolicy {
	if len(input) == 0 {
		return nil
	}
	out := make([]agentspec.CapabilityInsertionPolicy, len(input))
	copy(out, input)
	return out
}

func cloneProviderPolicies(input map[string]agentspec.ProviderPolicy) map[string]agentspec.ProviderPolicy {
	if input == nil {
		return nil
	}
	out := make(map[string]agentspec.ProviderPolicy, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneRuntimeSafetySpec(input *safety.RuntimeSafetySpec) *safety.RuntimeSafetySpec {
	if input == nil {
		return nil
	}
	clone := *input
	return &clone
}

// setClassPolicies stores global capability-class policies and re-wraps all tools.
func (r *CapabilityRegistry) setClassPolicies(policies map[string]agentspec.AgentPermissionLevel) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.globalPolicies = cloneGlobalPolicies(policies)
	r.refreshRuntimePolicyLocked()
	r.mu.Unlock()
}

// GetToolPolicies returns a snapshot of per-tool execution policies.
func (r *CapabilityRegistry) GetToolPolicies() map[string]agentspec.ToolPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]agentspec.ToolPolicy, len(r.currentRuntimePolicyLocked().toolPolicies))
	for k, v := range r.currentRuntimePolicyLocked().toolPolicies {
		out[k] = v
	}
	return out
}

// GetClassPolicies returns a snapshot of capability-class permission policies.
func (r *CapabilityRegistry) GetClassPolicies() map[string]agentspec.AgentPermissionLevel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneGlobalPolicies(r.currentRuntimePolicyLocked().globalPolicies)
}

// CapturePolicySnapshot returns the effective registry policy state.
func (r *CapabilityRegistry) CapturePolicySnapshot() *capresult.PolicySnapshot {
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

func (r *CapabilityRegistry) capturePolicySnapshotLocked(now time.Time) *capresult.PolicySnapshot {
	if r == nil {
		return nil
	}
	policy := r.currentRuntimePolicyLocked()
	snapshot := &capresult.PolicySnapshot{
		ID:                 fmt.Sprintf("policy-%d", now.UnixNano()),
		CapturedAt:         now,
		AgentID:            r.registeredAgentID,
		ToolPolicies:       make(map[string]agentspec.ToolPolicy, len(policy.toolPolicies)),
		CapabilityPolicies: append([]agentspec.CapabilityPolicy{}, policy.capabilityPolicies...),
		ExposurePolicies:   append([]agentspec.CapabilityExposurePolicy{}, policy.exposurePolicies...),
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

func clonePolicySnapshot(input *capresult.PolicySnapshot) *capresult.PolicySnapshot {
	if input == nil {
		return nil
	}
	return &capresult.PolicySnapshot{
		ID:                 input.ID,
		CapturedAt:         input.CapturedAt,
		AgentID:            input.AgentID,
		ToolPolicies:       cloneToolPolicies(input.ToolPolicies),
		CapabilityPolicies: append([]agentspec.CapabilityPolicy{}, input.CapabilityPolicies...),
		ExposurePolicies:   append([]agentspec.CapabilityExposurePolicy{}, input.ExposurePolicies...),
		InsertionPolicies:  cloneInsertionPolicies(input.InsertionPolicies),
		GlobalPolicies:     cloneGlobalPolicies(input.GlobalPolicies),
		ProviderPolicies:   cloneProviderPolicies(input.ProviderPolicies),
		RuntimeSafety:      cloneRuntimeSafetySpec(input.RuntimeSafety),
		Revocations: capresult.RevocationSnapshot{
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
		r.safety = runtime.NewRuntimeSafetyController()
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
		r.safety = runtime.NewRuntimeSafetyController()
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
		r.safety = runtime.NewRuntimeSafetyController()
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
		r.safety = runtime.NewRuntimeSafetyController()
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
		r.safety = runtime.NewRuntimeSafetyController()
	}
	controller := r.safety
	r.mu.Unlock()
	return controller.RecordSessionNetworkRequest(sessionID, count)
}

// UpdateToolPolicy updates a single tool's execution policy in-memory.
func (r *CapabilityRegistry) UpdateToolPolicy(name string, policy agentspec.ToolPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.toolPolicies == nil {
		r.toolPolicies = make(map[string]agentspec.ToolPolicy)
	}
	r.toolPolicies[name] = policy
	r.refreshRuntimePolicyLocked()
}

// UpdateClassPolicy updates a single capability-class policy in-memory.
func (r *CapabilityRegistry) UpdateClassPolicy(class string, level agentspec.AgentPermissionLevel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.globalPolicies == nil {
		r.globalPolicies = make(map[string]agentspec.AgentPermissionLevel)
	}
	if level == "" {
		delete(r.globalPolicies, class)
	} else {
		r.globalPolicies[class] = level
	}
	r.refreshRuntimePolicyLocked()
}

func effectiveClassPolicy(tool ports.Tool, policies map[string]agentspec.AgentPermissionLevel) agentspec.AgentPermissionLevel {
	var result agentspec.AgentPermissionLevel
	for _, label := range capabilityPolicyLabels(tool) {
		level, ok := policies[label]
		if !ok {
			continue
		}
		switch {
		case level == agentspec.AgentPermissionDeny:
			return agentspec.AgentPermissionDeny
		case level == agentspec.AgentPermissionAsk && result != agentspec.AgentPermissionDeny:
			result = agentspec.AgentPermissionAsk
		case level == agentspec.AgentPermissionAllow && result == "":
			result = agentspec.AgentPermissionAllow
		}
	}
	return result
}

// UseTelemetry wires a telemetry sink for all tool executions.
func (r *CapabilityRegistry) UseTelemetry(telemetry fwtelemetry.Telemetry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.telemetry = telemetry
}

// RestrictTo removes tools not present in the allowed set.
func (r *CapabilityRegistry) RestrictTo(allowed []string) {
	if len(allowed) == 0 {
		return
	}
	selectors := make([]agentspec.CapabilitySelector, 0, len(allowed))
	for _, name := range allowed {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		selectors = append(selectors, agentspec.CapabilitySelector{Name: name, Kind: agentspec.CapabilityKindTool})
	}
	r.RestrictToCapabilities(selectors)
}

// RestrictToCapabilities removes tools and capabilities not matched by the selector set.
func (r *CapabilityRegistry) RestrictToCapabilities(allowed []agentspec.CapabilitySelector) {
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

func matchesAnyCapabilitySelector(selectors []agentspec.CapabilitySelector, desc descriptor.CapabilityDescriptor) bool {
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

func capabilityPolicyLabels(tool ports.Tool) []string {
	if tool == nil {
		return nil
	}
	labels := make(map[string]struct{})
	desc := descriptor.ToolDescriptor(context.Background(), tool)
	for _, rc := range risk.Classify(desc.EffectClasses, desc.Source.Scope) {
		labels[strings.ToLower(strings.TrimSpace(string(rc)))] = struct{}{}
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

func capabilityPolicyLabelsForDescriptor(desc descriptor.CapabilityDescriptor) []string {
	labels := make(map[string]struct{})
	for _, rc := range risk.Classify(desc.EffectClasses, desc.Source.Scope) {
		labels[strings.ToLower(strings.TrimSpace(string(rc)))] = struct{}{}
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

func effectiveCapabilityPolicy(tool ports.Tool, policies []agentspec.CapabilityPolicy) agentspec.AgentPermissionLevel {
	if tool == nil || len(policies) == 0 {
		return ""
	}
	desc := descriptor.ToolDescriptor(context.Background(), tool)
	var result agentspec.AgentPermissionLevel
	for _, policy := range policies {
		if !SelectorMatchesDescriptor(policy.Selector, desc) {
			continue
		}
		switch {
		case policy.Execute == agentspec.AgentPermissionDeny:
			return agentspec.AgentPermissionDeny
		case policy.Execute == agentspec.AgentPermissionAsk && result != agentspec.AgentPermissionDeny:
			result = agentspec.AgentPermissionAsk
		case policy.Execute == agentspec.AgentPermissionAllow && result == "":
			result = agentspec.AgentPermissionAllow
		}
	}
	return result
}

func effectiveCapabilityPolicyForDescriptor(desc descriptor.CapabilityDescriptor, policies []agentspec.CapabilityPolicy) agentspec.AgentPermissionLevel {
	return effectiveCompiledCapabilityPolicyForProfile(buildDescriptorProfile(desc), compileCapabilityPolicies(policies))
}

func effectiveCompiledCapabilityPolicyForProfile(profile descriptorProfile, policies []compiledCapabilityPolicy) agentspec.AgentPermissionLevel {
	if len(policies) == 0 {
		return ""
	}
	var result agentspec.AgentPermissionLevel
	for _, policy := range policies {
		if !compiledSelectorMatches(policy.selector, profile) {
			continue
		}
		switch {
		case policy.execute == agentspec.AgentPermissionDeny:
			return agentspec.AgentPermissionDeny
		case policy.execute == agentspec.AgentPermissionAsk && result != agentspec.AgentPermissionDeny:
			result = agentspec.AgentPermissionAsk
		case policy.execute == agentspec.AgentPermissionAllow && result == "":
			result = agentspec.AgentPermissionAllow
		}
	}
	return result
}

func effectiveClassPolicyForDescriptor(desc descriptor.CapabilityDescriptor, policies map[string]agentspec.AgentPermissionLevel) agentspec.AgentPermissionLevel {
	return effectiveClassPolicyForProfile(buildDescriptorProfile(desc), policies)
}

func effectiveClassPolicyForProfile(profile descriptorProfile, policies map[string]agentspec.AgentPermissionLevel) agentspec.AgentPermissionLevel {
	var result agentspec.AgentPermissionLevel
	for _, label := range profile.classLabels {
		level, ok := policies[label]
		if !ok {
			continue
		}
		switch {
		case level == agentspec.AgentPermissionDeny:
			return agentspec.AgentPermissionDeny
		case level == agentspec.AgentPermissionAsk && result != agentspec.AgentPermissionDeny:
			result = agentspec.AgentPermissionAsk
		case level == agentspec.AgentPermissionAllow && result == "":
			result = agentspec.AgentPermissionAllow
		}
	}
	return result
}

// SetModelProfile sets the active model profile for the registry to enable custom tool aliasing.
func (r *CapabilityRegistry) SetModelProfile(p *model.ModelProfile) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelProfile = p
}

// NormalizeToolName resolves potential capability/tool call aliases to canonical names.
func (r *CapabilityRegistry) NormalizeToolName(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.normalizeToolNameLocked(name)
}

func (r *CapabilityRegistry) normalizeToolNameLocked(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	profile := r.modelProfile
	if profile != nil && profile.ToolCalling.Aliases != nil {
		if canonical, exists := profile.ToolCalling.Aliases[name]; exists {
			return canonical
		}
	}
	if canonical, exists := DefaultToolNameNormalization[name]; exists {
		return canonical
	}
	return name
}
