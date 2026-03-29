package capabilities

import (
	"sort"
	"strings"
	"sync"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	debugcaps "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/debug"
	localcaps "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/local"
)

type EucloCapabilityRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]euclotypes.EucloCodingCapability
}

func NewEucloCapabilityRegistry() *EucloCapabilityRegistry {
	return &EucloCapabilityRegistry{capabilities: map[string]euclotypes.EucloCodingCapability{}}
}

func NewDefaultCapabilityRegistry(env agentenv.AgentEnvironment) *EucloCapabilityRegistry {
	reg := NewEucloCapabilityRegistry()
	_ = reg.Register(debugcaps.NewInvestigateRegressionCapability(env))
	_ = reg.Register(localcaps.NewDesignAlternativesCapability(env))
	_ = reg.Register(localcaps.NewExecutionProfileSelectCapability(env))
	_ = reg.Register(localcaps.NewTraceAnalyzeCapability(env))
	_ = reg.Register(localcaps.NewDiffSummaryCapability(env))
	_ = reg.Register(localcaps.NewTraceToRootCauseCapability(env))
	_ = reg.Register(localcaps.NewVerificationSummaryCapability(env))
	_ = reg.Register(localcaps.NewMigrationExecuteCapability(env))
	_ = reg.Register(localcaps.NewReviewFindingsCapability(env))
	_ = reg.Register(localcaps.NewReviewCompatibilityCapability(env))
	_ = reg.Register(localcaps.NewReviewImplementIfSafeCapability(env))
	_ = reg.Register(localcaps.NewRefactorAPICompatibleCapability(env))
	return reg
}

func (r *EucloCapabilityRegistry) Register(cap euclotypes.EucloCodingCapability) error {
	if r == nil || cap == nil {
		return nil
	}
	id := strings.TrimSpace(cap.Descriptor().ID)
	if id == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[id] = cap
	return nil
}

func (r *EucloCapabilityRegistry) Lookup(id string) (euclotypes.EucloCodingCapability, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	cap, ok := r.capabilities[strings.TrimSpace(id)]
	return cap, ok
}

func (r *EucloCapabilityRegistry) ForProfile(profileID string) []euclotypes.EucloCodingCapability {
	if r == nil {
		return nil
	}
	profileID = strings.TrimSpace(profileID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]euclotypes.EucloCodingCapability, 0, len(r.capabilities))
	for _, cap := range r.capabilities {
		if profileID == "" || supportsProfile(cap, profileID) {
			out = append(out, cap)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Descriptor().ID < out[j].Descriptor().ID
	})
	return out
}

func supportsProfile(cap euclotypes.EucloCodingCapability, profileID string) bool {
	if cap == nil || profileID == "" {
		return true
	}
	annotations := cap.Descriptor().Annotations
	if annotations == nil {
		return true
	}
	raw, ok := annotations["supported_profiles"]
	if !ok || raw == nil {
		return true
	}
	switch typed := raw.(type) {
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) == profileID {
				return true
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) == profileID {
				return true
			}
		}
		return false
	default:
		return true
	}
}
