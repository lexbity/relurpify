package capabilities

import "strings"

// MatchByKeywords matches capabilities by keywords in the instruction.
// Returns a list of candidates sorted by score.
func MatchByKeywords(instruction string, familyID string, registry *EucloCapabilityRegistry, negativeConstraints []string) []CapabilityCandidate {
	if registry == nil {
		return nil
	}
	return registry.MatchByKeywords(instruction, familyID, negativeConstraints)
}

// FallbackForFamily returns the fallback capability for a family.
func FallbackForFamily(familyID string, registry *EucloCapabilityRegistry) (string, bool) {
	if registry != nil {
		if desc, ok := registry.FallbackForFamily(familyID); ok {
			return desc.ID, true
		}
	}

	switch familyID {
	case "debug":
		return "euclo:cap.bisect", true
	case "repair":
		return "euclo:cap.targeted_refactor", true
	case "review":
		return "euclo:cap.code_review", true
	default:
		return "", false
	}
}

// scoreCapabilityByKeywords scores a capability based on keyword matching.
func scoreCapabilityByKeywords(capabilityID, instruction string, negativeConstraints []string) float64 {
	keywords := extractKeywordsFromCapabilityID(capabilityID)

	score := 0.0
	for _, kw := range keywords {
		if strings.Contains(instruction, strings.ToLower(kw)) {
			score += 1.0
		}
	}

	for _, constraint := range negativeConstraints {
		if strings.Contains(instruction, strings.ToLower(constraint)) {
			score *= 0.5
		}
	}

	return score
}

// extractKeywordsFromCapabilityID extracts keywords from a capability ID.
func extractKeywordsFromCapabilityID(capabilityID string) []string {
	parts := strings.Split(capabilityID, ":")
	if len(parts) < 2 {
		return []string{}
	}
	capPart := strings.TrimPrefix(parts[1], "cap.")
	return strings.Split(capPart, "_")
}
