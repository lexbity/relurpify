package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type stubModel struct {
	responses     []*core.LLMResponse
	errs          []error
	toolResponses []*core.LLMResponse
	calls         int
	toolCalls     int
	prompts       []string
	toolPrompts   []string
	toolNamesSeen [][]string
}

func (m *stubModel) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	m.calls++
	m.prompts = append(m.prompts, prompt)
	idx := m.calls - 1
	if idx < len(m.errs) && m.errs[idx] != nil {
		return nil, m.errs[idx]
	}
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return &core.LLMResponse{Text: "{}"}, nil
}

func (m *stubModel) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (m *stubModel) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *stubModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	m.toolCalls++
	if len(messages) > 0 {
		m.toolPrompts = append(m.toolPrompts, messages[0].Content)
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	m.toolNamesSeen = append(m.toolNamesSeen, names)
	idx := m.toolCalls - 1
	if idx < len(m.toolResponses) {
		return m.toolResponses[idx], nil
	}
	return &core.LLMResponse{Text: "{}"}, nil
}

type recordingTelemetry struct {
	events []core.Event
}

func (t *recordingTelemetry) Emit(event core.Event) {
	t.events = append(t.events, event)
}

type memoryCheckpointStore struct {
	saved []*Checkpoint
}

func (m *memoryCheckpointStore) Save(checkpoint *Checkpoint) error {
	m.saved = append(m.saved, checkpoint)
	return nil
}

func (m *memoryCheckpointStore) Load(taskID, checkpointID string) (*Checkpoint, error) {
	for _, cp := range m.saved {
		if cp.TaskID == taskID && cp.CheckpointID == checkpointID {
			return cp, nil
		}
	}
	return nil, errors.New("not found")
}

type runnerStage struct {
	name          string
	contract      ContractDescriptor
	prompt        string
	output        any
	outputs       []any
	decodeErr     error
	decodeErrs    []error
	validateErr   error
	validateErrs  []error
	applyErr      error
	applyFn       func(ctx *core.Context, output any)
	decodeCalls   int
	validateCalls int
}

func (s *runnerStage) Name() string                                  { return s.name }
func (s *runnerStage) Contract() ContractDescriptor                  { return s.contract }
func (s *runnerStage) BuildPrompt(ctx *core.Context) (string, error) { return s.prompt, nil }
func (s *runnerStage) Decode(resp *core.LLMResponse) (any, error) {
	idx := s.decodeCalls
	s.decodeCalls++
	if idx < len(s.decodeErrs) && s.decodeErrs[idx] != nil {
		return nil, s.decodeErrs[idx]
	}
	if s.decodeErr != nil {
		return nil, s.decodeErr
	}
	if idx < len(s.outputs) {
		return s.outputs[idx], nil
	}
	return s.output, nil
}
func (s *runnerStage) Validate(output any) error {
	idx := s.validateCalls
	s.validateCalls++
	if idx < len(s.validateErrs) && s.validateErrs[idx] != nil {
		return s.validateErrs[idx]
	}
	return s.validateErr
}
func (s *runnerStage) Apply(ctx *core.Context, output any) error {
	if s.applyFn != nil {
		s.applyFn(ctx, output)
	}
	return s.applyErr
}

func (s *runnerStage) AllowedToolNames() []string { return nil }

func makeRunnerStage(name, inputKey, outputKey string, output any) *runnerStage {
	return &runnerStage{
		name:   name,
		prompt: name + " prompt",
		contract: ContractDescriptor{
			Name: name + "-contract",
			Metadata: ContractMetadata{
				InputKey:      inputKey,
				OutputKey:     outputKey,
				SchemaVersion: "v1",
			},
		},
		output: output,
	}
}

type toolStage struct {
	*runnerStage
	allowedTools []string
	requireTool  bool
}

func (s *toolStage) AllowedToolNames() []string {
	return append([]string{}, s.allowedTools...)
}

func (s *toolStage) RequiresToolExecution(task *core.Task, state *core.Context, tools []core.Tool) bool {
	return s.requireTool
}

type stubTool struct {
	name      string
	available bool
	result    *core.ToolResult
	calls     int
}

