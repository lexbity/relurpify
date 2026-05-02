package capability

import (
	"fmt"
	"sort"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ExecutionCapabilityCatalogEntry records the effective visibility of one
// admitted capability for a single execution snapshot.
type ExecutionCapabilityCatalogEntry struct {
	Descriptor    core.CapabilityDescriptor
	Exposure      core.CapabilityExposure
	Inspectable   bool
	Callable      bool
	ModelCallable bool
	LocalTool     bool
	localTool     contracts.Tool
}

// ExecutionCapabilityCatalogSnapshot freezes the effective capability catalog
// for one agent execution so callers can reuse a compiled descriptive view
// instead of repeatedly deriving it from live registry state.
//
// Important: this snapshot is descriptive, not authoritative for authorization.
// It is safe to use for stable execution-scoped views such as:
//   - callable/inspectable catalog presentation
//   - model tool lists
//   - provenance descriptor lookup
//   - policy snapshot attachment for reporting
//
// It must not replace live authorization, safety, revocation, or permission
// checks at invocation time. Capability execution decisions still belong to the
// live registry, permission manager, and runtime safety controller.
type ExecutionCapabilityCatalogSnapshot struct {
	ID         string
	CapturedAt time.Time
	AgentID    string

	entries                []ExecutionCapabilityCatalogEntry
	callableCapabilities   []core.CapabilityDescriptor
	inspectableCaps        []core.CapabilityDescriptor
	modelCallableTools     []contracts.Tool
	modelCallableToolSpecs []contracts.LLMToolSpec
	policySnapshot         *core.PolicySnapshot
	allowedCapabilities    []core.CapabilitySelector
}

// CaptureExecutionCatalogSnapshot compiles an execution-scoped descriptive
// capability catalog from the registry's current admitted capabilities and
// effective runtime policy.
func (r *CapabilityRegistry) CaptureExecutionCatalogSnapshot() *ExecutionCapabilityCatalogSnapshot {
	if r == nil {
		return nil
	}
	if r.delegate != nil {
		// Capture from base then filter to the declared allowlist.
		base := r.delegate.CaptureExecutionCatalogSnapshot()
		if base == nil || r.toolIDAllowlist == nil {
			return base
		}
		return base.filteredByAllowlist(r.toolIDAllowlist, r.delegate)
	}
	now := time.Now().UTC()

	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := &ExecutionCapabilityCatalogSnapshot{
		ID:                     fmt.Sprintf("capability-catalog-%d", now.UnixNano()),
		CapturedAt:             now,
		AgentID:                r.registeredAgentID,
		entries:                make([]ExecutionCapabilityCatalogEntry, 0, len(r.entries)),
		callableCapabilities:   make([]core.CapabilityDescriptor, 0, len(r.entries)),
		inspectableCaps:        make([]core.CapabilityDescriptor, 0, len(r.entries)),
		modelCallableTools:     make([]contracts.Tool, 0, len(r.entries)),
		modelCallableToolSpecs: make([]contracts.LLMToolSpec, 0, len(r.entries)),
		policySnapshot:         r.capturePolicySnapshotLocked(now),
		allowedCapabilities:    core.CloneCapabilitySelectors(r.allowedCapabilities),
	}

	ids := make([]string, 0, len(r.entries))
	for id := range r.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		entry := r.entries[id]
		if entry == nil {
			continue
		}
		exposure := r.effectiveExposureLocked(entry.descriptor)
		catalogEntry := ExecutionCapabilityCatalogEntry{
			Descriptor:    entry.descriptor,
			Exposure:      exposure,
			Inspectable:   exposure != core.CapabilityExposureHidden,
			Callable:      exposure == core.CapabilityExposureCallable,
			ModelCallable: exposure == core.CapabilityExposureCallable && (entry.legacyTool != nil || isInvocableCapabilityEntry(entry)),
			LocalTool:     entry.legacyTool != nil,
			localTool:     entry.legacyTool,
		}
		snapshot.entries = append(snapshot.entries, catalogEntry)
		if catalogEntry.Inspectable {
			snapshot.inspectableCaps = append(snapshot.inspectableCaps, entry.descriptor)
		}
		if catalogEntry.Callable {
			snapshot.callableCapabilities = append(snapshot.callableCapabilities, entry.descriptor)
			switch {
			case entry.legacyTool != nil:
				snapshot.modelCallableTools = append(snapshot.modelCallableTools, entry.legacyTool)
				snapshot.modelCallableToolSpecs = append(snapshot.modelCallableToolSpecs, contracts.LLMToolSpecFromTool(unwrapTool(entry.legacyTool)))
			case isInvocableCapabilityEntry(entry):
				snapshot.modelCallableToolSpecs = append(snapshot.modelCallableToolSpecs, core.LLMToolSpecFromDescriptor(entry.descriptor))
			}
		}
	}

	return snapshot
}

func isInvocableCapabilityEntry(entry *capabilityEntry) bool {
	if entry == nil {
		return false
	}
	_, ok := entry.handler.(core.InvocableCapabilityHandler)
	return ok
}

// Entries returns the admitted catalog entries for this execution.
func (s *ExecutionCapabilityCatalogSnapshot) Entries() []ExecutionCapabilityCatalogEntry {
	if s == nil {
		return nil
	}
	return append([]ExecutionCapabilityCatalogEntry(nil), s.entries...)
}

// CallableCapabilities returns the callable capability descriptors for this execution.
func (s *ExecutionCapabilityCatalogSnapshot) CallableCapabilities() []core.CapabilityDescriptor {
	if s == nil {
		return nil
	}
	return append([]core.CapabilityDescriptor(nil), s.callableCapabilities...)
}

