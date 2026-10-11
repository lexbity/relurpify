package registry

import (
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
)

// bareCapabilityName derives the bare name of a capability ID (D3). For the
// invocable-capability ID shape "<ns>:cap.<name>" (e.g. "euclo:cap.test_run")
// the bare name is "<name>"; any other ID contributes the suffix after the
// final ':', or the whole ID when it has no colon.
func bareCapabilityName(id string) string {
	bare := id
	if i := strings.LastIndex(id, ":"); i >= 0 {
		bare = id[i+1:]
	}
	if name, ok := strings.CutPrefix(bare, "cap."); ok && name != "" {
		return name
	}
	return bare
}

// CallableCapabilityNames returns the names and IDs of every capability
// exposed as callable. This is the declared-vocabulary surface: recipe load
// validation resolves `may invoke` names against it and suggests nearest
// candidates from it. Deterministically sorted.
func (r *CapabilityRegistry) CallableCapabilityNames() []string {
	if r == nil {
		return nil
	}
	if r.delegate != nil {
		return r.delegate.CallableCapabilityNames()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]struct{}, len(r.capabilities))
	out := make([]string, 0, len(r.capabilities))
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, capability := range r.capabilities {
		if r.effectiveExposureLocked(capability) == agentspec.CapabilityExposureHidden {
			continue
		}
		add(capability.ID)
		add(capability.Name)
	}
	sort.Strings(out)
	return out
}