func (t *stubTool) Name() string        { return t.name }
func (t *stubTool) Description() string { return t.name + " description" }
func (t *stubTool) Category() string    { return "test" }
func (t *stubTool) Parameters() []core.ToolParameter {
	return nil
}
func (t *stubTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	t.calls++
	if t.result != nil {
		return t.result, nil
	}
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"name": t.name}}, nil
}
func (t *stubTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.available
}
func (t *stubTool) Permissions() core.ToolPermissions { return core.ToolPermissions{} }
func (t *stubTool) Tags() []string                    { return nil }

func TestRunnerExecuteHappyPath(t *testing.T) {
	model := &stubModel{responses: []*core.LLMResponse{{Text: `{"ok":1}`}, {Text: `{"ok":2}`}}}
	stage1 := makeRunnerStage("explore", "in", "stage1.out", map[string]any{"files": 1})
	stage1.applyFn = func(ctx *core.Context, output any) { ctx.Set("stage1.out", output) }
	stage2 := makeRunnerStage("analyze", "stage1.out", "stage2.out", map[string]any{"issues": 2})
	stage2.applyFn = func(ctx *core.Context, output any) { ctx.Set("stage2.out", output) }
	telemetry := &recordingTelemetry{}

	runner := &Runner{Options: RunnerOptions{Model: model, Telemetry: telemetry, ModelName: "test-model"}}
	state := core.NewContext()
	results, err := runner.Execute(context.Background(), &core.Task{ID: "task-1"}, state, []Stage{stage1, stage2})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if got := state.GetString("stage2.out"); got == "" {
		t.Fatalf("expected stage2 output in context")
	}
	if len(telemetry.events) != 4 {
		t.Fatalf("expected 4 telemetry events, got %d", len(telemetry.events))
	}
}

func TestRunnerExecuteStopsOnDecodeFailure(t *testing.T) {
	model := &stubModel{responses: []*core.LLMResponse{{Text: `oops`}}}
	stage := makeRunnerStage("analyze", "in", "out", nil)
	stage.decodeErr = errors.New("bad json")

	runner := &Runner{Options: RunnerOptions{Model: model}}
	results, err := runner.Execute(context.Background(), &core.Task{ID: "task-2"}, core.NewContext(), []Stage{stage})
	if err == nil {
		t.Fatal("expected decode failure")
	}
	if len(results) != 1 {
		t.Fatalf("expected one stage result, got %d", len(results))
	}
}

func TestRunnerExecuteStopsOnValidationFailure(t *testing.T) {
	model := &stubModel{responses: []*core.LLMResponse{{Text: `{}`}}}
	stage := makeRunnerStage("analyze", "in", "out", map[string]any{"issues": 0})
	stage.validateErr = errors.New("missing issues")

	runner := &Runner{Options: RunnerOptions{Model: model}}
	results, err := runner.Execute(context.Background(), &core.Task{ID: "task-3"}, core.NewContext(), []Stage{stage})
	if err == nil {
		t.Fatal("expected validation failure")
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
}

func TestRunnerExecuteCheckpointsAfterStages(t *testing.T) {
	model := &stubModel{responses: []*core.LLMResponse{{Text: `{}`}, {Text: `{}`}}}
	stage1 := makeRunnerStage("explore", "in", "a", map[string]any{"a": 1})
	stage2 := makeRunnerStage("analyze", "a", "b", map[string]any{"b": 2})
	store := &memoryCheckpointStore{}

	runner := &Runner{Options: RunnerOptions{
		Model:                model,
		CheckpointStore:      store,
		CheckpointAfterStage: true,
	}}
	results, err := runner.Execute(context.Background(), &core.Task{ID: "task-4"}, core.NewContext(), []Stage{stage1, stage2})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if len(store.saved) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(store.saved))
	}
}

