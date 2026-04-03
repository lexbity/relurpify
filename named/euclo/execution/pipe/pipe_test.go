package pipe

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

type stubStage struct {
	name   string
	output any
}

func (s *stubStage) Name() string { return s.name }

func (s *stubStage) Contract() frameworkpipeline.ContractDescriptor {
	return frameworkpipeline.ContractDescriptor{
		Name: s.name + "-contract",
		Metadata: frameworkpipeline.ContractMetadata{
			InputKey:      "in",
			OutputKey:     "out",
			SchemaVersion: "v1",
		},
	}
}

func (s *stubStage) BuildPrompt(*core.Context) (string, error) { return "prompt", nil }
func (s *stubStage) Decode(*core.LLMResponse) (any, error)     { return s.output, nil }
func (s *stubStage) Validate(any) error                        { return nil }
func (s *stubStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("pipeline.stage_output", output)
	return nil
}

func TestNewReturnsConfiguredRunner(t *testing.T) {
	env := testutil.Env(t)

	runner := New(env)
	if runner == nil {
		t.Fatal("expected runner")
	}
	if runner.Model != env.Model {
		t.Fatal("expected model to be wired from environment")
	}
	if runner.Tools != env.Registry {
		t.Fatal("expected registry to be wired from environment")
	}
}

func TestExecuteStagesRunsConfiguredStages(t *testing.T) {
	env := testutil.Env(t)
	stages := []frameworkpipeline.Stage{
		&stubStage{name: "analyze", output: map[string]any{"status": "ok"}},
	}

	state := core.NewContext()
	result, err := ExecuteStages(context.Background(), env, &core.Task{
		ID:          "pipeline-task",
		Instruction: "inspect state",
	}, state, stages)
	if err != nil {
		t.Fatalf("ExecuteStages: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := state.GetString("pipeline.results_summary"); got == "" {
		t.Fatal("expected pipeline results summary")
	}
}
