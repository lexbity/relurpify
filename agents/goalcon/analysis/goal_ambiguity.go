package analysis

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"strings"
	"time"
)

// AmbiguityIndicator identifies a potential ambiguity in a goal.
type AmbiguityIndicator struct {
	Type           string  // "vague_verb", "missing_object", "conflicting_predicates", "unclear_scope"
	Description    string  // Human-readable description
	AffectedPhrase string  // Part of goal that's ambiguous
	Confidence     float32 // 0.0-1.0, confidence in ambiguity
	Suggestion     string  // How to clarify
	Timestamp      time.Time
}

// AmbiguityScore represents the overall ambiguity of a goal.
type AmbiguityScore struct {
	Goal         types.GoalCondition
	OverallScore float32 // 0.0 (clear) to 1.0 (very ambiguous)
	Indicators   []*AmbiguityIndicator
	ShouldRefine bool
	Reason       string
	Confidence   float32 // Confidence in the ambiguity assessment
}

// VagueVerbs is a list of verbs that often indicate ambiguous intentions.
var VagueVerbs = []string{
	"improve", "enhance", "optimize", "fix", "update", "handle", "process",
	"work", "make", "do", "check", "verify", "validate", "ensure", "support",
}

// AmbiguityAnalyzer detects and scores ambiguities in goals.
type AmbiguityAnalyzer struct {
	ambiguityThreshold float32 // Default 0.6
	maxIndicators      int     // Max indicators to report (default 5)
}

// NewAmbiguityAnalyzer creates a new ambiguity analyzer.
func NewAmbiguityAnalyzer() *AmbiguityAnalyzer {
	return &AmbiguityAnalyzer{
		ambiguityThreshold: 0.6,
		maxIndicators:      5,
	}
}

// SetThreshold sets the ambiguity threshold for refinement decisions.
func (aa *AmbiguityAnalyzer) SetThreshold(threshold float32) {
	if aa != nil && threshold >= 0 && threshold <= 1.0 {
		aa.ambiguityThreshold = threshold
	}
}

// AnalyzeAmbiguities examines a goal and classification response for ambiguities.
func (aa *AmbiguityAnalyzer) AnalyzeAmbiguities(goal types.GoalCondition, response *ClassificationResponse) *AmbiguityScore {
	if aa == nil {
		return nil
	}

	score := &AmbiguityScore{
		Goal:       goal,
		Indicators: make([]*AmbiguityIndicator, 0),
	}

	// Phase 1: Use ambiguities from ClassificationResponse (already computed by LLM!)
	if response != nil && len(response.Ambiguities) > 0 {
		for i, amb := range response.Ambiguities {
			if i >= aa.maxIndicators {
				break
			}
			indicator := &AmbiguityIndicator{
				Type:           "from_classifier",
				Description:    amb,
				AffectedPhrase: amb,
				Confidence:     0.75, // Trust the LLM's ambiguity detection
				Suggestion:     "Clarify: " + amb,
				Timestamp:      time.Now().UTC(),
			}
			score.Indicators = append(score.Indicators, indicator)
		}
	}

	// Phase 2: Pattern-based detection (fallback if no LLM response)
	if len(score.Indicators) == 0 {
		indicators := aa.detectVagueLanguage(goal)
		for i, indicator := range indicators {
			if i >= aa.maxIndicators {
				break
			}
			score.Indicators = append(score.Indicators, indicator)
		}
	}

	// Calculate overall score
	if len(score.Indicators) > 0 {
		totalConfidence := float32(0)
		for _, ind := range score.Indicators {
			totalConfidence += ind.Confidence
		}
		score.OverallScore = totalConfidence / float32(len(score.Indicators))
		score.Confidence = score.OverallScore
	}

	// Decide if refinement is needed
	score.ShouldRefine = score.OverallScore >= aa.ambiguityThreshold
	if score.ShouldRefine {
		score.Reason = "Goal ambiguity score exceeds threshold"
	}

	return score
}

