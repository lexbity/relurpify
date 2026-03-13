package capabilityplan

import (
	"fmt"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// AdmissionResult records whether a capability candidate was admitted.
type AdmissionResult struct {
	CapabilityID   string
	CapabilityName string
	Kind           core.CapabilityKind
	Admitted       bool
	Reason         string
}

// AdmitSkillCapabilities admits prompt/resource capabilities from resolved
// skills against the final selector set and records explicit results.
func AdmitSkillCapabilities(registry *capability.Registry, resolved []agents.ResolvedSkill, allowed []core.CapabilitySelector) ([]AdmissionResult, error) {
	if registry == nil {
		return nil, fmt.Errorf("capability registry required")
	}
	results := EvaluateSkillCapabilities(resolved, allowed)
	candidates := agents.EnumerateSkillCapabilities(resolved)
	for idx, candidate := range candidates {
		if idx >= len(results) || !results[idx].Admitted {
			continue
		}
		desc := core.NormalizeCapabilityDescriptor(candidate.Descriptor)
		switch {
		case candidate.PromptHandler != nil:
			if err := registry.RegisterPromptCapability(candidate.PromptHandler); err != nil {
				return results[:idx], err
			}
		case candidate.ResourceHandler != nil:
			if err := registry.RegisterResourceCapability(candidate.ResourceHandler); err != nil {
				return results[:idx], err
			}
		case desc.ID != "":
			if err := registry.RegisterCapability(desc); err != nil {
				return results[:idx], err
			}
		default:
			results[idx].Admitted = false
			results[idx].Reason = "candidate missing registration handler"
		}
	}
	return results, nil
}

// EvaluateSkillCapabilities evaluates prompt/resource capabilities from
// resolved skills against the final selector set without mutating the registry.
func EvaluateSkillCapabilities(resolved []agents.ResolvedSkill, allowed []core.CapabilitySelector) []AdmissionResult {
	candidates := agents.EnumerateSkillCapabilities(resolved)
	results := make([]AdmissionResult, 0, len(candidates))
	for _, candidate := range candidates {
		desc := core.NormalizeCapabilityDescriptor(candidate.Descriptor)
		result := AdmissionResult{
			CapabilityID:   desc.ID,
			CapabilityName: desc.Name,
			Kind:           desc.Kind,
		}
		if !matchesAnySelector(allowed, desc) {
			result.Reason = "filtered by allowed capabilities"
			results = append(results, result)
			continue
		}
		switch {
		case candidate.PromptHandler != nil, candidate.ResourceHandler != nil, desc.ID != "":
			result.Admitted = true
			result.Reason = "admitted"
		default:
			result.Reason = "candidate missing registration handler"
		}
		results = append(results, result)
	}
	return results
}

func matchesAnySelector(selectors []core.CapabilitySelector, desc core.CapabilityDescriptor) bool {
	if len(selectors) == 0 {
		return true
	}
	for _, selector := range selectors {
		if core.SelectorMatchesDescriptor(selector, desc) {
			return true
		}
	}
	return false
}
