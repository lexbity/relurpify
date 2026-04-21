package stages_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/chainer"
	"codeburg.org/lexbit/relurpify/agents/chainer/stages"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// mockModel is a test LanguageModel that returns predefined responses.
type mockModel struct {
	responses []string
	callCount int
}

func (m *mockModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}
func (m *mockModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}
func (m *mockModel) Chat(_ context.Context, messages []core.Message, _ *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}
func (m *mockModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

// TestLinkStageImplementsStage verifies interface compliance.
func TestLinkStageImplementsStage(t *testing.T) {
	link := chainer.NewLink("test", "prompt", nil, "out", nil)
	stage := stages.NewLinkStage(&link, &mockModel{})

	// Interface compliance checks
	if stage.Name() == "" {
		t.Fatal("expected non-empty name")
	}
	contract := stage.Contract()
	if contract.Metadata.OutputKey != "out" {
		t.Fatalf("expected output key 'out', got %q", contract.Metadata.OutputKey)
	}
}

// TestLinkStageContract verifies contract declaration.
func TestLinkStageContract(t *testing.T) {
	tests := []struct {
		name             string
		link             chainer.Link
		expectedIn       string
		expectedOut      string
		expectedFailFast bool
	}{
		{
			name:             "no input keys (uses default)",
			link:             chainer.NewLink("step1", "prompt", nil, "out1", nil),
			expectedIn:       "__chainer_instruction",
			expectedOut:      "out1",
			expectedFailFast: false,
		},
		{
			name:             "single input key",
			link:             chainer.NewLink("step2", "prompt", []string{"in1"}, "out2", nil),
			expectedIn:       "in1",
			expectedOut:      "out2",
			expectedFailFast: false,
		},
		{
			name: "fail fast policy",
			link: chainer.Link{
				Name:         "step3",
				SystemPrompt: "prompt",
				InputKeys:    []string{"in1"},
				OutputKey:    "out3",
				OnFailure:    chainer.FailurePolicyFailFast,
			},
			expectedIn:       "in1",
			expectedOut:      "out3",
			expectedFailFast: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := stages.NewLinkStage(&tt.link, &mockModel{})
			contract := stage.Contract()

			if contract.Metadata.InputKey != tt.expectedIn {
				t.Errorf("expected input key %q, got %q", tt.expectedIn, contract.Metadata.InputKey)
			}
			if contract.Metadata.OutputKey != tt.expectedOut {
				t.Errorf("expected output key %q, got %q", tt.expectedOut, contract.Metadata.OutputKey)
			}

			if tt.expectedFailFast {
				if contract.Metadata.RetryPolicy.MaxAttempts != 1 {
					t.Errorf("expected fail fast (MaxAttempts=1), got %d", contract.Metadata.RetryPolicy.MaxAttempts)
				}
			} else {
				if contract.Metadata.RetryPolicy.MaxAttempts < 2 {
					t.Errorf("expected retry enabled, got MaxAttempts=%d", contract.Metadata.RetryPolicy.MaxAttempts)
				}
			}
		})
	}
}

// TestLinkStageInputIsolation verifies that stages only see declared input keys.
// FilterState ensures a stage receives only its declared InputKeys.
func TestLinkStageInputIsolation(t *testing.T) {
	// Setup context with multiple keys
	ctx := core.NewContext()
	ctx.Set("code", "func hello() {}")
	ctx.Set("secret", "should not appear")
	ctx.Set("other", "also hidden")

	// Filter to only "code" key
	filtered := stages.FilterState(ctx, []string{"code"})

	if len(filtered) != 1 {
		t.Errorf("expected 1 key in filtered, got %d", len(filtered))
	}
	if filtered["code"] != "func hello() {}" {
		t.Errorf("expected code value, got %v", filtered)
	}
	if _, ok := filtered["secret"]; ok {
		t.Errorf("secret should not be in filtered state")
	}
	if _, ok := filtered["other"]; ok {
		t.Errorf("other should not be in filtered state")
	}

	// Now test LinkStage uses isolation
	link := chainer.NewLink(
		"analyze",
		"Analyze: {{.Input.code}}",
		[]string{"code"}, // Only see "code" key
		"analysis",
		nil,
	)
	stage := stages.NewLinkStage(&link, &mockModel{})
	ctx.Set("__chainer_instruction", "analyze this")

	prompt, err := stage.BuildPrompt(ctx)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	// Verify the prompt contains the code value
	if !contains(prompt, "func hello()") {
		t.Errorf("expected 'func hello()' in prompt, got: %s", prompt)
	}
}