// detectVagueLanguage uses pattern matching to find vague language in goals.
func (aa *AmbiguityAnalyzer) detectVagueLanguage(goal types.GoalCondition) []*AmbiguityIndicator {
	var indicators []*AmbiguityIndicator

	description := strings.ToLower(goal.Description)

	// Check for vague verbs
	for _, verb := range VagueVerbs {
		if strings.Contains(description, verb) {
			indicator := &AmbiguityIndicator{
				Type:           "vague_verb",
				Description:    "Vague verb '" + verb + "' may indicate unclear intent",
				AffectedPhrase: verb,
				Confidence:     0.5,
				Suggestion:     "Use more specific verb (e.g., instead of 'improve', specify 'increase speed by 10%')",
				Timestamp:      time.Now().UTC(),
			}
			indicators = append(indicators, indicator)
			if len(indicators) >= 2 {
				break // Limit vague verb indicators
			}
		}
	}

	// Check for too many predicates (suggests unclear scope)
	if len(goal.Predicates) > 5 {
		indicator := &AmbiguityIndicator{
			Type:           "unclear_scope",
			Description:    "Goal has many predicates; may be unclear what the primary objective is",
			AffectedPhrase: "multiple predicates",
			Confidence:     0.4,
			Suggestion:     "Break goal into smaller, more focused sub-goals",
			Timestamp:      time.Now().UTC(),
		}
		indicators = append(indicators, indicator)
	}

	// Check for missing objects (predicates with no clear targets)
	if len(goal.Predicates) > 0 {
		for _, pred := range goal.Predicates {
			predStr := string(pred)
			if len(strings.Fields(predStr)) < 2 {
				indicator := &AmbiguityIndicator{
					Type:           "missing_object",
					Description:    "types.Predicate '" + predStr + "' lacks clear object or target",
					AffectedPhrase: predStr,
					Confidence:     0.6,
					Suggestion:     "Specify what should be " + predStr,
					Timestamp:      time.Now().UTC(),
				}
				indicators = append(indicators, indicator)
				if len(indicators) >= 2 {
					break
				}
			}
		}
	}

	return indicators
}

// ShouldRefineGoal determines if a goal needs refinement based on analysis.
func (aa *AmbiguityAnalyzer) ShouldRefineGoal(goal types.GoalCondition, response *ClassificationResponse) bool {
	if aa == nil {
		return false
	}

	score := aa.AnalyzeAmbiguities(goal, response)
	if score == nil {
		return false
	}

	return score.ShouldRefine
}

// GetRefinementSuggestions returns human-friendly suggestions for clarification.
func (aa *AmbiguityAnalyzer) GetRefinementSuggestions(score *AmbiguityScore) []string {
	if score == nil || len(score.Indicators) == 0 {
		return nil
	}

	suggestions := make([]string, 0, len(score.Indicators))
	for _, indicator := range score.Indicators {
		if indicator.Suggestion != "" {
			suggestions = append(suggestions, indicator.Suggestion)
		}
	}

	return suggestions
}

// FormatAmbiguityReport generates a human-readable ambiguity report.
func (aa *AmbiguityAnalyzer) FormatAmbiguityReport(score *AmbiguityScore) string {
	if score == nil {
		return "No ambiguity analysis available"
	}

	var sb strings.Builder
	sb.WriteString("Goal Ambiguity Analysis\n")
	sb.WriteString("========================\n\n")
	sb.WriteString("Goal: " + score.Goal.Description + "\n")
	sb.WriteString("Overall Ambiguity Score: " + formatScore(score.OverallScore) + "\n")
	sb.WriteString("Recommendation: ")

	if score.ShouldRefine {
		sb.WriteString("REFINE THIS GOAL\n")
	} else {
		sb.WriteString("Goal is clear enough to proceed\n")
	}

	if len(score.Indicators) > 0 {
		sb.WriteString("\nAmbiguities Found:\n")
		for i, indicator := range score.Indicators {
			sb.WriteString("  " + string(rune(i+1)) + ". " + indicator.Description + "\n")
			sb.WriteString("     Suggestion: " + indicator.Suggestion + "\n")
		}
	}

	return sb.String()
}

// formatScore converts a 0-1 score to percentage string.
func formatScore(score float32) string {
	if score < 0 {
		return "0%"
	}
	if score > 1 {
		return "100%"
	}
	percent := int(score * 100)
	switch {
	case percent <= 0:
		return "0%"
	case percent >= 100:
		return "100%"
	default:
		// Simple percentage formatting without imports
		tensDigit := percent / 10
		onesDigit := percent % 10
		return string(rune('0'+tensDigit)) + string(rune('0'+onesDigit)) + "%"
	}
}
