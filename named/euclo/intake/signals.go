package intake

import (
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// SignalKind represents the type of classification signal.
type SignalKind string

const (
	SignalKindKeyword       SignalKind = "keyword"
	SignalKindTaskStructure SignalKind = "task_structure"
	SignalKindErrorText     SignalKind = "error_text"
	SignalKindContextHint   SignalKind = "context_hint"
	SignalKindUserRecipe    SignalKind = "user_recipe"
	SignalKindNegative      SignalKind = "negative"
	SignalKindDefault       SignalKind = "default"
)

// ClassificationSignal represents a detected signal for family classification.
type ClassificationSignal struct {
	Kind     SignalKind
	FamilyID string
	Weight   float64
	Source   string
}

// Signal weight constants
const (
	WeightKeyword       = 1.0
	WeightTaskStructure = 0.8
	WeightErrorText     = 1.5
	WeightContextHint   = 2.0
	WeightUserRecipe    = 1.8
	WeightNegative      = 0.0 // Negative signals don't contribute to scoring
	WeightDefault       = 0.5
)

// Error text patterns
var errorPatterns = []string{
	"panic:", "error:", "undefined:", "nil pointer", "index out of range",
	"test failed", "compilation error", "type error", "syntax error",
	"broken", "crash", "exception", "fatal",
}

// CollectSignals scans the instruction and context for classification signals.
func CollectSignals(envelope *TaskEnvelope, recipeKeywords map[string][]string, registry *families.KeywordFamilyRegistry) []ClassificationSignal {
	var signals []ClassificationSignal
	lowerInstruction := strings.ToLower(envelope.Instruction)

	// 1. Keyword signals - match against family keywords
	if registry != nil {
		for _, family := range registry.All() {
			for _, kw := range family.Keywords {
				if strings.Contains(lowerInstruction, strings.ToLower(kw)) {
					signals = append(signals, ClassificationSignal{
						Kind:     SignalKindKeyword,
						FamilyID: family.ID,
						Weight:   WeightKeyword,
						Source:   "keyword:" + kw,
					})
				}
			}
		}
	}

	// 2. Error text signals
	for _, pattern := range errorPatterns {
		if strings.Contains(lowerInstruction, pattern) {
			signals = append(signals, ClassificationSignal{
				Kind:     SignalKindErrorText,
				FamilyID: families.FamilyDebug, // Error text strongly suggests debug
				Weight:   WeightErrorText,
				Source:   "error_text:" + pattern,
			})
			break // Only add one error signal
		}
	}

	// 3. Context hint signal
	if envelope.FamilyHint != "" {
		signals = append(signals, ClassificationSignal{
			Kind:     SignalKindContextHint,
			FamilyID: envelope.FamilyHint,
			Weight:   WeightContextHint,
			Source:   "context_hint",
		})
	}

	// 4. User recipe signal
	for familyID, keywords := range recipeKeywords {
		for _, kw := range keywords {
			if strings.Contains(lowerInstruction, strings.ToLower(kw)) {
				signals = append(signals, ClassificationSignal{
					Kind:     SignalKindUserRecipe,
					FamilyID: familyID,
					Weight:   WeightUserRecipe,
					Source:   "user_recipe:" + kw,
				})
				break
			}
		}
	}

	// 5. Negative signals (collected separately, not scored)
	if len(envelope.NegativeConstraintSeeds) > 0 {
		signals = append(signals, ClassificationSignal{
			Kind:     SignalKindNegative,
			FamilyID: "",
			Weight:   WeightNegative,
			Source:   "negative_constraints",
		})
	}

	return signals
}

// ScoreSignals sums weights per family and returns ranked candidates.
func ScoreSignals(signals []ClassificationSignal, familyList []families.KeywordFamily, weightOverrides map[string]float64) []families.FamilyCandidate {
	scores := make(map[string]float64)

	// Build keyword lookup for signal matching
	keywordToFamily := make(map[string]string)
	for _, family := range familyList {
		for _, kw := range family.Keywords {
			keywordToFamily[strings.ToLower(kw)] = family.ID
		}
	}

	// Process keyword signals from instruction
	// Note: We need the instruction text here, which should be passed separately
	// For now, we'll score based on the signals passed in

	for _, signal := range signals {
		if signal.Kind == SignalKindNegative {
			continue // Negative signals don't contribute to scoring
		}

		// Apply weight overrides if present
		weight := signal.Weight
		if weightOverrides != nil {
			overrideKey := string(signal.Kind) + ":" + signal.FamilyID
			if override, ok := weightOverrides[overrideKey]; ok {
				weight *= override
			}
		}

		scores[signal.FamilyID] += weight
	}

	// Build candidates
	var candidates []families.FamilyCandidate
	for familyID, score := range scores {
		candidates = append(candidates, families.FamilyCandidate{
			FamilyID: familyID,
			Score:    score,
		})
	}

	// Sort by score descending
	sortCandidates(candidates)

	return candidates
}

// sortCandidates sorts candidates by score descending.
func sortCandidates(candidates []families.FamilyCandidate) {
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Score > candidates[i].Score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
}

// ScoredClassification represents the result of tier-1 classification.
type ScoredClassification struct {
	WinningFamily       string
	FamilyCandidates    []families.FamilyCandidate
	Confidence          float64
	Ambiguous           bool
	Signals             []ClassificationSignal
	NegativeConstraints []string
}

// ClassifyTaskScored performs tier-1 classification using signal scoring.
func ClassifyTaskScored(envelope *TaskEnvelope, registry *families.KeywordFamilyRegistry, recipeKeywords map[string][]string) *ScoredClassification {
	// Collect signals
	signals := CollectSignals(envelope, recipeKeywords, registry)

	// Score signals
	allFamilies := registry.All()
	candidates := ScoreSignals(signals, allFamilies, nil)

	// If no candidates, inject default baseline
	if len(candidates) == 0 {
		candidates = []families.FamilyCandidate{
			{FamilyID: families.FamilyImplementation, Score: WeightDefault},
		}
		signals = append(signals, ClassificationSignal{
			Kind:     SignalKindDefault,
			FamilyID: families.FamilyImplementation,
			Weight:   WeightDefault,
			Source:   "default_baseline",
		})
	}

	// Calculate confidence and ambiguity
	confidence, ambiguous := calculateConfidence(candidates)

	return &ScoredClassification{
		WinningFamily:       candidates[0].FamilyID,
		FamilyCandidates:    candidates,
		Confidence:          confidence,
		Ambiguous:           ambiguous,
		Signals:             signals,
		NegativeConstraints: envelope.NegativeConstraintSeeds,
	}
}

// collectKeywordSignals extracts keyword signals from instruction text.
func collectKeywordSignals(instruction string, registry *families.KeywordFamilyRegistry) []ClassificationSignal {
	var signals []ClassificationSignal
	lowerInstruction := strings.ToLower(instruction)

	for _, family := range registry.All() {
		for _, kw := range family.Keywords {
			if strings.Contains(lowerInstruction, strings.ToLower(kw)) {
				signals = append(signals, ClassificationSignal{
					Kind:     SignalKindKeyword,
					FamilyID: family.ID,
					Weight:   WeightKeyword,
					Source:   "keyword:" + kw,
				})
			}
		}
	}

	return signals
}

// calculateConfidence computes confidence and ambiguity from candidates.
func calculateConfidence(candidates []families.FamilyCandidate) (float64, bool) {
	if len(candidates) == 0 {
		return 0.0, true
	}

	if len(candidates) == 1 {
		return 1.0, false
	}

	// Confidence is the ratio of top score to sum of all scores
	totalScore := 0.0
	for _, c := range candidates {
		totalScore += c.Score
	}

	if totalScore == 0 {
		return 0.0, true
	}

	confidence := candidates[0].Score / totalScore

	// Ambiguous if top two are within 0.1
	ambiguous := false
	if len(candidates) >= 2 {
		diff := candidates[0].Score - candidates[1].Score
		if diff < 0.1 {
			ambiguous = true
		}
	}

	return confidence, ambiguous
}
