package intake

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// scriptedTieModel is the fake LanguageModel for tie-break tests: Generate
// returns the scripted JSON body and records every prompt it is asked.
type scriptedTieModel struct {
	response string
	err      error
	prompts  []string
}

func (m *scriptedTieModel) Generate(_ context.Context, prompt string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.prompts = append(m.prompts, prompt)
	if m.err != nil {
		return nil, m.err
	}
	return &model.LLMResponse{Text: m.response}, nil
}

func (m *scriptedTieModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	return nil, nil
}

func (m *scriptedTieModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, nil
}

func (m *scriptedTieModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, nil
}

// recordingTieTelemetry captures emitted events for the observability pins.
type recordingTieTelemetry struct {
	events []telemetry.Event
}

func (r *recordingTieTelemetry) Emit(ev telemetry.Event) {
	r.events = append(r.events, ev)
}

func tiebreakFixture(t *testing.T, response string) (*IntakePipelineNode, *scriptedTieModel, *recordingTieTelemetry, context.Context) {
	t.Helper()
	registry := families.NewRegistry()
	require.NoError(t, registry.Register(families.KeywordFamily{ID: "debug", DisplayName: "Debugging and repair"}))
	require.NoError(t, registry.Register(families.KeywordFamily{ID: "migration", DisplayName: "Migration and upgrades"}))

	lm := &scriptedTieModel{response: response}
	recorder := &recordingTieTelemetry{}
	ctx := telemetry.WithTelemetry(context.Background(), recorder)

	node := NewIntakePipelineNode("euclo.intake", registry, 512, nil).WithTier2Model(lm)
	return node, lm, recorder, ctx
}

func ambiguousTie() *ScoredClassification {
	return &ScoredClassification{
		WinningFamily: "debug",
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: "debug", Score: 0.55},
			{FamilyID: "migration", Score: 0.50},
		},
		Confidence: 0.52,
		Ambiguous:  true,
	}
}

// TestFamilyTieAdoptedInSetAnswer proves option A's core: an in-set answer at
// or above the floor adopts the picked family, clears the ambiguous flag, and
// records the tiebreak source and signal. The prompt carried exactly the tied
// pair — never a third option.
func TestFamilyTieAdoptedInSetAnswer(t *testing.T) {
	node, lm, recorder, ctx := tiebreakFixture(t, `{"id": "debug", "confidence": 0.9}`)

	adopted := node.disambiguateFamilyTie(ctx, ambiguousTie(), "fix the misspelling in hello.py")

	require.Same(t, adopted, adopted, "sanity: result returned")
	require.Equal(t, "debug", adopted.WinningFamily)
	require.False(t, adopted.Ambiguous, "a floored in-set answer resolves the ambiguity")
	require.Equal(t, 0.9, adopted.Confidence)
	require.Equal(t, "model_family_tiebreak", adopted.DisambiguationSource)

	require.Len(t, lm.prompts, 1, "exactly one bounded model call")
	require.Contains(t, lm.prompts[0], "debug")
	require.Contains(t, lm.prompts[0], "migration")
	require.NotContains(t, lm.prompts[0], "implementation", "the prompt is set-restricted to the tied pair")

	var tiebreaks []ClassificationSignal
	for _, s := range adopted.Signals {
		if s.Kind == SignalKindFamilyTiebreak {
			tiebreaks = append(tiebreaks, s)
		}
	}
	require.Len(t, tiebreaks, 1)
	require.Equal(t, "debug", tiebreaks[0].FamilyID)

	require.NotEmpty(t, recorder.events, "every attempt is reported on the telemetry sink")
	ev := recorder.events[len(recorder.events)-1]
	require.Equal(t, telemetry.EventIntentFamilyDisambiguation, ev.Type)
	require.Equal(t, "applied", ev.Message)
	require.Equal(t, "debug", ev.Metadata["picked"])
}

// TestFamilyTieKeepsDeterministicOnRejectedAnswer proves the model can never
// create a third option: an out-of-set id keeps the deterministic result with
// its ambiguous flag (the clarification posture stands).
func TestFamilyTieKeepsDeterministicOnRejectedAnswer(t *testing.T) {
	node, lm, _, ctx := tiebreakFixture(t, `{"id": "coding", "confidence": 0.99}`)

	scored := ambiguousTie()
	adopted := node.disambiguateFamilyTie(ctx, scored, "fix the misspelling")

	require.Same(t, scored, adopted, "the deterministic classification is returned untouched")
	require.True(t, adopted.Ambiguous)
	require.Equal(t, "debug", adopted.WinningFamily)
	require.Empty(t, adopted.DisambiguationSource)
	require.Len(t, lm.prompts, 1)
}

// TestFamilyTieKeepsDeterministicOnUnparseable pins the unparseable outcome.
func TestFamilyTieKeepsDeterministicOnUnparseable(t *testing.T) {
	node, _, _, ctx := tiebreakFixture(t, "I think you should debug it")

	scored := ambiguousTie()
	adopted := node.disambiguateFamilyTie(ctx, scored, "fix it")
	require.Same(t, scored, adopted)
	require.True(t, adopted.Ambiguous)
	for _, s := range adopted.Signals {
		require.NotEqual(t, SignalKindFamilyTiebreak, s.Kind, "no tiebreak signal is appended on unparseable responses")
	}
}

