package agentgraph

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

type CapabilityCatalog interface {
	InspectableCapabilities() []descriptor.CapabilityDescriptor
}

type PreflightIssue struct {
	NodeID   string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Code     string `json:"code,omitempty" yaml:"code,omitempty"`
	Message  string `json:"message,omitempty" yaml:"message,omitempty"`
	Blocking bool   `json:"blocking,omitempty" yaml:"blocking,omitempty"`
}

type PlacementDecision struct {
	NodeID                 string                          `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	RequiredSelector       agentspec.CapabilitySelector    `json:"required_selector,omitempty" yaml:"required_selector,omitempty"`
	Preference             PlacementPreference             `json:"preference,omitempty" yaml:"preference,omitempty"`
	SelectedCapabilityID   string                          `json:"selected_capability_id,omitempty" yaml:"selected_capability_id,omitempty"`
	SelectedCapability     descriptor.CapabilityDescriptor `json:"selected_capability,omitempty" yaml:"selected_capability,omitempty"`
	CandidateCapabilityIDs []string                        `json:"candidate_capability_ids,omitempty" yaml:"candidate_capability_ids,omitempty"`
	Reason                 string                          `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type PreflightReport struct {
	GeneratedAt time.Time           `json:"generated_at" yaml:"generated_at"`
	Issues      []PreflightIssue    `json:"issues,omitempty" yaml:"issues,omitempty"`
	Placements  []PlacementDecision `json:"placements,omitempty" yaml:"placements,omitempty"`
}

func (r PreflightReport) HasBlockingIssues() bool {
	for _, issue := range r.Issues {
		if issue.Blocking {
			return true
		}
	}
	return false
}

func (g *Graph) SetCapabilityCatalog(catalog CapabilityCatalog) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.capabilityCatalog = catalog
	g.invalidatePreflightLocked()
}

func (g *Graph) LastPreflightReport() *PreflightReport {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.lastPreflight == nil {
		return nil
	}
	report := *g.lastPreflight
	report.Issues = append([]PreflightIssue(nil), g.lastPreflight.Issues...)
	report.Placements = append([]PlacementDecision(nil), g.lastPreflight.Placements...)
	return &report
}

func (g *Graph) Preflight() (*PreflightReport, error) {
	g.mu.RLock()
	if !g.preflightDirty && g.lastPreflight != nil {
		report := *g.lastPreflight
		report.Issues = append([]PreflightIssue(nil), g.lastPreflight.Issues...)
		report.Placements = append([]PlacementDecision(nil), g.lastPreflight.Placements...)
		err := g.lastPreflightErr
		g.mu.RUnlock()
		return &report, err
	}
	nodes := make([]Node, 0, len(g.nodes))
	contracts := make(map[string]NodeContract, len(g.nodeContracts))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	for id, contract := range g.nodeContracts {
		contracts[id] = contract
	}
	catalog := g.capabilityCatalog
	g.mu.RUnlock()

	report := &PreflightReport{GeneratedAt: time.Now().UTC()}
	var descriptors []descriptor.CapabilityDescriptor
	if catalog != nil {
		descriptors = append(descriptors, catalog.InspectableCapabilities()...)
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID() < nodes[j].ID() })
	sort.SliceStable(descriptors, func(i, j int) bool { return descriptors[i].ID < descriptors[j].ID })

	for _, node := range nodes {
		contract, ok := contracts[node.ID()]
		if !ok {
			contract = ResolveNodeContract(node)
		}
		if catalog == nil {
			continue
		}
		for _, selector := range contract.RequiredCapabilities {
			decision, issues := preflightPlacementDecision(node.ID(), selector, contract, descriptors)
			report.Issues = append(report.Issues, issues...)
			if decision.SelectedCapabilityID != "" {
				report.Placements = append(report.Placements, decision)
			}
		}
	}
	var err error
	if report.HasBlockingIssues() {
		err = blockingPreflightError(report.Issues)
	}
	g.mu.Lock()
	g.lastPreflight = report
	g.lastPreflightErr = err
	g.preflightDirty = false
	g.mu.Unlock()
	return report, err
}

func preflightPlacementDecision(nodeID string, selector agentspec.CapabilitySelector, contract NodeContract, descriptors []descriptor.CapabilityDescriptor) (PlacementDecision, []PreflightIssue) {
	decision := PlacementDecision{
		NodeID:           nodeID,
		RequiredSelector: selector,
		Preference:       contract.PreferredPlacement,
	}
	matches := matchingDescriptors(selector, descriptors)
	if len(matches) == 0 {
		return decision, []PreflightIssue{{
			NodeID:   nodeID,
			Code:     "capability_missing",
			Message:  fmt.Sprintf("no registered capability matches selector %s", selectorString(selector)),
			Blocking: true,
		}}
	}
	filtered := filterDescriptorsForContract(contract, matches)
	if len(filtered) == 0 {
		return decision, []PreflightIssue{{
			NodeID:   nodeID,
			Code:     "capability_constraints_unsatisfied",
			Message:  fmt.Sprintf("matching capabilities for %s do not satisfy trust/risk constraints", selectorString(selector)),
			Blocking: true,
		}}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return placementScore(contract.PreferredPlacement, filtered[i]) > placementScore(contract.PreferredPlacement, filtered[j])
	})
	for _, desc := range filtered {
		decision.CandidateCapabilityIDs = append(decision.CandidateCapabilityIDs, desc.ID)
	}
	decision.SelectedCapability = filtered[0]
	decision.SelectedCapabilityID = filtered[0].ID
	decision.Reason = placementReason(contract.PreferredPlacement, filtered[0])
	return decision, nil
}

