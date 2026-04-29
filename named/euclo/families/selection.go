package families

import (
	"sort"
)

// SelectionRequest defines a request for family selection.
type SelectionRequest struct {
	Keywords      []string
	IntentKeywords []string
	MaxFamilies   int
}

// SelectionResult contains selected families with scores.
type SelectionResult struct {
	Families []ScoredFamily
}

// ScoredFamily represents a family with its selection score.
type ScoredFamily struct {
	Family KeywordFamily
	Score  float64
}

// SelectFamilies selects the best matching families based on keywords.
func SelectFamilies(registry *KeywordFamilyRegistry, req SelectionRequest) SelectionResult {
	if registry == nil {
		return SelectionResult{}
	}

	allFamilies := registry.All()
	scored := make([]ScoredFamily, 0, len(allFamilies))

	for _, family := range allFamilies {
		score := scoreFamily(family, req.Keywords, req.IntentKeywords)
		if score > 0 {
			scored = append(scored, ScoredFamily{
				Family: family,
				Score:  score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// Limit results
	if req.MaxFamilies > 0 && len(scored) > req.MaxFamilies {
		scored = scored[:req.MaxFamilies]
	}

	return SelectionResult{
		Families: scored,
	}
}

// scoreFamily calculates a match score for a family based on keywords.
func scoreFamily(family KeywordFamily, keywords, intentKeywords []string) float64 {
	score := 0.0

	// Score based on keyword matches
	keywordSet := make(map[string]bool)
	for _, kw := range keywords {
		keywordSet[kw] = true
	}

	for _, kw := range family.Keywords {
		if keywordSet[kw] {
			score += 1.0
		}
	}

	// Bonus for intent keyword matches
	intentSet := make(map[string]bool)
	for _, ik := range intentKeywords {
		intentSet[ik] = true
	}

	for _, ik := range family.IntentKeywords {
		if intentSet[ik] {
			score += 2.0 // Higher weight for intent keywords
		}
	}

	return score
}
