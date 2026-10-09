package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/model"
)

// Tier-2 disambiguation outcome vocabulary (D10). These are the recorded
// outcome classes; the confidence-floor downgrade (`applied` → `low_confidence`)
// is applied by the caller that owns the floor (orchestrate/selection_config).
const (
	// Tier2OutcomeApplied: the model returned an in-set candidate id with a
	// confidence at or above the configured floor, and the caller adopted it.
	Tier2OutcomeApplied = "applied"
	// Tier2OutcomeRejected: the model returned an id outside the candidate set.
	Tier2OutcomeRejected = "rejected"
	// Tier2OutcomeUnparseable: the model response carried no usable JSON.
	Tier2OutcomeUnparseable = "unparseable"
	// Tier2OutcomeLowConfidence: an in-set id whose confidence is below the floor.
	Tier2OutcomeLowConfidence = "low_confidence"
	// Tier2OutcomeUnavailable: the model was absent, errored, or timed out.
	Tier2OutcomeUnavailable = "unavailable"
)

// CandidateRef is one deterministic candidate presented to the Tier-2
// disambiguator. The ref carries only public candidate metadata — never raw
// user text.
type CandidateRef struct {
	ID              string
	Kind            string
	Description     string
	MatchedKeywords []string
}

// Tier2Outcome is the result of one bounded Tier-2 disambiguation attempt.
// Outcome is `applied` for a valid in-set answer (the caller may downgrade to
// `low_confidence`), `rejected` for an out-of-set id, `unparseable` for a
// malformed response, or `unavailable` for a model error/absence.
type Tier2Outcome struct {
	Outcome     string
	CandidateID string
	Confidence  float64
	Model       string
	Latency     time.Duration
}

// DisambiguateWithLLM asks the model to choose exactly one candidate id from
// refs. The model can never select outside refs: any id not present in the set
// is rejected. A model error, timeout, or absent model yields an `unavailable`
// outcome and a non-nil error so the caller keeps the deterministic winner.
func DisambiguateWithLLM(ctx context.Context, lm model.LanguageModel, utterance, familyID, streamedContext string, refs []CandidateRef) (Tier2Outcome, error) {
	start := time.Now()
	outcome := Tier2Outcome{Outcome: Tier2OutcomeUnavailable}
	if lm == nil {
		return outcome, fmt.Errorf("no language model provided for tier-2 disambiguation")
	}
	if len(refs) == 0 {
		return outcome, fmt.Errorf("tier-2 disambiguation requires at least one candidate ref")
	}

	prompt := BuildDisambiguationPrompt(utterance, familyID, streamedContext, refs)
	resp, err := lm.Generate(ctx, prompt, &model.LLMOptions{MaxTokens: 128, Temperature: 0.1})
	outcome.Latency = time.Since(start)
	outcome.Model = modelName(lm)
	if err != nil {
		return outcome, fmt.Errorf("tier-2 model call failed: %w", err)
	}

	text := ""
	if resp != nil {
		text = resp.Text
	}
	parsed, ok := parseDisambiguationResponse(text)
	if !ok {
		outcome.Outcome = Tier2OutcomeUnparseable
		return outcome, nil
	}
	outcome.CandidateID = parsed.ID
	outcome.Confidence = parsed.Confidence
	if !containsCandidateRef(refs, parsed.ID) {
		outcome.Outcome = Tier2OutcomeRejected
		return outcome, nil
	}
	outcome.Outcome = Tier2OutcomeApplied
	return outcome, nil
}

// BuildDisambiguationPrompt renders the bounded disambiguation prompt. It
// includes only the supplied candidate refs; a candidate outside the ref set
// can never appear in the prompt.
func BuildDisambiguationPrompt(utterance, familyID, streamedContext string, refs []CandidateRef) string {
	var b strings.Builder
	b.WriteString("You disambiguate between candidate routes that were already selected deterministically.\n")
	b.WriteString("Choose exactly one id from the candidate list below. Do not invent ids.\n\n")
	b.WriteString("Task: ")
	b.WriteString(strings.TrimSpace(utterance))
	b.WriteString("\n")
	if family := strings.TrimSpace(familyID); family != "" {
		b.WriteString("Family: ")
		b.WriteString(family)
		b.WriteString("\n")
	}
	if context := strings.TrimSpace(streamedContext); context != "" {
		b.WriteString("Context: ")
		b.WriteString(context)
		b.WriteString("\n")
	}
	b.WriteString("\nCandidates:\n")
	for _, ref := range refs {
		b.WriteString("- id: ")
		b.WriteString(ref.ID)
		b.WriteString(" | kind: ")
		b.WriteString(ref.Kind)
		if description := strings.TrimSpace(ref.Description); description != "" {
			b.WriteString(" | ")
			b.WriteString(description)
		}
		if len(ref.MatchedKeywords) > 0 {
			b.WriteString(" | keywords: ")
			b.WriteString(strings.Join(ref.MatchedKeywords, ","))
		}
		b.WriteString("\n")
	}
	b.WriteString("\nRespond with ONLY a JSON object: ")
	b.WriteString(`{"id": "<one candidate id>", "confidence": 0.0}`)
	b.WriteString("\n")
	return b.String()
}

type disambiguationResponse struct {
	ID         string  `json:"id"`
	Confidence float64 `json:"confidence"`
}

// parseDisambiguationResponse extracts the JSON object from the model response
// and returns ok=false for a malformed or empty answer.
func parseDisambiguationResponse(text string) (disambiguationResponse, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return disambiguationResponse{}, false
	}
	var parsed disambiguationResponse
	if err := json.Unmarshal([]byte(text[start:end+1]), &parsed); err != nil {
		return disambiguationResponse{}, false
	}
	parsed.ID = strings.TrimSpace(parsed.ID)
	if parsed.ID == "" {
		return disambiguationResponse{}, false
	}
	return parsed, true
}

func containsCandidateRef(refs []CandidateRef, id string) bool {
	id = strings.TrimSpace(id)
	for _, ref := range refs {
		if ref.ID == id {
			return true
		}
	}
	return false
}

// modelName reports a stable model identifier for the record. It prefers an
// explicit Name() method and falls back to the concrete type name.
func modelName(lm model.LanguageModel) string {
	if named, ok := lm.(interface{ Name() string }); ok {
		if name := strings.TrimSpace(named.Name()); name != "" {
			return name
		}
	}
	return fmt.Sprintf("%T", lm)
}
