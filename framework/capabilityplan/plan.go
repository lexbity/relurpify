package capabilityplan

import (
	"fmt"

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

// Candidate describes a capability candidate before admission into the
// registry. Callers may source these from skills or any other framework-owned
// contribution mechanism.
type Candidate struct {
	Descriptor      core.CapabilityDescriptor
	PromptHandler   core.PromptCapabilityHandler
	ResourceHandler core.ResourceCapabilityHandler
}

// AdmitCandidates admits capability candidates against the final selector set
// and records explicit results.
func AdmitCandidates(registry *capability.Registry, candidates []Candidate, allowed []core.CapabilitySelector) ([]AdmissionResult, error) {
	if registry == nil {
		return nil, fmt.Errorf("capability registry required")
	}
	results := EvaluateCandidates(candidates, allowed)
	items := make([]capability.RegistrationBatchItem, 0, len(candidates))
	for idx, candidate := range candidates {
		if idx >= len(results) || !results[idx].Admitted {
			continue
		}
		desc := core.NormalizeCapabilityDescriptor(candidate.Descriptor)
		item := capability.RegistrationBatchItem{Descriptor: desc}
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
			desc := core.NormalizeCapabilityDescriptor(candidate.Descriptor)
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
func EvaluateCandidates(candidates []Candidate, allowed []core.CapabilitySelector) []AdmissionResult {
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