func matchingDescriptors(selector agentspec.CapabilitySelector, descriptors []descriptor.CapabilityDescriptor) []descriptor.CapabilityDescriptor {
	out := make([]descriptor.CapabilityDescriptor, 0, len(descriptors))
	for _, desc := range descriptors {
		if registry.SelectorMatchesDescriptor(selector, desc) {
			out = append(out, desc)
		}
	}
	return out
}

func filterDescriptorsForContract(contract NodeContract, descriptors []descriptor.CapabilityDescriptor) []descriptor.CapabilityDescriptor {
	out := make([]descriptor.CapabilityDescriptor, 0, len(descriptors))
	for _, desc := range descriptors {
		if contract.RequiredTrustClass != "" && trustRank(desc.TrustClass) < trustRank(contract.RequiredTrustClass) {
			continue
		}
		if contract.MaxRiskClass != "" && riskExceeds(contract.MaxRiskClass, risk.Classify(desc.EffectClasses, desc.Source.Scope)) {
			continue
		}
		out = append(out, desc)
	}
	return out
}

func placementScore(preference PlacementPreference, desc descriptor.CapabilityDescriptor) int {
	score := trustRank(desc.TrustClass) * 100
	score -= maxRiskRank(risk.Classify(desc.EffectClasses, desc.Source.Scope)) * 10
	switch preference {
	case PlacementPreferenceLocal:
		if desc.Source.ProviderID == "" {
			score += 50
		}
	case PlacementPreferenceRemote:
		if desc.Source.ProviderID != "" {
			score += 50
		}
	case PlacementPreferenceSticky:
		if strings.TrimSpace(desc.SessionAffinity) != "" {
			score += 50
		}
	}
	return score
}

func placementReason(preference PlacementPreference, desc descriptor.CapabilityDescriptor) string {
	switch preference {
	case PlacementPreferenceLocal:
		if desc.Source.ProviderID == "" {
			return "selected highest-trust local capability"
		}
	case PlacementPreferenceRemote:
		if desc.Source.ProviderID != "" {
			return "selected highest-trust remote/provider capability"
		}
	case PlacementPreferenceSticky:
		if desc.SessionAffinity != "" {
			return "selected sticky-session capability with matching affinity"
		}
	}
	return "selected highest-trust lowest-risk matching capability"
}

func selectorString(selector agentspec.CapabilitySelector) string {
	if selector.ID != "" {
		return selector.ID
	}
	if selector.Name != "" {
		return selector.Name
	}
	if selector.Kind != "" {
		return string(selector.Kind)
	}
	return "unknown-selector"
}

func blockingPreflightError(issues []PreflightIssue) error {
	for _, issue := range issues {
		if issue.Blocking {
			return fmt.Errorf("graph preflight failed: %s (%s)", issue.Message, issue.NodeID)
		}
	}
	return nil
}

func riskExceeds(max risk.RiskClass, actual []risk.RiskClass) bool {
	if max == "" {
		return false
	}
	limit := riskRank(max)
	for _, rc := range actual {
		if riskRank(rc) > limit {
			return true
		}
	}
	return false
}

func maxRiskRank(risks []risk.RiskClass) int {
	max := 0
	for _, rc := range risks {
		if rank := riskRank(rc); rank > max {
			max = rank
		}
	}
	return max
}

func riskRank(r risk.RiskClass) int {
	switch r {
	case risk.RiskClassReadOnly:
		return 1
	case risk.RiskClassSessioned:
		return 2
	case risk.RiskClassNetwork:
		return 3
	case risk.RiskClassExecute:
		return 4
	case risk.RiskClassCredentialed:
		return 5
	case risk.RiskClassExfiltration:
		return 6
	case risk.RiskClassDestructive:
		return 7
	default:
		return 0
	}
}

func trustRank(trust agentspec.TrustClass) int {
	switch trust {
	case agentspec.TrustClassRemoteDeclared:
		return 1
	case agentspec.TrustClassProviderLocalUntrusted:
		return 2
	case agentspec.TrustClassRemoteApproved:
		return 3
	case agentspec.TrustClassWorkspaceTrusted:
		return 4
	case agentspec.TrustClassBuiltinTrusted:
		return 5
	default:
		return 0
	}
}
