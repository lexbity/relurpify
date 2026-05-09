package intake

import (
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// ResolveIntent derives the prompt-facing intent classification state from a scored classification.
// It keeps the result focused on family, evidence, and interpretation metadata rather than
// capability-sequence invention.
func ResolveIntent(classification *ScoredClassification, envelope *TaskEnvelope, registry *families.KeywordFamilyRegistry, classificationSource string) *IntentClassification {
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

	intent.MixedIntent = len(classification.FamilyCandidates) > 1

	var (
		family families.KeywordFamily
		ok     bool
	)
	if registry != nil {
		family, ok = registry.Lookup(classification.WinningFamily)
	}
	if !ok {
		intent.EditPermitted = true
		intent.RequiresVerification = false
		intent.RiskLevel = "unknown"
	} else {
		intent.EditPermitted = family.DefaultHITLPolicy != families.HITLPolicyAlways
		intent.RequiresVerification = family.DefaultVerification == families.VerificationRequired
		intent.RiskLevel = getRiskLevelForFamily(family.ID)
	}

	if len(envelope.WorkspaceScopes) > 0 {
		intent.Scope = strings.TrimSpace(envelope.WorkspaceScopes[0])
	} else {
		intent.Scope = "workspace"
	}

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
	codes := []string{"source:" + classificationSource}

	if classification.Ambiguous {
		codes = append(codes, "ambiguous")
	} else {
		codes = append(codes, "confident")
	}
	if envelope.FamilyHint != "" {
		codes = append(codes, "family_hint:"+envelope.FamilyHint)
	}
	if len(classification.NegativeConstraints) > 0 {
		codes = append(codes, "negative_constraints")
	}
	if len(envelope.SessionPins) > 0 {
		codes = append(codes, "session_pinned")
	}
	if envelope.ExplicitVerification {
		codes = append(codes, "explicit_verification")
	}
	return codes
}