// InspectableCapabilities returns the non-hidden capability descriptors for this execution.
func (s *ExecutionCapabilityCatalogSnapshot) InspectableCapabilities() []core.CapabilityDescriptor {
	if s == nil {
		return nil
	}
	return append([]core.CapabilityDescriptor(nil), s.inspectableCaps...)
}

// ModelCallableLLMToolSpecs returns the precompiled LLM tool specs for this execution.
func (s *ExecutionCapabilityCatalogSnapshot) ModelCallableLLMToolSpecs() []contracts.LLMToolSpec {
	if s == nil {
		return nil
	}
	return append([]contracts.LLMToolSpec(nil), s.modelCallableToolSpecs...)
}

// ModelCallableTools returns the callable local tools for this execution.
func (s *ExecutionCapabilityCatalogSnapshot) ModelCallableTools() []contracts.Tool {
	if s == nil {
		return nil
	}
	return append([]contracts.Tool(nil), s.modelCallableTools...)
}

// GetModelTool resolves a callable local tool from the execution snapshot.
func (s *ExecutionCapabilityCatalogSnapshot) GetModelTool(name string) (contracts.Tool, bool) {
	if s == nil {
		return nil, false
	}
	normalized := normalizeComparable(name)
	if normalized == "" {
		return nil, false
	}
	for _, tool := range s.modelCallableTools {
		if tool == nil {
			continue
		}
		if normalizeComparable(tool.Name()) == normalized {
			return tool, true
		}
	}
	return nil, false
}

// GetCapability resolves a capability entry by capability ID or public name.
func (s *ExecutionCapabilityCatalogSnapshot) GetCapability(idOrName string) (ExecutionCapabilityCatalogEntry, bool) {
	if s == nil {
		return ExecutionCapabilityCatalogEntry{}, false
	}
	normalized := normalizeComparable(idOrName)
	if normalized == "" {
		return ExecutionCapabilityCatalogEntry{}, false
	}
	for _, entry := range s.entries {
		if normalizeComparable(entry.Descriptor.ID) == normalized || normalizeComparable(entry.Descriptor.Name) == normalized {
			return entry, true
		}
	}
	return ExecutionCapabilityCatalogEntry{}, false
}

// PolicySnapshot returns a cloned policy snapshot for this execution.
// The returned snapshot is intended for provenance and reporting, not to
// authorize future invocations after runtime policy has changed.
func (s *ExecutionCapabilityCatalogSnapshot) PolicySnapshot() *core.PolicySnapshot {
	if s == nil {
		return nil
	}
	return clonePolicySnapshot(s.policySnapshot)
}

// AllowedCapabilities returns the admitted capability selectors active for this execution.
func (s *ExecutionCapabilityCatalogSnapshot) AllowedCapabilities() []core.CapabilitySelector {
	if s == nil {
		return nil
	}
	return core.CloneCapabilitySelectors(s.allowedCapabilities)
}

// filteredByAllowlist returns a copy of this snapshot with all lists restricted
// to capabilities present in the allowlist map. reg is used to resolve IDs for
// tool entries that only carry a name. Called by CapabilityRegistry.CaptureExecutionCatalogSnapshot
// when the registry is a WithAllowlist-scoped view.
func (s *ExecutionCapabilityCatalogSnapshot) filteredByAllowlist(allowed map[string]struct{}, reg *CapabilityRegistry) *ExecutionCapabilityCatalogSnapshot {
	if s == nil {
		return nil
	}
	out := &ExecutionCapabilityCatalogSnapshot{
		ID:                  s.ID,
		CapturedAt:          s.CapturedAt,
		AgentID:             s.AgentID,
		policySnapshot:      s.policySnapshot,
		allowedCapabilities: s.allowedCapabilities,
	}

	isAllowed := func(id string) bool {
		_, ok := allowed[id]
		return ok
	}

	for _, e := range s.entries {
		if !isAllowed(e.Descriptor.ID) {
			continue
		}
		out.entries = append(out.entries, e)
		if e.Inspectable {
			out.inspectableCaps = append(out.inspectableCaps, e.Descriptor)
		}
		if e.Callable {
			out.callableCapabilities = append(out.callableCapabilities, e.Descriptor)
		}
		if e.ModelCallable {
			if e.LocalTool && e.localTool != nil {
				out.modelCallableTools = append(out.modelCallableTools, e.localTool)
				out.modelCallableToolSpecs = append(out.modelCallableToolSpecs, contracts.LLMToolSpecFromTool(unwrapTool(e.localTool)))
			} else {
				out.modelCallableToolSpecs = append(out.modelCallableToolSpecs, core.LLMToolSpecFromDescriptor(e.Descriptor))
			}
		}
	}

	// Also filter the standalone modelCallableTools list (tools registered via legacy path)
	// that may not have a matching entry. Use reg for ID resolution.
	seen := make(map[string]struct{}, len(out.modelCallableTools))
	for _, t := range out.modelCallableTools {
		if t != nil {
			seen[t.Name()] = struct{}{}
		}
	}
	for _, t := range s.modelCallableTools {
		if t == nil {
			continue
		}
		if _, already := seen[t.Name()]; already {
			continue
		}
		if desc, ok := reg.GetCapability(t.Name()); ok {
			if isAllowed(desc.ID) {
				out.modelCallableTools = append(out.modelCallableTools, t)
				out.modelCallableToolSpecs = append(out.modelCallableToolSpecs, contracts.LLMToolSpecFromTool(unwrapTool(t)))
			}
		}
	}

	return out
}
