package pipeline

import (
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

type stubStage struct {
	name        string
	contract    ContractDescriptor
	output      any
	decodeErr   error
	validateErr error
	applyErr    error
	applied     any
}

func (s *stubStage) Name() string { return s.name }

func (s *stubStage) Contract() ContractDescriptor { return s.contract }

func (s *stubStage) BuildPrompt(ctx *core.Context) (string, error) { return "prompt", nil }

func (s *stubStage) Decode(resp *core.LLMResponse) (any, error) {
	if s.decodeErr != nil {
		return nil, s.decodeErr
	}
	return s.output, nil
}

func (s *stubStage) Validate(output any) error {
	return s.validateErr
}

func (s *stubStage) Apply(ctx *core.Context, output any) error {
	s.applied = output
	return s.applyErr
}

func makeStubStage() *stubStage {
	return &stubStage{
		name: "analyze",
		contract: ContractDescriptor{
			Name: "issue-list",
			Metadata: ContractMetadata{
				InputKey:      "pipeline.input",
				OutputKey:     "pipeline.output",
				SchemaVersion: "v1",
			},
		},
		output: map[string]any{"issues": 1},
	}
}

func TestDecodeStageOutputWrapsDecodeFailures(t *testing.T) {
	stage := makeStubStage()
	stage.decodeErr = errors.New("bad json")

	_, err := DecodeStageOutput(stage, &core.LLMResponse{Text: "oops"})
	if err == nil {
		t.Fatal("expected decode error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected DecodeError, got %T", err)
	}
	if decodeErr.Stage != "analyze" || decodeErr.Contract != "issue-list" {
		t.Fatalf("unexpected decode error context: %+v", decodeErr)
	}
}

func TestValidateStageOutputWrapsValidationFailures(t *testing.T) {
	stage := makeStubStage()
	stage.validateErr = errors.New("missing issues")

	err := ValidateStageOutput(stage, stage.output)
	if err == nil {
		t.Fatal("expected validation error")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if validationErr.Stage != "analyze" || validationErr.Contract != "issue-list" {
		t.Fatalf("unexpected validation error context: %+v", validationErr)
	}
}

func TestApplyStageOutputReturnsApplyError(t *testing.T) {
	stage := makeStubStage()
	stage.applyErr = errors.New("write failed")

	err := ApplyStageOutput(stage, core.NewContext(), stage.output)
	if err == nil {
		t.Fatal("expected apply error")
	}
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) {
		t.Fatalf("expected ApplyError, got %T", err)
	}
	if applyErr.Stage != "analyze" || applyErr.Contract != "issue-list" {
		t.Fatalf("unexpected apply error context: %+v", applyErr)
	}
}

func TestApplyStageOutputPassesOutputToStage(t *testing.T) {
	stage := makeStubStage()
	ctx := core.NewContext()

	if err := ApplyStageOutput(stage, ctx, stage.output); err != nil {
		t.Fatalf("expected apply success, got %v", err)
	}
	if stage.applied == nil {
		t.Fatal("expected applied output to be recorded")
	}
}
