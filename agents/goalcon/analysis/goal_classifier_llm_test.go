package analysis

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/agents/goalcon/types"
	"github.com/lexcodex/relurpify/framework/core"
)

// mockLLMModel simulates a language model for testing.
type mockLLMModel struct {
	response string
	err      error
}

func (m *mockLLMModel) Generate(_ context.Context, _ string, _ *core.LLMOptions) (*core.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &core.LLMResponse{Text: m.response}, nil
}

func (m *mockLLMModel) GenerateStream(_ context.Context, _ string, _ *core.LLMOptions) (<-chan string, error) {
	return nil, nil
}

func (m *mockLLMModel) Chat(_ context.Context, _ []core.Message, _ *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, nil
}

func (m *mockLLMModel) ChatWithTools(_ context.Context, _ []core.Message, _ []core.LLMToolSpec, _ *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, nil
}

func TestClassifyGoalWithLLM_Success(t *testing.T) {
	model := &mockLLMModel{
		response: `{
			"predicates": ["file_content_known", "edit_plan_known", "file_modified"],
			"confidence": 0.95,
			"reasoning": "task requires reading, planning, and writing changes"
		}`,
	}

	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{Name: "ReadFile", Effects: []types.Predicate{"file_content_known"}})
	registry.Register(types.Operator{Name: "AnalyzeCode", Preconditions: []types.Predicate{"file_content_known"}, Effects: []types.Predicate{"edit_plan_known"}})
	registry.Register(types.Operator{Name: "WriteFile", Preconditions: []types.Predicate{"edit_plan_known"}, Effects: []types.Predicate{"file_modified"}})

	config := DefaultClassifierConfig()
	goal := ClassifyGoalWithLLM("fix the bug", model, registry, config)

	if len(goal.Predicates) != 3 {
		t.Fatalf("expected 3 predicates, got %d: %v", len(goal.Predicates), goal.Predicates)
	}
	if goal.Predicates[0] != "file_content_known" {
		t.Errorf("expected file_content_known, got %s", goal.Predicates[0])
	}
}

func TestClassifyGoalWithLLM_LowConfidence(t *testing.T) {
	model := &mockLLMModel{
		response: `{
			"predicates": ["edit_plan_known"],
			"confidence": 0.3,
			"reasoning": "unclear what the task requires",
			"ambiguities": ["is this a code change or a config change?"]
		}`,
	}

	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{Name: "ReadFile", Effects: []types.Predicate{"file_content_known"}})

	config := DefaultClassifierConfig()
	config.MinConfidence = 0.5 // Won't meet threshold
	goal := ClassifyGoalWithLLM("fix something", model, registry, config)

	// Should fall back to keyword matching
	if goal.Description != "fix something" {
		t.Errorf("expected description to be set, got %s", goal.Description)
	}
	// Keyword matching should detect "fix"
	found := false
	for _, p := range goal.Predicates {
		if p == "file_modified" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected file_modified from keyword fallback, got %v", goal.Predicates)
	}
}

func TestClassifyGoalWithLLM_ModelError(t *testing.T) {
	model := &mockLLMModel{
		err: ErrTestModelFailed,
	}

	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{Name: "ReadFile", Effects: []types.Predicate{"file_content_known"}})

	config := DefaultClassifierConfig()
	config.FallbackOnFail = true

	goal := ClassifyGoalWithLLM("analyze the code", model, registry, config)

	// Should fall back to keyword matching
	if goal.Description != "analyze the code" {
		t.Errorf("expected description to be set")
	}
	found := false
	for _, p := range goal.Predicates {
		if p == "edit_plan_known" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected edit_plan_known from keyword fallback, got %v", goal.Predicates)
	}
}

func TestClassifyGoalWithLLM_Disabled(t *testing.T) {
	model := &mockLLMModel{
		response: `{
			"predicates": ["test_result_known"],
			"confidence": 0.99,
			"reasoning": "very clear"
		}`,
	}

	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{Name: "RunTests", Effects: []types.Predicate{"test_result_known"}})

	config := DefaultClassifierConfig()
	config.Enabled = false // Disable LLM classification

	goal := ClassifyGoalWithLLM("run tests", model, registry, config)

	// Should skip LLM and use keyword matching directly
	found := false
	for _, p := range goal.Predicates {
		if p == "test_result_known" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected test_result_known from keyword fallback, got %v", goal.Predicates)
	}
}

