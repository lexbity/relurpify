package registry

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
)

// AdmissionResult records whether a capability candidate was admitted.
type AdmissionResult struct {
	CapabilityID   string
	CapabilityName string
	Kind           agentspec.CapabilityKind
	Admitted       bool
	Reason         string
}

// Candidate describes a capability candidate before admission into the
// registry. Callers may source these from skills or any other framework-owned
// contribution mechanism.
type Candidate struct {
	Descriptor      descriptor.CapabilityDescriptor
	PromptHandler   handler.PromptCapabilityHandler
	ResourceHandler handler.ResourceCapabilityHandler
}

// AdmitCandidates admits capability candidates against the final selector set
// and records explicit results.
func AdmitCandidates(registry *CapabilityRegistry, candidates []Candidate, allowed []agentspec.CapabilitySelector) ([]AdmissionResult, error) {
	if registry == nil {
		return nil, fmt.Errorf("capability registry required")
	}
	results := EvaluateCandidates(candidates, allowed)
	items := make([]RegistrationBatchItem, 0, len(candidates))
	for idx, candidate := range candidates {
		if idx >= len(results) || !results[idx].Admitted {
			continue
		}
		desc := descriptor.NormalizeCapabilityDescriptor(candidate.Descriptor)
		item := RegistrationBatchItem{Descriptor: desc}
		switch {
		case candidate.PromptHandler != nil:
			item.PromptHandler = candidate.PromptHandler
		case candidate.ResourceHandler != nil:
			item.ResourceHandler = candidate.ResourceHandler
		case desc.ID != "":
		default:
			results[idx].Admitted = false
			results[idx].Reason = "candidate missing registration handler"
			continue
		}
		items = append(items, item)
	}
	if err := registry.RegisterBatch(items); err != nil {
		for idx, candidate := range candidates {
			if idx >= len(results) || !results[idx].Admitted {
				continue
			}
			desc := descriptor.NormalizeCapabilityDescriptor(candidate.Descriptor)
			if desc.ID == "" && candidate.PromptHandler == nil && candidate.ResourceHandler == nil {
				return results[:idx], err
			}
		}
		return results, err
	}
	return results, nil
}

// EvaluateCandidates evaluates capability candidates against the final selector
// set without mutating the registry.
func EvaluateCandidates(candidates []Candidate, allowed []agentspec.CapabilitySelector) []AdmissionResult {
	results := make([]AdmissionResult, 0, len(candidates))
	for _, candidate := range candidates {
		desc := descriptor.NormalizeCapabilityDescriptor(candidate.Descriptor)
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

func matchesAnySelector(selectors []agentspec.CapabilitySelector, desc descriptor.CapabilityDescriptor) bool {
	if len(selectors) == 0 {
		return true
	}
	for _, selector := range selectors {
		if SelectorMatchesDescriptor(selector, desc) {
			return true
		}
	}
	return false
}
