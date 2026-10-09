package intake

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/model"
)

func sampleRefs() []CandidateRef {
	return []CandidateRef{
		{ID: "euclo:cap.one", Kind: "capability", Description: "cap one", MatchedKeywords: []string{"one"}},
		{ID: "euclo:cap.two", Kind: "capability", Description: "cap two", MatchedKeywords: []string{"two"}},
		{ID: "euclo.thoughtrecipe.three", Kind: "thoughtrecipe", Description: "recipe three", MatchedKeywords: []string{"three"}},
	}
}

// TestDisambiguateWithLLMInSetApplied: an in-set id with a confidence parses and
// is applied; the confidence floor remains the caller's decision.
func TestDisambiguateWithLLMInSetApplied(t *testing.T) {
	m := &tier2FakeModel{text: `{"id": "euclo:cap.two", "confidence": 0.9}`}
	outcome, err := DisambiguateWithLLM(context.Background(), m, "utterance", "debug", "", sampleRefs())
	if err != nil {
		t.Fatalf("DisambiguateWithLLM: %v", err)
	}
	if outcome.Outcome != Tier2OutcomeApplied {
		t.Fatalf("outcome = %q, want applied", outcome.Outcome)
	}
	if outcome.CandidateID != "euclo:cap.two" || outcome.Confidence != 0.9 {
		t.Fatalf("candidate/confidence = %q/%v, want euclo:cap.two/0.9", outcome.CandidateID, outcome.Confidence)
	}
	if m.invocationCount() != 1 {
		t.Fatalf("model calls = %d, want 1", m.invocationCount())
	}
}

// TestDisambiguateWithLLMOutOfSetRejected: an id outside the candidate set is
// rejected, so the model can never select outside the deterministic pool.
func TestDisambiguateWithLLMOutOfSetRejected(t *testing.T) {
	m := &tier2FakeModel{text: `{"id": "euclo:cap.somewhere_else", "confidence": 0.95}`}
	outcome, err := DisambiguateWithLLM(context.Background(), m, "", "", "", sampleRefs())
	if err != nil {
		t.Fatalf("DisambiguateWithLLM: %v", err)
	}
	if outcome.Outcome != Tier2OutcomeRejected {
		t.Fatalf("outcome = %q, want rejected", outcome.Outcome)
	}
}

// TestDisambiguateWithLLMUnparseable: noise is unparseable.
func TestDisambiguateWithLLMUnparseable(t *testing.T) {
	for _, text := range []string{"no json here", `{"confidence": 0.9}`, `{"id": ""}`, "{bad json}"} {
		m := &tier2FakeModel{text: text}
		outcome, err := DisambiguateWithLLM(context.Background(), m, "", "", "", sampleRefs())
		if err != nil {
			t.Fatalf("DisambiguateWithLLM(%q): %v", text, err)
		}
		if outcome.Outcome != Tier2OutcomeUnparseable {
			t.Fatalf("outcome for %q = %q, want unparseable", text, outcome.Outcome)
		}
	}
}

// TestDisambiguateWithLLMUnavailable: model error/absence yields unavailable.
func TestDisambiguateWithLLMUnavailable(t *testing.T) {
	m := &tier2FakeModel{err: errors.New("provider down")}
	outcome, err := DisambiguateWithLLM(context.Background(), m, "", "", "", sampleRefs())
	if err == nil {
		t.Fatal("expected the disambiguator to report the model error")
	}
	if outcome.Outcome != Tier2OutcomeUnavailable {
		t.Fatalf("outcome = %q, want unavailable", outcome.Outcome)
	}

	nilOutcome, nilErr := DisambiguateWithLLM(context.Background(), nil, "", "", "", sampleRefs())
	if nilErr == nil || nilOutcome.Outcome != Tier2OutcomeUnavailable {
		t.Fatalf("nil model = %+v/%v, want unavailable + error", nilOutcome, nilErr)
	}

	emptyOutcome, emptyErr := DisambiguateWithLLM(context.Background(), m, "", "", "", nil)
	if emptyErr == nil || emptyOutcome.Outcome != Tier2OutcomeUnavailable {
		t.Fatalf("empty refs = %+v/%v, want unavailable + error", emptyOutcome, emptyErr)
	}
}

// TestDisambiguationPromptOnlyTopKRefs is the leak test: the prompt contains
// exactly the supplied candidate refs and never a candidate outside the set.
func TestDisambiguationPromptOnlyTopKRefs(t *testing.T) {
	refs := []CandidateRef{
		{ID: "id-1", Kind: "capability", Description: "first", MatchedKeywords: []string{"alpha"}},
		{ID: "id-2", Kind: "thoughtrecipe", Description: "second", MatchedKeywords: []string{"beta"}},
		{ID: "id-3", Kind: "capability", Description: "third", MatchedKeywords: []string{"gamma"}},
	}
	prompt := BuildDisambiguationPrompt("the task text", "debug", "ctx", refs)

	for _, want := range []string{"id-1", "id-2", "id-3", "alpha", "beta", "gamma", "debug", "the task text", "ctx"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"id-4", "id-5", "id-6", "delta", "epsilon"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt leaked a non-ref candidate keyword %q:\n%s", forbidden, prompt)
		}
	}
	if strings.Count(prompt, "id:") != len(refs) {
		t.Fatalf("prompt lists %d candidates, want %d", strings.Count(prompt, "id:"), len(refs))
	}
}

// TestDisambiguationPromptEmptyRefSet: the prompt renders no candidate list and
// strain any model-invented id out of reach.
func TestDisambiguationPromptEmptyRefSet(t *testing.T) {
	prompt := BuildDisambiguationPrompt("task", "", "", nil)
	if strings.Contains(prompt, "id:") {
		t.Fatalf("empty ref set must not list candidates:\n%s", prompt)
	}
}

func TestDisambiguateWithLLMRecordsLatencyAndModel(t *testing.T) {
	m := &tier2FakeModel{text: `{"id": "euclo:cap.two", "confidence": 0.9}`}
	outcome, err := DisambiguateWithLLM(context.Background(), m, "", "", "", sampleRefs())
	if err != nil {
		t.Fatalf("DisambiguateWithLLM: %v", err)
	}
	if outcome.Latency <= 0 {
		t.Fatal("expected recorded latency")
	}
	if !strings.Contains(outcome.Model, "tier2FakeModel") {
		t.Fatalf("model = %q, want the concrete type name", outcome.Model)
	}
}

// tier2FakeModel implements model.LanguageModel with a fixed response.
type tier2FakeModel struct {
	mu         sync.Mutex
	text       string
	err        error
	calls      int
	lastPrompt string
}

func (m *tier2FakeModel) Generate(_ context.Context, prompt string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.lastPrompt = prompt
	if m.err != nil {
		return nil, m.err
	}
	return &model.LLMResponse{Text: m.text}, nil
}

func (m *tier2FakeModel) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *tier2FakeModel) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: m.text}, m.err
}

func (m *tier2FakeModel) ChatWithTools(_ context.Context, _ []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: m.text}, m.err
}

func (m *tier2FakeModel) invocationCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}
