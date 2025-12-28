package framework_test

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
)

type stubLLM struct {
	text string
}

func (s *stubLLM) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: s.text}, nil
}

func (s *stubLLM) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubLLM) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubLLM) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.Tool, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

type stubCompressionStrategy struct {
	compressed *core.CompressedContext
	should     bool
	recent     int
}

func (s *stubCompressionStrategy) Compress(interactions []core.Interaction, llm core.LanguageModel) (*core.CompressedContext, error) {
	return s.compressed, nil
}

func (s *stubCompressionStrategy) ShouldCompress(ctx *core.Context, budget *core.ContextBudget) bool {
	return s.should
}

func (s *stubCompressionStrategy) EstimateTokens(cc *core.CompressedContext) int {
	if cc == nil {
		return 0
	}
	return cc.CompressedTokens
}

func (s *stubCompressionStrategy) KeepRecent() int {
	if s.recent == 0 {
		return 5
	}
	return s.recent
}
