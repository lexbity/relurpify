package capabilities

// CapabilityFamily groups related capabilities.
type CapabilityFamily struct {
	ID                 string   // Family identifier
	Name               string   // Human-readable name
	Description        string   // Description of the family
	CapabilityIDs      []string // Capabilities in this family
	FallbackCapability string   // Preferred fallback capability for the family
}

// CapabilitySelection represents a selected capability with metadata.
type CapabilitySelection struct {
	CapabilityID string  // Selected capability ID
	FamilyID     string  // Family the capability belongs to
	MatchReason  string  // Why this capability was selected
	Confidence   float64 // Confidence score (0-1)
}

// CapabilityCandidate represents a capability being considered for selection.
type CapabilityCandidate struct {
	CapabilityID string
	FamilyID     string
	Score        float64
}
