package capability

import (
	"context"
	"fmt"
	"sort"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

type NodeSelectionCriteria struct {
	PreferNodeID   string
	PreferPlatform NodePlatform
	RequireOnline  bool
	MaxRiskClass   agentspec.RiskClass
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

// RegisterNodeProvider registers a physical device node as a provider.
func (r *CapabilityRegistry) RegisterNodeProvider(ctx context.Context, provider NodeProvider) error {
	if r == nil {
		return fmt.Errorf("registry unavailable")
	}
	if provider == nil {
		return fmt.Errorf("node provider required")
	}
	desc := provider.Descriptor()
	nodeDesc := provider.NodeDescriptor()
	desc.Kind = ProviderKindNodeDevice
	desc.TrustBaseline = nodeDesc.TrustClass
	if err := desc.Validate(); err != nil {
		return err
	}
	registrar, err := r.ProviderCapabilityRegistrar(desc, agentspec.ProviderPolicy{DefaultTrust: nodeDesc.TrustClass})
	if err != nil {
		return err
	}
	if err := provider.RegisterCapabilities(ctx, registrar); err != nil {
		return err
	}
	r.mu.Lock()
	if r.nodeProviders == nil {
		r.nodeProviders = map[string]NodeProvider{}
	}
	r.nodeProviders[desc.ID] = provider
	r.mu.Unlock()
	return nil
}

// InvokeOnBestNode invokes a capability on the best available node.
func (r *CapabilityRegistry) InvokeOnBestNode(ctx context.Context, capabilityName string, args map[string]any, criteria NodeSelectionCriteria, state *contextdata.Envelope) (*ports.CapabilityExecutionResult, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	candidates := r.nodeCapabilityCandidates(capabilityName, criteria)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no matching node capability found for %s", capabilityName)
	}
	best := candidates[0]
	return r.InvokeCapability(ctx, state, best.descriptor.ID, args)
}

type nodeCapabilityCandidate struct {
	descriptor CapabilityDescriptor
	health     NodeHealth
	score      int
}

func (r *CapabilityRegistry) nodeCapabilityCandidates(capabilityName string, criteria NodeSelectionCriteria) []nodeCapabilityCandidate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var candidates []nodeCapabilityCandidate
	for _, entry := range r.entries {
		if entry == nil || entry.descriptor.Name != capabilityName {
			continue
		}
		if entry.descriptor.Source.ProviderID == "" {
			continue
		}
		provider, ok := r.nodeProviders[entry.descriptor.Source.ProviderID]
		if !ok {
			continue
		}
		if criteria.MaxRiskClass != "" && riskExceeds(criteria.MaxRiskClass, entry.descriptor.RiskClasses) {
			continue
		}
		health, err := provider.NodeHealth(context.Background())
		if err != nil {
			continue
		}
		if criteria.RequireOnline && !health.Online {
			continue
		}
		score := 0
		if criteria.PreferNodeID != "" && provider.NodeDescriptor().ID == criteria.PreferNodeID {
			score += 100
		}
		if criteria.PreferPlatform != "" && provider.NodeDescriptor().Platform == criteria.PreferPlatform {
			score += 10
		}
		if health.Online {
			score += 5
		}
		if health.Foreground {
			score += 1
		}
		candidates = append(candidates, nodeCapabilityCandidate{
			descriptor: entry.descriptor,
			health:     health,
			score:      score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	return candidates
}

func riskExceeds(max agentspec.RiskClass, actual []agentspec.RiskClass) bool {
	if max == "" {
		return false
	}
	limit := riskRank(max)
	for _, risk := range actual {
		if riskRank(risk) > limit {
			return true
		}
	}
	return false
}

func riskRank(risk agentspec.RiskClass) int {
	switch risk {
	case agentspec.RiskClassReadOnly:
		return 1
	case agentspec.RiskClassSessioned:
		return 2
	case agentspec.RiskClassNetwork:
		return 3
	case agentspec.RiskClassExecute:
		return 4
	case agentspec.RiskClassCredentialed:
		return 5
	case agentspec.RiskClassExfiltration:
		return 6
	case agentspec.RiskClassDestructive:
		return 7
	default:
		return 0
	}
}
