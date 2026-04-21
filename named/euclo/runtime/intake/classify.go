package intake

import (
	"strings"

	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

// ClassifyTask performs keyword-based classification for backward compatibility.
// For confidence-scored classification with ambiguity detection, use ClassifyTaskScored.
// This is extracted from runtime/classification.go ClassifyTask.
func ClassifyTask(envelope eucloruntime.TaskEnvelope) eucloruntime.TaskClassification {
	scored := ClassifyTaskScored(envelope)
	return scored.TaskClassification
}

// ClassifyTaskScored performs signal-based classification with confidence scoring
// and ambiguity detection. Returns ranked mode candidates with the signals that
// contributed to each score.
// This is extracted from runtime/classification.go ClassifyTaskScored.
func ClassifyTaskScored(envelope eucloruntime.TaskEnvelope) eucloruntime.ScoredClassification {
	signals := eucloruntime.CollectSignals(envelope)
	candidates := eucloruntime.ScoreSignals(signals)

	// Build intents from candidates for backward compat.
	intents := make([]string, 0, len(candidates))
	reasons := make([]string, 0, len(signals))
	for _, c := range candidates {
		intents = append(intents, c.Mode)
	}
	for _, s := range signals {
		reasons = append(reasons, s.Kind+":"+s.Value)
	}

	// Default to code if no keyword/task_structure/error_text signals fired.
	// Skip baseline injection when a review signal is already present.
	hasStrongSignal := false
	hasReviewSignal := false
	for _, s := range signals {
		if s.Kind == "keyword" || s.Kind == "task_structure" || s.Kind == "error_text" || s.Kind == "context_hint" || s.Kind == "user_recipe" {
			hasStrongSignal = true
		}
		// Only suppress the code baseline for explicit review signals
		if s.Mode == "review" && (s.Kind == "keyword" || s.Kind == "context_hint" || s.Kind == "task_structure") {
			hasReviewSignal = true
		}
	}
	if !hasStrongSignal && !hasReviewSignal {
		// Inject a baseline code signal so it wins over weak workspace signals.
		signals = append(signals, eucloruntime.ClassificationSignal{
			Kind: "default", Value: "code", Weight: eucloruntime.WeightDefault, Mode: "code",
		})
		candidates = eucloruntime.ScoreSignals(signals)
		intents = make([]string, 0, len(candidates))
		for _, c := range candidates {
			intents = append(intents, c.Mode)
		}
		reasons = append(reasons, "default:code")
	}
	if len(intents) == 0 {
		intents = []string{"code"}
		reasons = append(reasons, "default:code")
		candidates = []eucloruntime.ModeCandidate{{Mode: "code", Score: 0, Signals: []string{"default"}}}
	}

	lower := strings.ToLower(envelope.Instruction)

	classification := eucloruntime.TaskClassification{
		IntentFamilies:                 intents,
		RecommendedMode:                intents[0],
		MixedIntent:                    len(intents) > 1,
		EditPermitted:                  envelope.EditPermitted,
		RequiresEvidenceBeforeMutation: containsIntent(intents, "debug") || strings.TrimSpace(envelope.ExplicitVerification) != "",
		RequiresDeterministicStages:    containsIntent(intents, "planning") || containsIntent(intents, "review"),
		Scope:                          "local",
		RiskLevel:                      "low",
		ReasonCodes:                    reasons,
		TaskType:                       envelope.TaskType,
	}
	if classification.MixedIntent {
		classification.RiskLevel = "medium"
	}
	if containsIntent(intents, "planning") || strings.Contains(lower, "across") || strings.Contains(lower, "multiple") {
		classification.Scope = "cross_cutting"
		classification.RiskLevel = "medium"
	}
	if containsIntent(intents, "review") && !envelope.EditPermitted {
		classification.RiskLevel = "low"
	}
	if !envelope.EditPermitted {
		classification.ReasonCodes = append(classification.ReasonCodes, "constraint:read_only")
	}
	if envelope.CapabilitySnapshot.HasVerificationTools {
		classification.ReasonCodes = append(classification.ReasonCodes, "capability:verification_available")
	}

	// Compute total weight for normalization.
	totalWeight := 0.0
	for _, s := range signals {
		totalWeight += s.Weight
	}

	return eucloruntime.ScoredClassification{
		TaskClassification: classification,
		Candidates:         candidates,
		Confidence:         eucloruntime.NormalizeConfidence(candidates, totalWeight),
		Ambiguous:          eucloruntime.IsAmbiguous(candidates),
		Signals:            signals,
	}
}

// containsIntent checks if a target intent is in the list.
func containsIntent(intents []string, target string) bool {
	for _, intent := range intents {
		if intent == target {
			return true
		}
	}
	return false
}
