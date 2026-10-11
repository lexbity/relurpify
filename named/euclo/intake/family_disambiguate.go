package intake

import (
	"context"
	"errors"
	"time"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// Family tie disambiguation (owner ruling 2026-10-11, option A). When the
// deterministic keyword classifier ties its top two families (top-2 scores
// within the ambiguity threshold), one bounded model call picks between the
// two. The contract mirrors the route Tier-2 disambiguator (wave 2, D10):
// set-restricted to the tied pair, confidence-floored, and the deterministic
// winner stands on any failure — model absent, errored, out-of-set answer,
// unparseable response. The model never creates a third option; it only
// chooses between the two the scorer produced.
const (
	// FamilyDisambiguationConfidenceMark is the self-reported confidence above
	// which an adopted in-set answer counts as confident. It is a MARKER
	// threshold, not a gate: below it the answer still adopts with
	// TiebreakLowConfidence set — live evidence (2026-10-11) showed small
	// local models report uncalibrated confidence (gemma4:e4b answered
	// correctly while reporting 0.0), so gating on it would discard correct
	// disambiguations.
	FamilyDisambiguationConfidenceMark = 0.5
)

// FamilyDisambiguation is the outcome of one bounded family tie-break
// attempt, using the shared Tier-2 outcome vocabulary (applied | rejected |
// unparseable | low_confidence | unavailable). `low_confidence` is applied
// by the caller that owns the floor.
type FamilyDisambiguation struct {
	Outcome    string
	FamilyID   string
	Confidence float64
	Latency    time.Duration
}

// DisambiguateFamilies asks the model to choose exactly one family id from
// the tied pair. Candidates MUST be the scorer's sorted output (top-2 are
// taken). The prompt carries only the two family ids and display names.
func DisambiguateFamilies(ctx context.Context, lm model.LanguageModel, utterance string, candidates []families.FamilyCandidate, registry *families.KeywordFamilyRegistry) (FamilyDisambiguation, error) {
	outcome := FamilyDisambiguation{Outcome: Tier2OutcomeUnavailable}
	if lm == nil {
		return outcome, errors.New("family disambiguation requires a language model")
	}
	if len(candidates) < 2 {
		return outcome, errors.New("family disambiguation requires at least two tied candidates")
	}

	refs := make([]CandidateRef, 0, 2)
	for _, candidate := range candidates[:2] {
		ref := CandidateRef{ID: candidate.FamilyID, Kind: "family"}
		if registry != nil {
			if def, ok := registry.Lookup(candidate.FamilyID); ok {
				ref.Description = def.DisplayName
			}
		}
		refs = append(refs, ref)
	}

	result, err := DisambiguateWithLLM(ctx, lm, utterance, "", "", refs, nil)
	outcome.Latency = result.Latency
	if err != nil {
		return outcome, err
	}
	outcome.Outcome = result.Outcome
	outcome.FamilyID = result.CandidateID
	outcome.Confidence = result.Confidence
	return outcome, nil
}

// disambiguateFamilyTie resolves a deterministic family tie through the
// bounded model disambiguator. It fires ONLY when the scorer marked the
// classification ambiguous with at least two tied candidates and a tie-break
// model is composed. Adoption rules: an IN-SET answer adopts the picked
// family — self-reported confidence is provenance, not a gate (live evidence,
// 2026-10-11: gemma4:e4b picked the correct family with sound reasoning while
// reporting confidence 0.0; small local models do not produce calibrated
// confidence, and a tie that the scorer itself could not separate is exactly
// where semantic judgment is the better evidence). Out-of-set, unparseable,
// and unavailable outcomes keep the deterministic result untouched,
// preserving the clarification posture downstream. Answers below the
// confidence mark adopt WITH a low-confidence marker in the reason codes.
// Every attempt is reported on the node context's telemetry sink.
func (n *IntakePipelineNode) disambiguateFamilyTie(ctx context.Context, scored *ScoredClassification, instruction string) *ScoredClassification {
	if scored == nil || !scored.Ambiguous || len(scored.FamilyCandidates) < 2 {
		return scored
	}
	if n.tier2Model == nil {
		emitFamilyDisambiguation(ctx, FamilyDisambiguation{Outcome: Tier2OutcomeUnavailable}, nil, scored)
		return scored
	}

	outcome, err := DisambiguateFamilies(ctx, n.tier2Model, instruction, scored.FamilyCandidates, n.registry)
	emitFamilyDisambiguation(ctx, outcome, err, scored)
	if err != nil {
		return scored
	}
	if outcome.Outcome != Tier2OutcomeApplied {
		return scored
	}

	adopted := *scored
	adopted.WinningFamily = outcome.FamilyID
	adopted.Ambiguous = false
	adopted.Confidence = outcome.Confidence
	adopted.DisambiguationSource = "model_family_tiebreak"
	adopted.Signals = append(append([]ClassificationSignal(nil), scored.Signals...), ClassificationSignal{
		Kind:     SignalKindFamilyTiebreak,
		FamilyID: outcome.FamilyID,
		Weight:   outcome.Confidence,
		Source:   "model_family_tiebreak",
	})
	adopted.TiebreakLowConfidence = outcome.Confidence < FamilyDisambiguationConfidenceMark
	return &adopted
}

func emitFamilyDisambiguation(ctx context.Context, outcome FamilyDisambiguation, err error, scored *ScoredClassification) {
	tel := telemetry.TelemetryFromContext(ctx)
	if tel == nil {
		return
	}
	tied := make([]string, 0, 2)
	for _, candidate := range scored.FamilyCandidates {
		if len(tied) == 2 {
			break
		}
		tied = append(tied, candidate.FamilyID)
	}
	ev := telemetry.Event{
		Type:      telemetry.EventIntentFamilyDisambiguation,
		Timestamp: time.Now().UTC(),
		Message:   outcome.Outcome,
		Metadata: map[string]any{
			"tied_families": tied,
			"picked":        outcome.FamilyID,
			"confidence":    outcome.Confidence,
			"latency_ms":    outcome.Latency.Milliseconds(),
		},
	}
	if err != nil {
		ev.Metadata["error"] = err.Error()
	}
	telemetry.StampCorrelation(ctx, &ev)
	tel.Emit(ev)
}
