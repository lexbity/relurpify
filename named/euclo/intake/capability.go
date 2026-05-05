package intake

import (
	"context"

	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// CapabilityClassifier maps families to capability sequences.
type CapabilityClassifier interface {
	Classify(ctx context.Context, instruction, familyID, streamedContext string, negativeConstraints []string) ([]string, string, error)
	ClassifyCapability(sel families.FamilySelection, overrides map[string]families.FamilyOverride) ([]string, string)
}

// capabilityClassifierImpl implements CapabilityClassifier.
type capabilityClassifierImpl struct {
	registry *families.KeywordFamilyRegistry
}

// NewCapabilityClassifier creates a new capability classifier.
func NewCapabilityClassifier(registry *families.KeywordFamilyRegistry) CapabilityClassifier {
	return &capabilityClassifierImpl{
		registry: registry,
	}
}

// Classify determines the capability sequence for a family selection.
// It uses the family's CapabilitySequence if available, otherwise falls back to FallbackCapability.
// For mixed intent (multiple families), it returns the sequence from the winning family.
func (c *capabilityClassifierImpl) Classify(ctx context.Context, instruction, familyID, streamedContext string, negativeConstraints []string) ([]string, string, error) {
	seq, source := c.ClassifyCapability(families.FamilySelection{WinningFamily: familyID}, nil)
	return seq, source, nil
}

// ClassifyCapability resolves a family to a capability sequence using registry metadata
// and optional overrides. This preserves the existing family-to-sequence tests while
// sharing the same classifier interface used by the tier-2 pipeline.
func (c *capabilityClassifierImpl) ClassifyCapability(sel families.FamilySelection, overrides map[string]families.FamilyOverride) ([]string, string) {
	if len(overrides) > 0 {
		if override, ok := overrides[sel.WinningFamily]; ok && len(override.CapabilitySequence) > 0 {
			return append([]string(nil), override.CapabilitySequence...), "override"
		}
	}
	if c == nil || c.registry == nil {
		return nil, "registry_unavailable"
	}

	family, ok := c.registry.Lookup(sel.WinningFamily)
	if !ok {
		return nil, "family_not_found"
	}
	if len(family.CapabilitySequence) > 0 {
		return append([]string(nil), family.CapabilitySequence...), "family_metadata"
	}
	if family.FallbackCapability != "" {
		return []string{family.FallbackCapability}, "fallback"
	}
	return nil, "no_capability"
}

// ResolveIntent produces an IntentClassification from a scored classification and task envelope.
// It populates CapabilitySequence, CapabilityOperator, ClassificationSource, MixedIntent,
// EditPermitted, RequiresVerification, Scope, RiskLevel, and ReasonCodes.
func ResolveIntent(classification *ScoredClassification, envelope *TaskEnvelope, registry *families.KeywordFamilyRegistry, overrides map[string]families.FamilyOverride, classificationSource string) *IntentClassification {
	if classification == nil {
		classification = &ScoredClassification{}
	}
	if envelope == nil {
		envelope = &TaskEnvelope{}
	}
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
	var (
		family families.KeywordFamily
		ok     bool
	)
	if registry != nil {
		family, ok = registry.Lookup(classification.WinningFamily)
	}
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
	capabilitySequence, source := classifier.ClassifyCapability(families.FamilySelection{WinningFamily: classification.WinningFamily}, overrides)
	if len(capabilitySequence) == 0 {
		capabilitySequence, source, _ = classifier.Classify(context.Background(), "", classification.WinningFamily, "", nil)
	}
	if classificationSource == "" && source != "" {
		classificationSource = source
	}
	intent.CapabilitySequence = capabilitySequence

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
