package intake

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

type stubLanguageModel struct {
	calls int
	resp  string
	err   error
}

func (s *stubLanguageModel) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &core.LLMResponse{Text: s.resp}, nil
}

func (s *stubLanguageModel) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return s.Generate(ctx, "", options)
}

func (s *stubLanguageModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	return s.Generate(ctx, "", options)
}

func (s *stubLanguageModel) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, nil
}

func TestTieredCapabilityClassifier_Tier1KeywordMatchWithoutModel(t *testing.T) {
	classifier := &TieredCapabilityClassifier{Registry: euclorelurpic.DefaultRegistry()}

	seq, op, err := classifier.Classify(context.Background(), "please explain this code", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seq) != 1 || seq[0] != euclorelurpic.CapabilityChatAsk {
		t.Fatalf("unexpected sequence: %v", seq)
	}
	if op != "AND" {
		t.Fatalf("unexpected operator: %q", op)
	}
}

func TestTieredCapabilityClassifier_Tier1DoesNotCallModel(t *testing.T) {
	model := &stubLanguageModel{resp: "id: euclo:chat.inspect"}
	classifier := &TieredCapabilityClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
		Model:    model,
	}

	seq, op, err := classifier.Classify(context.Background(), "please explain this code", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.calls != 0 {
		t.Fatalf("expected model not to be called, got %d calls", model.calls)
	}
	if len(seq) != 1 || seq[0] != euclorelurpic.CapabilityChatAsk {
		t.Fatalf("unexpected sequence: %v", seq)
	}
	if op != "AND" {
		t.Fatalf("unexpected operator: %q", op)
	}
}

func TestTieredCapabilityClassifier_Tier3FallbackWhenNoKeywordAndNilModel(t *testing.T) {
	classifier := &TieredCapabilityClassifier{Registry: euclorelurpic.DefaultRegistry()}

	seq, op, err := classifier.Classify(context.Background(), "gibberish request", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seq) != 1 || seq[0] != euclorelurpic.CapabilityChatAsk {
		t.Fatalf("unexpected sequence: %v", seq)
	}
	if op != "AND" {
		t.Fatalf("unexpected operator: %q", op)
	}
}

func TestTieredCapabilityClassifier_Tier2UsesModelWhenNoKeywordMatch(t *testing.T) {
	model := &stubLanguageModel{resp: "id: euclo:chat.inspect"}
	classifier := &TieredCapabilityClassifier{
		Registry: euclorelurpic.DefaultRegistry(),
		Model:    model,
	}

	seq, op, err := classifier.Classify(context.Background(), "gibberish request", "chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.calls == 0 {
		t.Fatal("expected model to be called")
	}
	if len(seq) != 1 || seq[0] != euclorelurpic.CapabilityChatInspect {
		t.Fatalf("unexpected sequence: %v", seq)
	}
	if op != "AND" {
		t.Fatalf("unexpected operator: %q", op)
	}
}