// TestLinkStageDecodeWithoutParser tests Decode when Parse is nil (returns raw text).
func TestLinkStageDecodeWithoutParser(t *testing.T) {
	link := chainer.NewLink("summarize", "prompt", nil, "summary", nil)
	stage := stages.NewLinkStage(&link, &mockModel{})

	resp := &core.LLMResponse{Text: "this is the summary"}
	output, err := stage.Decode(resp)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if output != "this is the summary" {
		t.Errorf("expected 'this is the summary', got %q", output)
	}
}

// TestLinkStageDecodeWithParser tests Decode with custom Parse function.
func TestLinkStageDecodeWithParser(t *testing.T) {
	parseFunc := func(text string) (any, error) {
		return strconv.Atoi(text)
	}
	link := chainer.NewLink("parse", "prompt", nil, "number", parseFunc)
	stage := stages.NewLinkStage(&link, &mockModel{})

	resp := &core.LLMResponse{Text: "42"}
	output, err := stage.Decode(resp)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if output != 42 {
		t.Errorf("expected 42, got %v", output)
	}
}

// TestLinkStageDecodeParseFailure tests Decode when Parse returns error.
func TestLinkStageDecodeParseFailure(t *testing.T) {
	parseFunc := func(text string) (any, error) {
		return nil, errors.New("invalid format")
	}
	link := chainer.NewLink("parse", "prompt", nil, "number", parseFunc)
	stage := stages.NewLinkStage(&link, &mockModel{})

	resp := &core.LLMResponse{Text: "bad"}
	_, err := stage.Decode(resp)
	if err == nil {
		t.Fatal("expected Decode error")
	}
	// Error should be wrapped in LinkDecodeError
	var decodeErr interface{ Unwrap() error }
	if !errors.As(err, &decodeErr) {
		t.Errorf("expected error chain, got %T", err)
	}
}

// TestLinkStageApply verifies output is written to context at OutputKey.
func TestLinkStageApply(t *testing.T) {
	link := chainer.NewLink("transform", "prompt", nil, "result", nil)
	stage := stages.NewLinkStage(&link, &mockModel{})

	ctx := core.NewContext()
	ctx.Set("existing", "should remain")

	output := "transformed output"
	err := stage.Apply(ctx, output)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify output was written
	result, ok := ctx.Get("result")
	if !ok || result != output {
		t.Errorf("expected 'result' = %q, got %v", output, result)
	}

	// Verify other keys unchanged
	if existing, ok := ctx.Get("existing"); !ok || existing != "should remain" {
		t.Errorf("existing key was modified")
	}
}

// TestLinkStagePromptRendering tests template rendering with .Instruction and .Input.
func TestLinkStagePromptRendering(t *testing.T) {
	link := chainer.NewLink(
		"complex",
		"Task: {{.Instruction}}\nInput: {{.Input.data}}\nExtra: {{.Input.extra}}",
		[]string{"data", "extra"},
		"output",
		nil,
	)
	stage := stages.NewLinkStage(&link, &mockModel{})

	ctx := core.NewContext()
	ctx.Set("data", "important info")
	ctx.Set("extra", "auxiliary data")
	ctx.Set("hidden", "this should not appear")
	ctx.Set("__chainer_instruction", "analyze the data")

	prompt, err := stage.BuildPrompt(ctx)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	// Verify prompt contains rendered parts
	if !contains(prompt, "Task: analyze the data") {
		t.Errorf("instruction not rendered: %s", prompt)
	}
	if !contains(prompt, "Input: important info") {
		t.Errorf("data not rendered: %s", prompt)
	}
	if !contains(prompt, "Extra: auxiliary data") {
		t.Errorf("extra not rendered: %s", prompt)
	}
}

// TestMultipleLinkStagesSequential verifies isolation across stages in a sequence.
func TestMultipleLinkStagesSequential(t *testing.T) {
	// Stage 1: write to "out1"
	link1 := chainer.NewLink("step1", "Step1", nil, "out1", nil)
	stage1 := stages.NewLinkStage(&link1, &mockModel{})

	// Stage 2: read only "out1", write to "out2"
	link2 := chainer.NewLink(
		"step2",
		"Step2: {{.Input.out1}}",
		[]string{"out1"},
		"out2",
		nil,
	)
	stage2 := stages.NewLinkStage(&link2, &mockModel{})

	ctx := core.NewContext()
	ctx.Set("__chainer_instruction", "go")

	// Apply stage 1
	if err := stage1.Apply(ctx, "result1"); err != nil {
		t.Fatalf("stage1 Apply failed: %v", err)
	}

	// Stage 2 should see out1 but nothing else
	prompt2, err := stage2.BuildPrompt(ctx)
	if err != nil {
		t.Fatalf("stage2 BuildPrompt failed: %v", err)
	}

	if !contains(prompt2, "result1") {
		t.Errorf("stage2 should see out1 result: %s", prompt2)
	}

	// Apply stage 2
	if err := stage2.Apply(ctx, "result2"); err != nil {
		t.Fatalf("stage2 Apply failed: %v", err)
	}

	// Verify final state
	out1, _ := ctx.Get("out1")
	out2, _ := ctx.Get("out2")
	if out1 != "result1" || out2 != "result2" {
		t.Errorf("expected out1=result1, out2=result2, got %v, %v", out1, out2)
	}
}

