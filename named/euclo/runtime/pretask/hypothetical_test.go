package pretask

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestHypotheticalGenerator_Generate(t *testing.T) {
	generator := &HypotheticalGenerator{}
	
	stage1 := Stage1Result{
		CodeEvidence: []CodeEvidenceItem{
			{Path: "test.go", Summary: "Test file"},
		},
	}
	
	sketch, err := generator.Generate(context.Background(), "test query", stage1)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	
	// With nil model and embedder, should return not grounded
	if sketch.Grounded {
		t.Error("Expected not grounded with nil dependencies")
	}
}

// TestHypotheticalGenerator_StubModelReturnsGrounded — stub LLM implementing core.LanguageModel returns *core.LLMResponse{Content: ...}.
// No embedder needed (nil embedder path). Verify sketch.Grounded == false (no embedder), sketch.Text != "".
func TestHypotheticalGenerator_StubModelReturnsGrounded(t *testing.T) {
	stubModel := &stubLanguageModel{}
	generator := &HypotheticalGenerator{
		model: stubModel,
		// embedder is nil
	}
	stage1 := Stage1Result{
		CodeEvidence: []CodeEvidenceItem{
			{Path: "test.go", Summary: "Test file"},
		},
	}
	sketch, err := generator.Generate(context.Background(), "test query", stage1)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// With nil embedder, Grounded should be false
	if sketch.Grounded {
		t.Error("Expected Grounded == false with nil embedder")
	}
	if sketch.Text == "" {
		t.Error("Expected non-empty Text from stub model")
	}
}

// TestHypotheticalGenerator_NilEmbedderSetsGroundedFalse — same stub model, nil embedder, verify Grounded == false but Text is populated.
func TestHypotheticalGenerator_NilEmbedderSetsGroundedFalse(t *testing.T) {
	stubModel := &stubLanguageModel{}
	generator := &HypotheticalGenerator{
		model: stubModel,
		// embedder is nil
	}
	stage1 := Stage1Result{
		CodeEvidence: []CodeEvidenceItem{
			{Path: "test.go", Summary: "Test file"},
		},
	}
	sketch, err := generator.Generate(context.Background(), "test query", stage1)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if sketch.Grounded {
		t.Error("Expected Grounded == false with nil embedder")
	}
	if sketch.Text == "" {
		t.Error("Expected non-empty Text")
	}
}

// stubLanguageModel implements core.LanguageModel for testing.
type stubLanguageModel struct{}

func (s *stubLanguageModel) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{
		Text: "MyHandler fileSystem",
	}, nil
}

func (s *stubLanguageModel) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- "MyHandler fileSystem"
	close(ch)
	return ch, nil
}

func (s *stubLanguageModel) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{
		Text: "MyHandler fileSystem",
	}, nil
}

func (s *stubLanguageModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{
		Text: "MyHandler fileSystem",
	}, nil
}