// TestFamilyTieConfidenceIsProvenanceNotGate pins the live-earned semantics:
// an in-set answer adopts at ANY self-reported confidence (gemma4:e4b picked
// correctly while reporting 0.0 — self-reported confidence from small local
// models is uncalibrated), with the low-confidence marker set below the mark.
func TestFamilyTieConfidenceIsProvenanceNotGate(t *testing.T) {
	atMark, _, _, ctx := tiebreakFixture(t, `{"id": "migration", "confidence": 0.5}`)
	adopted := atMark.disambiguateFamilyTie(ctx, ambiguousTie(), "fix it")
	require.Equal(t, "migration", adopted.WinningFamily)
	require.False(t, adopted.Ambiguous)
	require.False(t, adopted.TiebreakLowConfidence, "exactly-at-mark is confident")

	belowMark, _, recorder, ctx2 := tiebreakFixture(t, `{"id": "migration", "confidence": 0.0}`)
	adoptedLow := belowMark.disambiguateFamilyTie(ctx2, ambiguousTie(), "fix it")
	require.Equal(t, "migration", adoptedLow.WinningFamily, "an in-set answer adopts regardless of self-reported confidence")
	require.False(t, adoptedLow.Ambiguous)
	require.True(t, adoptedLow.TiebreakLowConfidence, "below-mark adoption carries the provenance marker")

	lowEvents := 0
	for _, ev := range recorder.events {
		if ev.Type == telemetry.EventIntentFamilyDisambiguation {
			lowEvents++
		}
	}
	require.Equal(t, 1, lowEvents)
}

// TestFamilyTieModelErrorKeepsDeterministic proves a failing model degrades
// to the deterministic result — availability never blocks classification.
func TestFamilyTieModelErrorKeepsDeterministic(t *testing.T) {
	node, _, _, ctx := tiebreakFixture(t, "")
	node.tier2Model.(*scriptedTieModel).err = context.DeadlineExceeded

	scored := ambiguousTie()
	adopted := node.disambiguateFamilyTie(ctx, scored, "fix it")
	require.Same(t, scored, adopted)
	require.True(t, adopted.Ambiguous)
}

// TestFamilyTieNilModelStaysDeterministic proves the nil-model composition
// (hermetic tiers, model-less embedders) never calls anything and never
// changes the result.
func TestFamilyTieNilModelStaysDeterministic(t *testing.T) {
	node := NewIntakePipelineNode("euclo.intake", families.NewRegistry(), 512, nil)
	scored := ambiguousTie()
	adopted := node.disambiguateFamilyTie(context.Background(), scored, "fix it")
	require.Same(t, scored, adopted)
	require.True(t, adopted.Ambiguous)
}

// TestFamilyTieNotFiredWithoutAmbiguity proves the model is consulted only
// for genuine ties: a clean classification and a single-candidate tie both
// skip the call entirely.
func TestFamilyTieNotFiredWithoutAmbiguity(t *testing.T) {
	node, lm, _, ctx := tiebreakFixture(t, `{"id": "debug", "confidence": 0.9}`)

	clean := &ScoredClassification{
		WinningFamily: "debug",
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: "debug", Score: 0.9},
			{FamilyID: "migration", Score: 0.2},
		},
		Confidence: 0.82,
	}
	kept := node.disambiguateFamilyTie(ctx, clean, "fix it")
	require.Same(t, clean, kept)
	require.Empty(t, lm.prompts, "no model call for a non-ambiguous classification")

	singleton := node.disambiguateFamilyTie(ctx, &ScoredClassification{
		WinningFamily:    "debug",
		FamilyCandidates: []families.FamilyCandidate{{FamilyID: "debug", Score: 1}},
		Ambiguous:        true,
	}, "fix it")
	require.NotNil(t, singleton)
	require.True(t, singleton.Ambiguous)
	require.Empty(t, lm.prompts, "the tied-pair guard errors before any model call")
}

// TestDisambiguateFamiliesRequiresTiedPair pins the API contract: fewer than
// two candidates is a caller bug, not an adoption.
func TestDisambiguateFamiliesRequiresTiedPair(t *testing.T) {
	lm := &scriptedTieModel{response: `{"id": "debug", "confidence": 0.9}`}
	_, err := DisambiguateFamilies(context.Background(), lm, "fix it",
		[]families.FamilyCandidate{{FamilyID: "debug", Score: 1}}, families.NewRegistry())
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least two")
}

// TestFamilyTiePromptCarriesFamilyNames proves the prompt gives the model
// what it needs to choose: the tied family ids AND their display names.
func TestFamilyTiePromptCarriesFamilyNames(t *testing.T) {
	node, lm, _, ctx := tiebreakFixture(t, `{"id": "debug", "confidence": 0.9}`)
	node.disambiguateFamilyTie(ctx, ambiguousTie(), "fix the misspelling in hello.py")

	require.Len(t, lm.prompts, 1)
	require.Contains(t, lm.prompts[0], "Debugging and repair")
	require.Contains(t, lm.prompts[0], "Migration and upgrades")
	require.Contains(t, lm.prompts[0], "fix the misspelling in hello.py")
	// The response contract is the shared Tier-2 JSON shape.
	require.Contains(t, lm.prompts[0], `"id"`)
	require.Contains(t, lm.prompts[0], "confidence")
}