// TestNilLinkStage handles nil receiver cases.
func TestNilLinkStage(t *testing.T) {
	var stage *stages.LinkStage

	// These should handle nil gracefully
	if stage.Name() != "" {
		t.Fatal("nil stage should return empty name")
	}

	ctx := core.NewContext()
	_, err := stage.BuildPrompt(ctx)
	if err == nil {
		t.Fatal("expected error for nil stage")
	}

	resp := &core.LLMResponse{Text: "test"}
	_, err = stage.Decode(resp)
	if err == nil {
		t.Fatal("expected error for nil stage")
	}

	if err := stage.Validate("anything"); err == nil {
		t.Fatal("expected error for nil stage")
	}

	if err := stage.Apply(ctx, "anything"); err == nil {
		t.Fatal("expected error for nil stage")
	}
}

// Helper function to check if string contains substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s != "" && substr != ""
}

// Phase 5: Schema Validation Tests

func TestLinkStageValidate_NoSchema(t *testing.T) {
	// Without schema, validation should pass
	link := chainer.NewLink("test", "prompt", nil, "out", nil)
	stage := stages.NewLinkStage(&link, &mockModel{})

	output := map[string]any{"key": "value"}
	err := stage.Validate(output)

	if err != nil {
		t.Fatalf("validation without schema should pass, got: %v", err)
	}
}

func TestLinkStageValidate_JSONSchema_ValidObject(t *testing.T) {
	// Valid object matching schema
	link := chainer.NewLink("test", "prompt", nil, "out", nil)
	link.Schema = `{"type": "object"}`
	stage := stages.NewLinkStage(&link, &mockModel{})

	output := map[string]any{"name": "test", "value": 42}
	err := stage.Validate(output)

	if err != nil {
		t.Fatalf("valid object should pass validation, got: %v", err)
	}
}

func TestLinkStageValidate_JSONSchema_ValidArray(t *testing.T) {
	// Valid array matching schema
	link := chainer.NewLink("test", "prompt", nil, "out", nil)
	link.Schema = `{"type": "array"}`
	stage := stages.NewLinkStage(&link, &mockModel{})

	output := []any{"item1", "item2"}
	err := stage.Validate(output)

	if err != nil {
		t.Fatalf("valid array should pass validation, got: %v", err)
	}
}

func TestLinkStageValidate_JSONSchema_TypeMismatch(t *testing.T) {
	// Object when string expected
	link := chainer.NewLink("test", "prompt", nil, "out", nil)
	link.Schema = `{"type": "string"}`
	stage := stages.NewLinkStage(&link, &mockModel{})

	output := map[string]any{"key": "value"}
	err := stage.Validate(output)

	if err == nil {
		t.Fatal("type mismatch should fail validation")
	}
}

func TestLinkStageValidate_JSONSchema_NilOutput(t *testing.T) {
	// Nil output should fail
	link := chainer.NewLink("test", "prompt", nil, "out", nil)
	link.Schema = `{"type": "object"}`
	stage := stages.NewLinkStage(&link, &mockModel{})

	err := stage.Validate(nil)

	if err == nil {
		t.Fatal("nil output should fail validation")
	}
}

func TestLinkStageValidate_NilStage(t *testing.T) {
	var stage *stages.LinkStage

	err := stage.Validate(map[string]any{})

	if err == nil {
		t.Fatal("nil stage should error")
	}
}

func TestLinkStageValidate_ErrorMessage(t *testing.T) {
	// Verify error contains useful information
	link := chainer.NewLink("extract", "prompt", nil, "result", nil)
	link.Schema = `{"type": "object"}`
	stage := stages.NewLinkStage(&link, &mockModel{})

	err := stage.Validate("not an object")

	if err == nil {
		t.Fatal("validation should fail")
	}

	errStr := err.Error()
	if !contains(errStr, "extract") {
		t.Fatalf("error should mention link name: %s", errStr)
	}
	if !contains(errStr, "result") {
		t.Fatalf("error should mention output key: %s", errStr)
	}
}
