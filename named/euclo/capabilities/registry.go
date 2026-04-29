package capabilities

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// EucloCapabilityRegistry holds registered relurpic capabilities.
type EucloCapabilityRegistry struct {
	mu           sync.RWMutex
	families     map[string]CapabilityFamily
	capabilities map[string]core.CapabilityDescriptor
	byFamily     map[string][]string
}

// NewRegistry creates a new empty capability registry.
func NewRegistry() *EucloCapabilityRegistry {
	r := &EucloCapabilityRegistry{
		families:     make(map[string]CapabilityFamily),
		capabilities: make(map[string]core.CapabilityDescriptor),
		byFamily:     make(map[string][]string),
	}
	r.loadBuiltins()
	return r
}

// RegisterFamily stores a capability family definition.
func (r *EucloCapabilityRegistry) RegisterFamily(family CapabilityFamily) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	if strings.TrimSpace(family.ID) == "" {
		return fmt.Errorf("family id required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.families[family.ID]; exists {
		return fmt.Errorf("family %q already registered", family.ID)
	}
	r.families[family.ID] = family
	return nil
}

// RegisterCapability stores a capability descriptor and binds it to a family.
func (r *EucloCapabilityRegistry) RegisterCapability(desc core.CapabilityDescriptor, family string) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}
	if strings.TrimSpace(desc.ID) == "" {
		return fmt.Errorf("capability id required")
	}
	if strings.TrimSpace(family) == "" {
		return fmt.Errorf("family id required")
	}

	desc = core.NormalizeCapabilityDescriptor(desc)
	if desc.RuntimeFamily == "" {
		desc.RuntimeFamily = core.CapabilityRuntimeFamilyRelurpic
	}
	if desc.Kind == "" {
		desc.Kind = core.CapabilityKindTool
	}
	if desc.Name == "" {
		desc.Name = desc.ID
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.capabilities[desc.ID]; exists {
		return fmt.Errorf("capability %q already registered", desc.ID)
	}
	r.capabilities[desc.ID] = desc
	r.byFamily[family] = appendUniqueString(r.byFamily[family], desc.ID)
	return nil
}

// Select returns a capability descriptor by ID.
func (r *EucloCapabilityRegistry) Select(capabilityID string) (core.CapabilityDescriptor, bool) {
	if r == nil {
		return core.CapabilityDescriptor{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.capabilities[capabilityID]
	return desc, ok
}

// FallbackForFamily returns the family's preferred fallback capability.
func (r *EucloCapabilityRegistry) FallbackForFamily(familyID string) (core.CapabilityDescriptor, bool) {
	if r == nil {
		return core.CapabilityDescriptor{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	if family, ok := r.families[familyID]; ok {
		if fallback := strings.TrimSpace(defaultFamilyFallback(familyID, family)); fallback != "" {
			if desc, ok := r.capabilities[fallback]; ok {
				return desc, true
			}
		}
	}

	if ids := r.byFamily[familyID]; len(ids) > 0 {
		if desc, ok := r.capabilities[ids[0]]; ok {
			return desc, true
		}
	}
	return core.CapabilityDescriptor{}, false
}

// MatchByKeywords scores capabilities in a family using keyword overlap.
func (r *EucloCapabilityRegistry) MatchByKeywords(instruction string, familyID string, negativeConstraints []string) []CapabilityCandidate {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	ids := append([]string(nil), r.byFamily[familyID]...)
	r.mu.RUnlock()

	if len(ids) == 0 {
		return nil
	}

	lowerInstruction := strings.ToLower(instruction)
	candidates := make([]CapabilityCandidate, 0, len(ids))
	for _, capID := range ids {
		score := scoreCapabilityByKeywords(capID, lowerInstruction, negativeConstraints)
		if score <= 0 {
			continue
		}
		candidates = append(candidates, CapabilityCandidate{
			CapabilityID: capID,
			FamilyID:     familyID,
			Score:        score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].CapabilityID < candidates[j].CapabilityID
		}
		return candidates[i].Score > candidates[j].Score
	})
	return candidates
}

// ListCapabilities returns all descriptors in stable ID order.
func (r *EucloCapabilityRegistry) ListCapabilities() []core.CapabilityDescriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.capabilities))
	for id := range r.capabilities {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]core.CapabilityDescriptor, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.capabilities[id])
	}
	return out
}

func appendUniqueString(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	out := make([]string, 0, len(existing)+len(values))
	for _, item := range existing {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	for _, item := range values {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (r *EucloCapabilityRegistry) loadBuiltins() {
	for _, family := range GetBuiltinFamilies() {
		r.families[family.ID] = family
		for _, capID := range family.CapabilityIDs {
			if _, exists := r.capabilities[capID]; exists {
				continue
			}
			r.capabilities[capID] = core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
				ID:            capID,
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
				Name:          capabilityDisplayName(capID),
				Description:   family.Name + " capability",
				Category:      family.ID,
				Tags:          []string{family.ID},
				Availability:  core.AvailabilitySpec{Available: true},
			})
			r.byFamily[family.ID] = appendUniqueString(r.byFamily[family.ID], capID)
		}
	}
}

func capabilityDisplayName(capID string) string {
	capID = strings.TrimSpace(capID)
	if capID == "" {
		return ""
	}
	capID = strings.TrimPrefix(capID, "euclo:cap.")
	capID = strings.ReplaceAll(capID, "_", " ")
	if capID == "" {
		return ""
	}
	return strings.ToUpper(capID[:1]) + capID[1:]
}

func defaultFamilyFallback(familyID string, family CapabilityFamily) string {
	if family.FallbackCapability != "" {
		return family.FallbackCapability
	}
	switch familyID {
	case FamilyCodeUnderstanding:
		return "euclo:cap.ast_query"
	case FamilyRefactorPatch:
		return "euclo:cap.targeted_refactor"
	case FamilyVerification:
		return "euclo:cap.test_run"
	case FamilyRegressionLocalization:
		return "euclo:cap.bisect"
	case FamilyMigrationCompat:
		return "euclo:cap.api_compat"
	case FamilyReviewSynthesis:
		return "euclo:cap.code_review"
	case FamilyArchitecture:
		return "euclo:cap.layer_check"
	default:
		return ""
	}
}
