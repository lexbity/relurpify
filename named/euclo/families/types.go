package families

// HITLPolicy defines when human-in-the-loop is required for a family.
type HITLPolicy string

const (
	HITLPolicyNever  HITLPolicy = "never"
	HITLPolicyAsk    HITLPolicy = "ask"
	HITLPolicyAlways HITLPolicy = "always"
)

// VerificationReq defines whether verification is required for a family.
type VerificationReq string

const (
	VerificationRequired     VerificationReq = "required"
	VerificationNotRequired  VerificationReq = "not_required"
	VerificationConfigurable VerificationReq = "configurable"
)

// KeywordFamily represents a keyword family for task classification.
type KeywordFamily struct {
	ID                  string
	DisplayName         string
	Keywords            []string
	IntentKeywords      []string
	DefaultHITLPolicy   HITLPolicy
	DefaultVerification VerificationReq
	RetrievalTemplate   string
	FallbackCapability  string
	DefaultBackground   bool
	CapabilitySequence  []string // Tier-2: ordered list of capability IDs
}

// FamilyOverride allows project-specific customization of a family.
type FamilyOverride struct {
	AddKeywords         []string
	RemoveKeywords      []string
	AddIntentKeywords   []string
	ReplaceHITLPolicy   *HITLPolicy
	ReplaceVerification *VerificationReq
	CapabilitySequence  []string // Override for capability sequence
	// SignalWeights maps signal kind names to multipliers applied during scoring.
	// Keys are signal kind names: "keyword:debug", "error_text", "task_structure", etc.
	// Values are multipliers (1.0 = unchanged, 2.0 = double weight, 0.0 = suppress).
	SignalWeights map[string]float64
}

// FamilyCandidate represents a family with its score during classification.
type FamilyCandidate struct {
	FamilyID string
	Score    float64
}

// FamilySelection represents the selected family with candidates.
type FamilySelection struct {
	WinningFamily string
	Candidates    []FamilyCandidate
	Confidence    float64
	Ambiguous     bool
}