func TestClassifyGoalWithLLM_Caching(t *testing.T) {
	callCount := 0
	baseModel := &mockLLMModel{
		response: `{
			"predicates": ["file_content_known"],
			"confidence": 0.9,
			"reasoning": "test"
		}`,
	}

	// Create a wrapper that tracks calls
	model := &callTrackingModel{baseModel: baseModel, callCount: &callCount}

	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{Name: "ReadFile", Effects: []types.Predicate{"file_content_known"}})

	config := DefaultClassifierConfig()
	cache := NewGoalCache(10)
	config.Cache = cache

	// First call
	goal1 := ClassifyGoalWithLLM("read the file", model, registry, config)
	if len(goal1.Predicates) == 0 {
		t.Fatal("expected predicates from first call")
	}

	// Second call with same instruction should use cache
	goal2 := ClassifyGoalWithLLM("read the file", model, registry, config)
	if callCount != 1 {
		t.Fatalf("expected 1 model call due to caching, got %d", callCount)
	}
	if goal2.Description != goal1.Description {
		t.Error("cached goal should match first goal")
	}
}

type callTrackingModel struct {
	baseModel  *mockLLMModel
	callCount  *int
}

func (m *callTrackingModel) Generate(ctx context.Context, prompt string, opts *core.LLMOptions) (*core.LLMResponse, error) {
	*m.callCount++
	return m.baseModel.Generate(ctx, prompt, opts)
}

func (m *callTrackingModel) GenerateStream(ctx context.Context, prompt string, opts *core.LLMOptions) (<-chan string, error) {
	return m.baseModel.GenerateStream(ctx, prompt, opts)
}

func (m *callTrackingModel) Chat(ctx context.Context, messages []core.Message, opts *core.LLMOptions) (*core.LLMResponse, error) {
	return m.baseModel.Chat(ctx, messages, opts)
}

func (m *callTrackingModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, opts *core.LLMOptions) (*core.LLMResponse, error) {
	return m.baseModel.ChatWithTools(ctx, messages, tools, opts)
}

func TestGoalCache_Basic(t *testing.T) {
	cache := NewGoalCache(10)

	goal := &types.GoalCondition{
		Description: "test",
		Predicates:  []types.Predicate{"x", "y"},
	}

	cache.Set("instruction1", goal)
	retrieved := cache.Get("instruction1")

	if retrieved == nil {
		t.Fatal("expected to retrieve cached goal")
	}
	if len(retrieved.Predicates) != 2 {
		t.Errorf("expected 2 predicates, got %d", len(retrieved.Predicates))
	}

	missing := cache.Get("nonexistent")
	if missing != nil {
		t.Fatal("expected nil for nonexistent key")
	}
}

func TestGoalCache_Clear(t *testing.T) {
	cache := NewGoalCache(10)
	goal := &types.GoalCondition{
		Description: "test",
		Predicates:  []types.Predicate{"x"},
	}

	cache.Set("key1", goal)
	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}

	cache.Clear()
	if cache.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cache.Size())
	}
}

func TestClassificationPrompt_Parsing(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{
			name: "plain json",
			raw: `{
				"predicates": ["x"],
				"confidence": 0.9,
				"reasoning": "test"
			}`,
			wantErr: false,
		},
		{
			name: "json with markdown",
			raw: "```json\n{\n  \"predicates\": [\"x\"],\n  \"confidence\": 0.9,\n  \"reasoning\": \"test\"\n}\n```",
			wantErr: false,
		},
		{
			name:    "invalid json",
			raw:     `{broken json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseClassificationResponse(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("want error=%v, got error=%v", tt.wantErr, err)
			}
			if !tt.wantErr && resp == nil {
				t.Fatal("expected non-nil response")
			}
		})
	}
}

func TestClassifierConfig_DefaultValues(t *testing.T) {
	config := DefaultClassifierConfig()

	if !config.Enabled {
		t.Error("expected Enabled=true")
	}
	if config.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}
	if !config.FallbackOnFail {
		t.Error("expected FallbackOnFail=true")
	}
	if config.Cache == nil {
		t.Error("expected non-nil cache")
	}
	if config.MinConfidence < 0.4 || config.MinConfidence > 0.6 {
		t.Errorf("expected MinConfidence around 0.5, got %f", config.MinConfidence)
	}
}

func TestPredicatesFromRegistry(t *testing.T) {
	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{
		Name:          "A",
		Preconditions: []types.Predicate{"x"},
		Effects:       []types.Predicate{"y", "z"},
	})
	registry.Register(types.Operator{
		Name:          "B",
		Preconditions: []types.Predicate{"y"},
		Effects:       []types.Predicate{"w"},
	})

	predicates := PredicatesFromRegistry(registry)

	if len(predicates) != 4 {
		t.Fatalf("expected 4 unique predicates, got %d: %v", len(predicates), predicates)
	}
}

// Note: TestGoalConAgent_UsesLLMClassifier has been moved to goalcon_agent_test.go
// to avoid circular import with goalcon package
// func TestGoalConAgent_UsesLLMClassifier(t *testing.T) {
// ...
// }

var ErrTestModelFailed = goLangErrorType{"test model failed"}

type goLangErrorType struct{ msg string }

func (e goLangErrorType) Error() string { return e.msg }
