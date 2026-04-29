package intake

import (
	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// CapabilityClassifier maps families to capability sequences.
type CapabilityClassifier struct {
	registry *families.KeywordFamilyRegistry
}

// NewCapabilityClassifier creates a new capability classifier.
func NewCapabilityClassifier(registry *families.KeywordFamilyRegistry) *CapabilityClassifier {
	return &CapabilityClassifier{
		registry: registry,
	}
}

// ClassifyCapability determines the capability sequence for a family selection.
// It uses the family's CapabilitySequence if available, otherwise falls back to FallbackCapability.
// For mixed intent (multiple families), it returns the sequence from the winning family.
func (c *CapabilityClassifier) ClassifyCapability(sel families.FamilySelection, overrides map[string]families.FamilyOverride) ([]string, string) {
	// Check for override first
	if override, ok := overrides[sel.WinningFamily]; ok && len(override.CapabilitySequence) > 0 {
		return override.CapabilitySequence, "override"
	}

	// Get the family to retrieve its capability sequence
	family, ok := c.registry.Lookup(sel.WinningFamily)
	if !ok {
		// Family not found, return empty
		return nil, "family_not_found"
	}

	// Use capability sequence if available
	if len(family.CapabilitySequence) > 0 {
		return family.CapabilitySequence, "family_metadata"
	}

	// Fall back to fallback capability
	if family.FallbackCapability != "" {
		return []string{family.FallbackCapability}, "fallback"
	}

	// No capability available
	return nil, "no_capability"
}

// ResolveIntent produces an IntentClassification from a scored classification and task envelope.
// It populates CapabilitySequence, CapabilityOperator, ClassificationSource, MixedIntent,
// EditPermitted, RequiresVerification, Scope, RiskLevel, and ReasonCodes.
func ResolveIntent(classification *ScoredClassification, envelope *TaskEnvelope, registry *families.KeywordFamilyRegistry, overrides map[string]families.FamilyOverride, classificationSource string) *IntentClassification {
	intent := &IntentClassification{
		WinningFamily:        classification.WinningFamily,
		FamilyCandidates:     classification.FamilyCandidates,
		Confidence:           classification.Confidence,
		Ambiguous:            classification.Ambiguous,
		Signals:              classification.Signals,
		NegativeConstraints:  classification.NegativeConstraints,
		ClassificationSource: classificationSource,
	}

	// Determine mixed intent and capability operator
	if len(classification.FamilyCandidates) > 1 {
		intent.MixedIntent = true
		intent.CapabilityOperator = "any"
	} else {
		intent.MixedIntent = false
		intent.CapabilityOperator = "all"
	}

	// Get the family to determine edit permission, verification, risk level
	family, ok := registry.Lookup(classification.WinningFamily)
	if !ok {
		// Family not found, use defaults
		intent.EditPermitted = true
		intent.RequiresVerification = false
		intent.RiskLevel = "unknown"
	} else {
		// Edit permission based on HITL policy
		intent.EditPermitted = (family.DefaultHITLPolicy != families.HITLPolicyAlways)

		// Verification requirement
		intent.RequiresVerification = (family.DefaultVerification == families.VerificationRequired)

		// Risk level based on family
		intent.RiskLevel = getRiskLevelForFamily(family.ID)
	}

	// Scope from envelope
	if len(envelope.WorkspaceScopes) > 0 {
		intent.Scope = envelope.WorkspaceScopes[0] // Use first scope for now
	} else {
		intent.Scope = "workspace"
	}

	// Capability sequence using classifier
	classifier := NewCapabilityClassifier(registry)
	intent.CapabilitySequence, _ = classifier.ClassifyCapability(families.FamilySelection{
		WinningFamily: classification.WinningFamily,
	}, overrides)

	// Reason codes
	intent.ReasonCodes = generateReasonCodes(classification, envelope, classificationSource)

	return intent
}

// getRiskLevelForFamily returns the risk level for a family.
func getRiskLevelForFamily(familyID string) string {
	switch familyID {
	case families.FamilyDebug:
		return "low"
	case families.FamilyReview:
		return "low"
	case families.FamilyInvestigation:
		return "low"
	case families.FamilyPlanning:
		return "medium"
	case families.FamilyImplementation:
		return "medium"
	case families.FamilyRefactor:
		return "medium"
	case families.FamilyRepair:
		return "high"
	case families.FamilyMigration:
		return "high"
	case families.FamilyArchitecture:
		return "high"
	default:
		return "unknown"
	}
}

// generateReasonCodes generates reason codes for the classification.
func generateReasonCodes(classification *ScoredClassification, envelope *TaskEnvelope, classificationSource string) []string {
	codes := []string{}

	// Classification source
	codes = append(codes, "source:"+classificationSource)

	// Ambiguity
	if classification.Ambiguous {
		codes = append(codes, "ambiguous")
	} else {
		codes = append(codes, "confident")
	}

	// Family hint
	if envelope.FamilyHint != "" {
		codes = append(codes, "family_hint:"+envelope.FamilyHint)
	}

	// Negative constraints
	if len(classification.NegativeConstraints) > 0 {
		codes = append(codes, "negative_constraints")
	}

	// Session pins
	if len(envelope.SessionPins) > 0 {
		codes = append(codes, "session_pinned")
	}

	// Explicit verification
	if envelope.ExplicitVerification {
		codes = append(codes, "explicit_verification")
	}

	return codes
}
