package capabilities

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// EucloCapabilityRegistry holds concrete EucloCodingCapability implementations.
// It provides Euclo's private view over coding capabilities — eligibility checks,
// contract inspection, and profile-scoped filtering — while the framework
// capability registry handles policy, safety, and exposure.
type EucloCapabilityRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]euclotypes.EucloCodingCapability
}

// NewEucloCapabilityRegistry creates an empty capability registry.
func NewEucloCapabilityRegistry() *EucloCapabilityRegistry {
	return &EucloCapabilityRegistry{
		capabilities: make(map[string]euclotypes.EucloCodingCapability),
	}
}

// Register adds a coding capability keyed by its descriptor ID.
func (r *EucloCapabilityRegistry) Register(cap euclotypes.EucloCodingCapability) error {
	if cap == nil {
		return fmt.Errorf("capability must not be nil")
	}
	id := strings.TrimSpace(cap.Descriptor().ID)
	if id == "" {
		return fmt.Errorf("capability descriptor ID must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[id] = cap
	return nil
}

// Lookup returns the capability with the given ID.
func (r *EucloCapabilityRegistry) Lookup(id string) (euclotypes.EucloCodingCapability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cap, ok := r.capabilities[strings.TrimSpace(id)]
	return cap, ok
}

// List returns all registered capabilities in sorted order by ID.
func (r *EucloCapabilityRegistry) List() []euclotypes.EucloCodingCapability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.capabilities))
	for k := range r.capabilities {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]euclotypes.EucloCodingCapability, 0, len(keys))
	for _, k := range keys {
		out = append(out, r.capabilities[k])
	}
	return out
}

// EligibleFor returns capabilities whose Eligible check passes for the
// given artifact state and capability snapshot.
func (r *EucloCapabilityRegistry) EligibleFor(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) []euclotypes.EucloCodingCapability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.capabilities))
	for k := range r.capabilities {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []euclotypes.EucloCodingCapability
	for _, k := range keys {
		cap := r.capabilities[k]
		if result := cap.Eligible(artifacts, snapshot); result.Eligible {
			out = append(out, cap)
		}
	}
	return out
}

// ForProfile returns capabilities whose descriptor annotations include the
// given profile ID in their "supported_profiles" list.
func (r *EucloCapabilityRegistry) ForProfile(profileID string) []euclotypes.EucloCodingCapability {
	profileID = strings.TrimSpace(strings.ToLower(profileID))
	if profileID == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.capabilities))
	for k := range r.capabilities {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []euclotypes.EucloCodingCapability
	for _, k := range keys {
		cap := r.capabilities[k]
		if capSupportsProfile(cap, profileID) {
			out = append(out, cap)
		}
	}
	return out
}

// capSupportsProfile checks if a capability's annotations include the profile.
func capSupportsProfile(cap euclotypes.EucloCodingCapability, profileID string) bool {
	annotations := cap.Descriptor().Annotations
	if annotations == nil {
		return false
	}
	raw, ok := annotations["supported_profiles"]
	if !ok {
		return false
	}
	switch typed := raw.(type) {
	case []string:
		for _, p := range typed {
			if strings.TrimSpace(strings.ToLower(p)) == profileID {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(strings.ToLower(s)) == profileID {
				return true
			}
		}
	case string:
		for _, p := range strings.Split(typed, ",") {
			if strings.TrimSpace(strings.ToLower(p)) == profileID {
				return true
			}
		}
	}
	return false
}
