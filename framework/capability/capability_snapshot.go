package capability

import (
	"sort"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// CapabilitySnapshot pairs an admitted capability descriptor with its current
// effective exposure. It includes hidden capabilities so callers can inspect
// policy-denied entries without consulting live registry internals.
type CapabilitySnapshot struct {
	Descriptor core.CapabilityDescriptor
	Exposure   core.CapabilityExposure
}

// AllCapabilitySnapshots returns every admitted capability together with its
// current effective exposure, including policy-hidden capabilities.
func (r *CapabilityRegistry) AllCapabilitySnapshots() []CapabilitySnapshot {
	if r == nil {
		return nil
	}
	if r.delegate != nil {
		base := r.delegate.AllCapabilitySnapshots()
		if r.toolIDAllowlist == nil {
			return base
		}
		filtered := make([]CapabilitySnapshot, 0, len(base))
		for _, snapshot := range base {
			if _, ok := r.toolIDAllowlist[snapshot.Descriptor.ID]; ok {
				filtered = append(filtered, snapshot)
			}
		}
		return filtered
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.capabilities))
	for id := range r.capabilities {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	snapshots := make([]CapabilitySnapshot, 0, len(ids))
	for _, id := range ids {
		desc, ok := r.capabilities[id]
		if !ok {
			continue
		}
		snapshots = append(snapshots, CapabilitySnapshot{
			Descriptor: desc,
			Exposure:   r.effectiveExposureLocked(desc),
		})
	}
	return snapshots
}
