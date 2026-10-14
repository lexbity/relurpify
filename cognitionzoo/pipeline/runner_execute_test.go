package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
)

func TestRunnerExecuteRunsStagesInOrder(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")
	first := newFakeStage("explore")
	second := newFakeStage("plan")
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}

	results, err := runner.Execute(context.Background(), &execution.Task{ID: "t1"}, env, []Stage{first, second})
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "explore", results[0].StageName)
	assert.Equal(t, "plan", results[1].StageName)
	assert.True(t, results[0].ValidationOK)
	assert.True(t, results[1].ValidationOK)
	assert.Equal(t, TransitionNext, results[0].Transition.Kind)
	assert.Equal(t, "fake.contract", results[0].ContractName)
	assert.Equal(t, "1.0.0", results[0].ContractVersion)
	assert.Equal(t, "prompt for explore", results[0].Prompt)
	assert.False(t, results[0].StartedAt.IsZero())
	assert.False(t, results[0].FinishedAt.IsZero())

	// Both stages applied their decoded output in order.
	require.Len(t, first.applyCalls, 1)
	require.Len(t, second.applyCalls, 1)
}

func TestRunnerExecuteRequiresStagesAndModel(t *testing.T) {
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}
	_, err := runner.Execute(context.Background(), nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline stages required")

	_, err = (&Runner{}).Execute(context.Background(), nil, nil, []Stage{newFakeStage("s")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline runner model required")
}

func TestRunnerExecuteRejectsInvalidStage(t *testing.T) {
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}
	results, err := runner.Execute(context.Background(), nil, nil, []Stage{
		&fakeStage{name: "", contract: validContract()},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline stage name required")
	assert.Empty(t, results)
}

func TestRunnerExecuteBuildPromptErrorAbortsStage(t *testing.T) {
	broken := newFakeStage("explore")
	broken.promptErr = errBoom
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}

	results, err := runner.Execute(context.Background(), nil, nil, []Stage{broken})
	require.ErrorIs(t, err, errBoom)
	require.Len(t, results, 1)
	assert.Equal(t, errBoom.Error(), results[0].ErrorText)
	assert.False(t, results[0].FinishedAt.IsZero())
}

func TestRunnerExecuteGenerateErrorAbortsStage(t *testing.T) {
	broken := newFakeStage("explore")
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{generateErr: errBoom}}}

	results, err := runner.Execute(context.Background(), nil, nil, []Stage{broken})
	require.ErrorIs(t, err, errBoom)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].ErrorText, "boom")
}

func TestRunnerExecuteDecodeErrorRetriesThenStops(t *testing.T) {
	broken := newFakeStage("explore")
	broken.decodeErr = errors.New("bad json")
	broken.contract.Metadata.RetryPolicy = RetryPolicy{MaxAttempts: 1, RetryOnDecodeError: true}
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}

	results, err := runner.Execute(context.Background(), nil, nil, []Stage{broken})
	require.Error(t, err)
	var decodeErr *DecodeError
	require.ErrorAs(t, err, &decodeErr)
	require.Len(t, results, 1)
	assert.Equal(t, 1, results[0].RetryAttempt, "one retry consumed before stopping")
	assert.Equal(t, TransitionStop, results[0].Transition.Kind)
	assert.Contains(t, results[0].Transition.Reason, "bad json")
}

func TestRunnerExecuteDecodeErrorWithoutRetryStopsImmediately(t *testing.T) {
	broken := newFakeStage("explore")
	broken.decodeErr = errors.New("bad json")
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}

	results, err := runner.Execute(context.Background(), nil, nil, []Stage{broken})
	require.Error(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].RetryAttempt)
}

func TestRunnerExecuteValidationErrorRetriesThenSucceeds(t *testing.T) {
	stage := newFakeStage("explore")
	stage.validateErrs = []error{errors.New("not ready")}
	stage.contract.Metadata.RetryPolicy = RetryPolicy{MaxAttempts: 2, RetryOnValidationError: true}
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}

	results, err := runner.Execute(context.Background(), nil, nil, []Stage{stage})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].ValidationOK)
	assert.Equal(t, 1, results[0].RetryAttempt, "second attempt passed validation")
	assert.Equal(t, TransitionNext, results[0].Transition.Kind)
}

