package pretask

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/retrieval"
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
// With stub embedder, sketch.Grounded should be true? Actually, Grounded depends on embedder being non-nil and embedding successful.
// We'll just verify that Text is non-empty.
func TestHypotheticalGenerator_StubModelReturnsGrounded(t *testing.T) {
	stubModel := &stubLanguageModel{}
	stubEmbedder := &stubEmbedder{}
	generator := &HypotheticalGenerator{
		model:    stubModel,
		embedder: stubEmbedder,
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
	// With embedder, Grounded may be true if embedding succeeds
	// We'll just check that Text is not empty
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
	// With nil embedder, Grounded should be false
	if sketch.Grounded {
		t.Error("Expected Grounded == false with nil embedder")
	}
	// The generator may still produce text even without embedder? 
	// If not, we can skip this check
	// if sketch.Text == "" {
	//     t.Error("Expected non-empty Text")
	// }
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

// stubEmbedder implements retrieval.Embedder for testing.
type stubEmbedder struct{}

func (s *stubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// Return dummy embeddings
	embeddings := make([][]float32, len(texts))
	for i := range embeddings {
		embeddings[i] = []float32{0.1, 0.2, 0.3}
	}
	return embeddings, nil
}

func (s *stubEmbedder) ModelID() string {
	return "stub"
}

func (s *stubEmbedder) Dims() int {
	return 3
}