func TestRunnerExecuteResumesAfterCheckpoint(t *testing.T) {
	model := &stubModel{responses: []*core.LLMResponse{{Text: `{}`}}}
	stage1 := makeRunnerStage("explore", "in", "a", map[string]any{"a": 1})
	stage2 := makeRunnerStage("analyze", "a", "b", map[string]any{"b": 2})
	stage2.applyFn = func(ctx *core.Context, output any) { ctx.Set("b", output) }

	checkpoint := &Checkpoint{
		CheckpointID: "cp-1",
		TaskID:       "task-5",
		StageName:    "explore",
		StageIndex:   0,
		CreatedAt:    stageTime(),
		Context:      core.NewContext(),
		Result:       StageResult{StageName: "explore", ContractName: "explore-contract", ContractVersion: "v1"},
	}
	checkpoint.Context.Set("a", map[string]any{"a": 1})

	runner := &Runner{Options: RunnerOptions{
		Model:            model,
		ResumeCheckpoint: checkpoint,
	}}
	results, err := runner.Execute(context.Background(), &core.Task{ID: "task-5"}, core.NewContext(), []Stage{stage1, stage2})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected resumed result set of 2, got %d", len(results))
	}
	if model.calls != 1 {
		t.Fatalf("expected resumed execution to skip first stage, got calls=%d", model.calls)
	}
}