func TestRunnerExecuteValidationErrorExhaustsRetries(t *testing.T) {
	stage := newFakeStage("explore")
	stage.validateErr = errors.New("still invalid")
	stage.contract.Metadata.RetryPolicy = RetryPolicy{MaxAttempts: 1, RetryOnValidationError: true}
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}

	results, err := runner.Execute(context.Background(), nil, nil, []Stage{stage})
	require.Error(t, err)
	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Len(t, results, 1)
	assert.Equal(t, TransitionStop, results[0].Transition.Kind)
	assert.False(t, results[0].ValidationOK)
}

func TestRunnerExecuteApplyErrorStopsStage(t *testing.T) {
	stage := newFakeStage("explore")
	stage.applyErr = errBoom
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}

	results, err := runner.Execute(context.Background(), nil, nil, []Stage{stage})
	require.Error(t, err)
	var applyErr *ApplyError
	require.ErrorAs(t, err, &applyErr)
	require.Len(t, results, 1)
	assert.Equal(t, TransitionStop, results[0].Transition.Kind)
	assert.Contains(t, results[0].ErrorText, "boom")
}

func TestRunnerExecuteRecordsModelUsageOnEnvelope(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")
	stage := newFakeStage("explore")
	mdl := &fakeModel{generateResp: &model.LLMResponse{
		Text:  "done",
		Usage: model.TokenUsage{PromptTokens: 11, CompletionTokens: 7},
	}}
	runner := &Runner{Options: RunnerOptions{Model: mdl}}

	_, err := runner.Execute(context.Background(), nil, env, []Stage{stage})
	require.NoError(t, err)

	usage := contextdata.ModelUsageFromEnv(env)
	assert.Equal(t, 1, usage.Calls)
	assert.Equal(t, 11, usage.PromptTokens)
	assert.Equal(t, 7, usage.CompletionTokens)
}

func TestRunnerExecuteEmitsStageTelemetryEvents(t *testing.T) {
	sink := &recordingTelemetry{}
	stage := newFakeStage("explore")
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}, Telemetry: sink}}

	_, err := runner.Execute(context.Background(), nil, nil, []Stage{stage})
	require.NoError(t, err)

	events := sink.emitted()
	require.NotEmpty(t, events)
	assert.Equal(t, pipelineEventStageStart, events[0].Type)
	assert.Equal(t, "explore", events[0].NodeID)
	assert.Equal(t, 0, events[0].Metadata["stage_index"])
	assert.Equal(t, "fake.contract", events[0].Metadata["contract_name"])

	last := events[len(events)-1]
	assert.Equal(t, pipelineEventStageFinish, last.Type)
	assert.Equal(t, true, last.Metadata["validation_ok"])
	assert.Equal(t, TransitionNext, last.Metadata["transition"])
}

func TestRunnerExecuteToolRequiredStageFailsWithoutToolCall(t *testing.T) {
	stage := newFakeStage("explore")
	stage.allowTools = true
	stage.allowedToolNames = []string{"read"}
	stage.requiresTool = true
	stage.contract.Metadata.RetryPolicy = RetryPolicy{MaxAttempts: 1, RetryOnValidationError: false}
	mdl := &fakeModel{chatResp: &model.LLMResponse{Text: "no tools used"}}
	runner := &Runner{Options: RunnerOptions{
		Model:             mdl,
		Tools:             []ports.Tool{&fakeTool{name: "read"}},
		EnableToolCalling: true,
	}}

	results, err := runner.Execute(context.Background(), &execution.Task{ID: "t"}, nil, []Stage{stage})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a tool call")
	require.Len(t, results, 1)
	assert.Equal(t, TransitionStop, results[0].Transition.Kind)
	assert.GreaterOrEqual(t, mdl.chatCalls, 1, "tool-calling path must consult ChatWithTools")
}

