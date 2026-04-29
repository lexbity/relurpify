package families

import (
	"fmt"
	"sort"
)

// KeywordFamilyRegistry holds registered keyword families and their overrides.
type KeywordFamilyRegistry struct {
	families  map[string]KeywordFamily
	overrides map[string]FamilyOverride
}

// NewRegistry creates a new empty family registry.
func NewRegistry() *KeywordFamilyRegistry {
	return &KeywordFamilyRegistry{
		families:  make(map[string]KeywordFamily),
		overrides: make(map[string]FamilyOverride),
	}
}

// Register adds a keyword family to the registry.
// Returns error if ID already exists.
func (r *KeywordFamilyRegistry) Register(family KeywordFamily) error {
	if _, exists := r.families[family.ID]; exists {
		return fmt.Errorf("family %q already registered", family.ID)
	}
	r.families[family.ID] = family
	return nil
}

// Override applies a patch to an existing family without re-registering.
// Returns error if family does not exist.
func (r *KeywordFamilyRegistry) Override(familyID string, patch FamilyOverride) error {
	if _, exists := r.families[familyID]; !exists {
		return fmt.Errorf("family %q not found", familyID)
	}
	r.overrides[familyID] = patch
	return nil
}

// Lookup returns a family by ID, with overrides applied.
func (r *KeywordFamilyRegistry) Lookup(familyID string) (KeywordFamily, bool) {
	base, exists := r.families[familyID]
	if !exists {
		return KeywordFamily{}, false
	}

	// Apply overrides if present
	if override, hasOverride := r.overrides[familyID]; hasOverride {
		result := base
		if len(override.AddKeywords) > 0 {
			result.Keywords = append(result.Keywords, override.AddKeywords...)
		}
		if len(override.RemoveKeywords) > 0 {
			result.Keywords = filterStrings(result.Keywords, override.RemoveKeywords)
		}
		if len(override.AddIntentKeywords) > 0 {
			result.IntentKeywords = append(result.IntentKeywords, override.AddIntentKeywords...)
		}
		if override.ReplaceHITLPolicy != nil {
			result.DefaultHITLPolicy = *override.ReplaceHITLPolicy
		}
		if override.ReplaceVerification != nil {
			result.DefaultVerification = *override.ReplaceVerification
		}
		return result, true
	}

	return base, true
}

// All returns all registered families in a stable order.
func (r *KeywordFamilyRegistry) All() []KeywordFamily {
	ids := make([]string, 0, len(r.families))
	for id := range r.families {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := make([]KeywordFamily, 0, len(ids))
	for _, id := range ids {
		family, _ := r.Lookup(id)
		result = append(result, family)
	}
	return result
}

// SignalWeightsFor returns merged signal weight overrides for a family.
// Returns nil if no overrides are defined.
func (r *KeywordFamilyRegistry) SignalWeightsFor(familyID string) map[string]float64 {
	override, exists := r.overrides[familyID]
	if !exists {
		return nil
	}
	return override.SignalWeights
}

// filterStrings removes items from slice that appear in remove list.
func filterStrings(slice, remove []string) []string {
	removeMap := make(map[string]bool)
	for _, r := range remove {
		removeMap[r] = true
	}

	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if !removeMap[s] {
			result = append(result, s)
		}
	}
	return result
}