func TestRunnerExecuteRetriesDecodeFailureWhenEnabled(t *testing.T) {
	model := &stubModel{responses: []*core.LLMResponse{{Text: `bad`}, {Text: `{}`}}}
	stage := makeRunnerStage("analyze", "in", "out", map[string]any{"issues": 1})
	stage.contract.Metadata.RetryPolicy = RetryPolicy{
		MaxAttempts:        1,
		RetryOnDecodeError: true,
	}
	stage.decodeErrs = []error{errors.New("bad json"), nil}

	runner := &Runner{Options: RunnerOptions{Model: model}}
	results, err := runner.Execute(context.Background(), &core.Task{ID: "task-6"}, core.NewContext(), []Stage{stage})
	if err != nil {
		t.Fatalf("expected decode retry success, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if got := results[0].RetryAttempt; got != 1 {
		t.Fatalf("expected one retry, got %d", got)
	}
	if model.calls != 2 {
		t.Fatalf("expected two model calls, got %d", model.calls)
	}
}

func TestRunnerExecuteRetriesValidationFailureWhenEnabled(t *testing.T) {
	model := &stubModel{responses: []*core.LLMResponse{{Text: `{}`}, {Text: `{}`}}}
	stage := makeRunnerStage("analyze", "in", "out", map[string]any{"issues": 1})
	stage.contract.Metadata.RetryPolicy = RetryPolicy{
		MaxAttempts:            1,
		RetryOnValidationError: true,
	}
	stage.validateErrs = []error{errors.New("missing issues"), nil}

	runner := &Runner{Options: RunnerOptions{Model: model}}
	results, err := runner.Execute(context.Background(), &core.Task{ID: "task-7"}, core.NewContext(), []Stage{stage})
	if err != nil {
		t.Fatalf("expected validation retry success, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if got := results[0].RetryAttempt; got != 1 {
		t.Fatalf("expected one retry, got %d", got)
	}
	if model.calls != 2 {
		t.Fatalf("expected two model calls, got %d", model.calls)
	}
}

func TestRunnerExecuteDoesNotRetryDecodeFailureWhenDisabled(t *testing.T) {
	model := &stubModel{responses: []*core.LLMResponse{{Text: `bad`}}}
	stage := makeRunnerStage("analyze", "in", "out", map[string]any{"issues": 1})
	stage.contract.Metadata.RetryPolicy = RetryPolicy{
		MaxAttempts:        2,
		RetryOnDecodeError: false,
	}
	stage.decodeErrs = []error{errors.New("bad json")}

	runner := &Runner{Options: RunnerOptions{Model: model}}
	results, err := runner.Execute(context.Background(), &core.Task{ID: "task-8"}, core.NewContext(), []Stage{stage})
	if err == nil {
		t.Fatal("expected decode failure")
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if model.calls != 1 {
		t.Fatalf("expected one model call, got %d", model.calls)
	}
	if got := results[0].RetryAttempt; got != 0 {
		t.Fatalf("expected zero retries, got %d", got)
	}
}

func TestRunnerExecuteFiltersStageTools(t *testing.T) {
	model := &stubModel{
		toolResponses: []*core.LLMResponse{{Text: `{"summary":"explored"}`}},
		responses:     []*core.LLMResponse{{Text: `{}`}},
	}
	stage := &toolStage{
		runnerStage: makeRunnerStage("explore", "in", "out", map[string]any{"files": 1}),
		allowedTools: []string{
			"file_read",
			"query_ast",
		},
	}
	stage.contract.Metadata.AllowTools = true
	toolA := &stubTool{name: "file_read", available: true}
	toolB := &stubTool{name: "query_ast", available: true}
	toolC := &stubTool{name: "file_write", available: true}

	runner := &Runner{Options: RunnerOptions{
		Model:             model,
		Tools:             []core.Tool{toolA, toolB, toolC},
		EnableToolCalling: true,
	}}
	_, err := runner.Execute(context.Background(), &core.Task{ID: "task-tools"}, core.NewContext(), []Stage{stage})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(model.toolNamesSeen) != 1 {
		t.Fatalf("expected one tool-calling request, got %d", len(model.toolNamesSeen))
	}
	if got := model.toolNamesSeen[0]; len(got) != 2 || got[0] != "file_read" || got[1] != "query_ast" {
		t.Fatalf("unexpected tool set: %#v", got)
	}
}

func TestRunnerExecuteUsesToolResultsForFinalResponse(t *testing.T) {
	model := &stubModel{
		toolResponses: []*core.LLMResponse{{
			ToolCalls: []core.ToolCall{{
				Name: "file_read",
				Args: map[string]any{"path": "main.rs"},
			}},
		}},
		responses: []*core.LLMResponse{{Text: `{}`}},
	}
	stage := &toolStage{
		runnerStage: makeRunnerStage("verify", "in", "out", map[string]any{"status": "pass"}),
		allowedTools: []string{
			"file_read",
		},
	}
	stage.contract.Metadata.AllowTools = true
	tool := &stubTool{
		name:      "file_read",
		available: true,
		result: &core.ToolResult{
			Success: true,
			Data:    map[string]interface{}{"content": "fn main() {}"},
		},
	}

	runner := &Runner{Options: RunnerOptions{
		Model:             model,
		Tools:             []core.Tool{tool},
		EnableToolCalling: true,
	}}
	_, err := runner.Execute(context.Background(), &core.Task{ID: "task-tool-observation"}, core.NewContext(), []Stage{stage})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(model.prompts) == 0 {
		t.Fatalf("expected final generation prompt after tool execution")
	}
	got := model.prompts[len(model.prompts)-1]
	if !strings.Contains(got, "Tool results:") || !strings.Contains(got, "file_read") {
		t.Fatalf("expected final prompt to include tool results, got %q", got)
	}
}

func TestRunnerExecuteRetriesToolRequiredStageWithForcedToolPrompt(t *testing.T) {
	model := &stubModel{
		toolResponses: []*core.LLMResponse{
			{Text: `{"status":"pass","checks":[{"name":"cli_cargo","status":"pass"}]}`},
			{ToolCalls: []core.ToolCall{{Name: "cli_cargo"}}},
		},
		responses: []*core.LLMResponse{{Text: `{"status":"pass","summary":"verified"}`}},
	}
	stage := &toolStage{
		runnerStage: makeRunnerStage("verify", "in", "out", map[string]any{"status": "pass"}),
		allowedTools: []string{
			"cli_cargo",
		},
		requireTool: true,
	}
	stage.contract.Metadata.AllowTools = true
	tool := &stubTool{name: "cli_cargo", available: true}

	runner := &Runner{Options: RunnerOptions{
		Model:             model,
		Tools:             []core.Tool{tool},
		EnableToolCalling: true,
	}}
	_, err := runner.Execute(context.Background(), &core.Task{ID: "task-tool-retry"}, core.NewContext(), []Stage{stage})
	if err != nil {
		t.Fatalf("expected success after forced tool prompt, got %v", err)
	}
	if len(model.toolPrompts) < 2 {
		t.Fatalf("expected retry tool prompt, got %d prompts", len(model.toolPrompts))
	}
	if !strings.Contains(model.toolPrompts[1], "Return a tool call now, not the final report.") {
		t.Fatalf("expected forced tool-call retry prompt, got %q", model.toolPrompts[1])
	}
}

func TestRunnerExecuteParsesToolCallsFromTextResponse(t *testing.T) {
	model := &stubModel{
		toolResponses: []*core.LLMResponse{{
			Text: `{"name":"file_read","arguments":{"path":"main.rs"}}`,
		}},
		responses: []*core.LLMResponse{{Text: `{}`}},
	}
	stage := &toolStage{
		runnerStage: makeRunnerStage("verify", "in", "out", map[string]any{"status": "pass"}),
		allowedTools: []string{
			"file_read",
		},
	}
	stage.contract.Metadata.AllowTools = true
	tool := &stubTool{
		name:      "file_read",
		available: true,
		result: &core.ToolResult{
			Success: true,
			Data:    map[string]interface{}{"content": "fn main() {}"},
		},
	}

	runner := &Runner{Options: RunnerOptions{
		Model:             model,
		Tools:             []core.Tool{tool},
		EnableToolCalling: true,
	}}
	_, err := runner.Execute(context.Background(), &core.Task{ID: "task-tool-text"}, core.NewContext(), []Stage{stage})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(model.prompts) == 0 || !strings.Contains(model.prompts[len(model.prompts)-1], "Tool results:") {
		t.Fatalf("expected parsed text tool call to trigger final prompt")
	}
}

type recordingCapabilityInvoker struct {
	names []string
	args  []map[string]any
}

func (r *recordingCapabilityInvoker) InvokeCapability(ctx context.Context, state *core.Context, idOrName string, args map[string]any) (*core.ToolResult, error) {
	r.names = append(r.names, idOrName)
	r.args = append(r.args, args)
	return &core.ToolResult{Success: true, Data: map[string]any{"invoked": idOrName}}, nil
}

func TestRunnerExecuteRoutesToolCallsThroughCapabilityInvoker(t *testing.T) {
	model := &stubModel{
		toolResponses: []*core.LLMResponse{{
			ToolCalls: []core.ToolCall{{
				Name: "file_read",
				Args: map[string]any{"path": "main.rs"},
			}},
		}},
		responses: []*core.LLMResponse{{Text: `{}`}},
	}
	stage := &toolStage{
		runnerStage: makeRunnerStage("verify", "in", "out", map[string]any{"status": "pass"}),
		allowedTools: []string{
			"file_read",
		},
	}
	stage.contract.Metadata.AllowTools = true
	tool := &stubTool{name: "file_read", available: true}
	invoker := &recordingCapabilityInvoker{}

	runner := &Runner{Options: RunnerOptions{
		Model:             model,
		Tools:             []core.Tool{tool},
		EnableToolCalling: true,
		CapabilityInvoker: invoker,
	}}
	_, err := runner.Execute(context.Background(), &core.Task{ID: "task-tool-invoker"}, core.NewContext(), []Stage{stage})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if tool.calls != 0 {
		t.Fatalf("expected direct tool execution to be bypassed, got calls=%d", tool.calls)
	}
	if len(invoker.names) != 1 || invoker.names[0] != "file_read" {
		t.Fatalf("unexpected invoker calls: %#v", invoker.names)
	}
}

func TestRunnerExecuteRepromptsWhenRequiredToolCallIsMissing(t *testing.T) {
	model := &stubModel{
		toolResponses: []*core.LLMResponse{
			{Text: `{"status":"pass"}`},
			{ToolCalls: []core.ToolCall{{
				Name: "cli_cargo",
				Args: map[string]any{"args": []any{"test"}},
			}}},
		},
		responses: []*core.LLMResponse{{Text: `{"status":"pass"}`}},
	}
	stage := &toolStage{
		runnerStage: makeRunnerStage("verify", "in", "out", map[string]any{"status": "pass"}),
		allowedTools: []string{
			"cli_cargo",
		},
		requireTool: true,
	}
	stage.contract.Metadata.AllowTools = true
	tool := &stubTool{
		name:      "cli_cargo",
		available: true,
		result: &core.ToolResult{
			Success: true,
			Data:    map[string]interface{}{"stdout": "test result: ok"},
		},
	}

	runner := &Runner{Options: RunnerOptions{
		Model:             model,
		Tools:             []core.Tool{tool},
		EnableToolCalling: true,
	}}
	_, err := runner.Execute(context.Background(), &core.Task{ID: "task-required", Instruction: "Run cli_cargo args [\"test\"]"}, core.NewContext(), []Stage{stage})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if model.toolCalls != 2 {
		t.Fatalf("expected second tool prompt after missing required tool call, got %d", model.toolCalls)
	}
	if len(model.prompts) == 0 || !strings.Contains(model.prompts[len(model.prompts)-1], "Tool results:") {
		t.Fatalf("expected final prompt with tool results after reprompt")
	}
}

func stageTime() time.Time {
	return time.Unix(1, 0).UTC()
}