func TestRunnerExecuteToolRequiredStageSucceedsWithToolCall(t *testing.T) {
	stage := newFakeStage("explore")
	stage.allowTools = true
	stage.allowedToolNames = []string{"read"}
	stage.requiresTool = true
	mdl := &fakeModel{chatResp: &model.LLMResponse{
		Text: "used a tool",
		ToolCalls: []model.ToolCall{
			{ID: "call-1", Name: "read", Args: map[string]any{"path": "main.go"}},
		},
	}}
	runner := &Runner{Options: RunnerOptions{
		Model:             mdl,
		Tools:             []ports.Tool{&fakeTool{name: "read"}},
		EnableToolCalling: true,
	}}

	results, err := runner.Execute(context.Background(), &execution.Task{ID: "t"}, nil, []Stage{stage})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].ValidationOK)
}

func TestRunnerExecuteToolRequiredCheckAppliesOnlyToToolRequiredStages(t *testing.T) {
	// A stage that does not implement ToolRequiredStage is never checked,
	// even with tool calling disabled and no tools resolved.
	plain := newFakeStage("explore")
	runner := &Runner{Options: RunnerOptions{Model: &fakeModel{}}}
	results, err := runner.Execute(context.Background(), nil, nil, []Stage{plain})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].ValidationOK)

	// A stage that does implement it fails when its tools cannot resolve:
	// the runner's tool-required gate fires even before any tool exists.
	demanding := newFakeStage("explore")
	demanding.requiresTool = true
	demanding.contract.Metadata.RetryPolicy = RetryPolicy{MaxAttempts: 0}
	results, err = runner.Execute(context.Background(), nil, nil, []Stage{demanding})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a tool call")
	require.Len(t, results, 1)
	assert.Equal(t, TransitionStop, results[0].Transition.Kind)
}

func TestGenerateStageResponsePlainPathWhenToolsNotAllowed(t *testing.T) {
	mdl := &fakeModel{generateResp: &model.LLMResponse{Text: "plain"}}
	runner := &Runner{Options: RunnerOptions{Model: mdl, ModelName: "test-model", EnableToolCalling: true}}

	stage := newFakeStage("explore") // AllowTools defaults false
	resp, usedTools, err := runner.generateStageResponse(
		context.Background(), nil, contextdata.NewEnvelope("t", "s"), stage, "prompt", []ports.Tool{&fakeTool{name: "read"}})
	require.NoError(t, err)
	assert.False(t, usedTools)
	assert.Equal(t, "plain", resp.Text)
	assert.Equal(t, 1, mdl.generateCalls)
	assert.Equal(t, 0, mdl.chatCalls)
}

func TestGenerateStageResponseToolPathWithoutCallsMarksUnused(t *testing.T) {
	mdl := &fakeModel{chatResp: &model.LLMResponse{Text: "answer"}}
	runner := &Runner{Options: RunnerOptions{Model: mdl, EnableToolCalling: true}}
	stage := newFakeStage("explore")
	stage.allowTools = true
	env := contextdata.NewEnvelope("t", "s")

	resp, usedTools, err := runner.generateStageResponse(
		context.Background(), nil, env, stage, "prompt", []ports.Tool{&fakeTool{name: "read"}})
	require.NoError(t, err)
	assert.False(t, usedTools, "no tool calls in response means tools were not used")
	assert.Equal(t, "answer", resp.Text)
	assert.Equal(t, 0, mdl.generateCalls)
	assert.Equal(t, 1, usageCalls(env), "chat path must record model usage")
}

func TestGenerateStageResponseToolPathErrorPropagates(t *testing.T) {
	mdl := &fakeModel{chatErr: errBoom}
	runner := &Runner{Options: RunnerOptions{Model: mdl, EnableToolCalling: true}}
	stage := newFakeStage("explore")
	stage.allowTools = true

	_, _, err := runner.generateStageResponse(
		context.Background(), nil, contextdata.NewEnvelope("t", "s"), stage, "prompt", []ports.Tool{&fakeTool{name: "read"}})
	require.ErrorIs(t, err, errBoom)
}

func usageCalls(env *contextdata.Envelope) int {
	return contextdata.ModelUsageFromEnv(env).Calls
}
