package analysis

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"context"
	"testing"

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

	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{Name: "ReadFile", Effects: []goalcon.types.Predicate{"file_content_known"}})
	registry.Register(goalcon.types.Operator{Name: "AnalyzeCode", Preconditions: []goalcon.types.Predicate{"file_content_known"}, Effects: []goalcon.types.Predicate{"edit_plan_known"}})
	registry.Register(goalcon.types.Operator{Name: "WriteFile", Preconditions: []goalcon.types.Predicate{"edit_plan_known"}, Effects: []goalcon.types.Predicate{"file_modified"}})

	config := goalcon.DefaultClassifierConfig()
	goal := goalcon.ClassifyGoalWithLLM("fix the bug", model, registry, config)

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

	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{Name: "ReadFile", Effects: []goalcon.types.Predicate{"file_content_known"}})

	config := goalcon.DefaultClassifierConfig()
	config.MinConfidence = 0.5 // Won't meet threshold
	goal := goalcon.ClassifyGoalWithLLM("fix something", model, registry, config)

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

	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{Name: "ReadFile", Effects: []goalcon.types.Predicate{"file_content_known"}})

	config := goalcon.DefaultClassifierConfig()
	config.FallbackOnFail = true

	goal := goalcon.ClassifyGoalWithLLM("analyze the code", model, registry, config)

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

	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{Name: "RunTests", Effects: []goalcon.types.Predicate{"test_result_known"}})

	config := goalcon.DefaultClassifierConfig()
	config.Enabled = false // Disable LLM classification

	goal := goalcon.ClassifyGoalWithLLM("run tests", model, registry, config)

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

	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{Name: "ReadFile", Effects: []goalcon.types.Predicate{"file_content_known"}})

	config := goalcon.DefaultClassifierConfig()
	cache := goalcon.NewGoalCache(10)
	config.Cache = cache

	// First call
	goal1 := goalcon.ClassifyGoalWithLLM("read the file", model, registry, config)
	if len(goal1.Predicates) == 0 {
		t.Fatal("expected predicates from first call")
	}

	// Second call with same instruction should use cache
	goal2 := goalcon.ClassifyGoalWithLLM("read the file", model, registry, config)
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
	cache := goalcon.NewGoalCache(10)

	goal := &goalcon.types.GoalCondition{
		Description: "test",
		Predicates:  []goalcon.types.Predicate{"x", "y"},
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
	cache := goalcon.NewGoalCache(10)
	goal := &goalcon.types.GoalCondition{
		Description: "test",
		Predicates:  []goalcon.types.Predicate{"x"},
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
			resp, err := goalcon.ParseClassificationResponse(tt.raw)
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
	config := goalcon.DefaultClassifierConfig()

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
	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{
		Name:          "A",
		Preconditions: []goalcon.types.Predicate{"x"},
		Effects:       []goalcon.types.Predicate{"y", "z"},
	})
	registry.Register(goalcon.types.Operator{
		Name:          "B",
		Preconditions: []goalcon.types.Predicate{"y"},
		Effects:       []goalcon.types.Predicate{"w"},
	})

	predicates := goalcon.PredicatesFromRegistry(registry)

	if len(predicates) != 4 {
		t.Fatalf("expected 4 unique predicates, got %d: %v", len(predicates), predicates)
	}
}

func TestGoalConAgent_UsesLLMClassifier(t *testing.T) {
	model := &mockLLMModel{
		response: `{
			"predicates": ["file_content_known", "edit_plan_known"],
			"confidence": 0.88,
			"reasoning": "task requires analysis"
		}`,
	}

	agent := &goalcon.GoalConAgent{
		Model:            model,
		Operators:        goalcon.DefaultOperatorRegistry(),
		ClassifierConfig: goalcon.DefaultClassifierConfig(),
	}

	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{Instruction: "analyze this code"}
	state := core.NewContext()

	_, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Check that goal was stored in context
	raw, ok := state.Get("goalcon.goal")
	if !ok {
		t.Fatal("expected goal in context")
	}

	goal, ok := raw.(goalcon.types.GoalCondition)
	if !ok {
		t.Fatal("expected types.GoalCondition type in context")
	}

	if len(goal.Predicates) == 0 {
		t.Error("expected predicates from LLM classifier")
	}
}

var ErrTestModelFailed = goLangErrorType{"test model failed"}

type goLangErrorType struct{ msg string }

func (e goLangErrorType) Error() string { return e.msg }
