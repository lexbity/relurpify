package chainer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents/chainer/checkpoint"
	chaintelemetry "github.com/lexcodex/relurpify/agents/chainer/telemetry"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
)

type testLanguageModel struct {
	generateResponses []string
	chatResponses     []string

	generateCalls   int
	chatCalls       int
	chatToolsCalls  int
	lastPrompt      string
	lastMessages    [][]core.Message
	lastToolSpecs   [][]core.LLMToolSpec
	lastToolPrompts []string
	generateErr     error
	chatErr         error
}

func (m *testLanguageModel) Generate(_ context.Context, prompt string, _ *core.LLMOptions) (*core.LLMResponse, error) {
	m.lastPrompt = prompt
	m.generateCalls++
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	if idx := m.generateCalls - 1; idx < len(m.generateResponses) {
		return &core.LLMResponse{Text: m.generateResponses[idx]}, nil
	}
	return &core.LLMResponse{Text: ""}, nil
}

func (m *testLanguageModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, nil
}

func (m *testLanguageModel) Chat(_ context.Context, messages []core.Message, _ *core.LLMOptions) (*core.LLMResponse, error) {
	copied := make([]core.Message, len(messages))
	copy(copied, messages)
	m.lastMessages = append(m.lastMessages, copied)
	m.chatCalls++
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	if idx := m.chatCalls - 1; idx < len(m.chatResponses) {
		return &core.LLMResponse{Text: m.chatResponses[idx]}, nil
	}
	return &core.LLMResponse{Text: ""}, nil
}

func (m *testLanguageModel) ChatWithTools(_ context.Context, messages []core.Message, tools []core.LLMToolSpec, _ *core.LLMOptions) (*core.LLMResponse, error) {
	copied := make([]core.Message, len(messages))
	copy(copied, messages)
	m.lastMessages = append(m.lastMessages, copied)
	copiedTools := make([]core.LLMToolSpec, len(tools))
	copy(copiedTools, tools)
	m.lastToolSpecs = append(m.lastToolSpecs, copiedTools)
	if len(messages) > 0 {
		m.lastToolPrompts = append(m.lastToolPrompts, messages[0].Content)
	}
	m.chatToolsCalls++
	if idx := m.chatToolsCalls - 1; idx < len(m.chatResponses) {
		return &core.LLMResponse{Text: m.chatResponses[idx]}, nil
	}
	return &core.LLMResponse{Text: ""}, nil
}

type recordingCheckpointStore struct {
	saves []*frameworkpipeline.Checkpoint
}

func (s *recordingCheckpointStore) Save(cp *frameworkpipeline.Checkpoint) error {
	s.saves = append(s.saves, cp)
	return nil
}

func (s *recordingCheckpointStore) Load(taskID, checkpointID string) (*frameworkpipeline.Checkpoint, error) {
	for _, cp := range s.saves {
		if cp.TaskID == taskID && cp.CheckpointID == checkpointID {
			return cp, nil
		}
	}
	return nil, fmt.Errorf("checkpoint not found")
}

func TestLinkConstructionAndValidation(t *testing.T) {
	inputKeys := []string{"first", "second"}
	link := NewLink("Example", "Prompt", inputKeys, "output", nil)
	inputKeys[0] = "mutated"

	if link.InputKeys[0] != "first" {
		t.Fatalf("expected input keys to be copied, got %+v", link.InputKeys)
	}
	if link.OnFailure != FailurePolicyRetry {
		t.Fatalf("expected retry policy, got %s", link.OnFailure)
	}
	if link.MaxRetries != 1 {
		t.Fatalf("expected one retry, got %d", link.MaxRetries)
	}

	summarize := NewSummarizeLink("summary", []string{"in"}, "out")
	if summarize.SystemPrompt == "" || summarize.Parse != nil {
		t.Fatalf("unexpected summarize link: %+v", summarize)
	}

	transform := NewTransformLink("transform", []string{"in"}, "out", func(text string) (any, error) { return strings.ToUpper(text), nil })
	if transform.Parse == nil {
		t.Fatal("expected transform link parser")
	}

	var nilChain *Chain
	if err := nilChain.Validate(); err == nil {
		t.Fatal("expected nil chain validation error")
	}
	if err := (&Chain{Links: []Link{{OutputKey: "x"}}}).Validate(); err == nil {
		t.Fatal("expected empty name error")
	}
	if err := (&Chain{Links: []Link{{Name: "x"}}}).Validate(); err == nil {
		t.Fatal("expected empty output key error")
	}
	if err := (&Chain{Links: []Link{{Name: "x", OutputKey: "x", InputKeys: []string{"x"}}}}).Validate(); err == nil {
		t.Fatal("expected self reference error")
	}
	if err := (&Chain{Links: []Link{link}}).Validate(); err != nil {
		t.Fatalf("expected valid chain, got %v", err)
	}
}

func TestRunnerHelpers(t *testing.T) {
	if filtered := FilterState(nil, []string{"missing"}); len(filtered) != 0 {
		t.Fatalf("expected empty filtered state, got %+v", filtered)
	}

	state := core.NewContext()
	state.Set("a", 1)
	state.Set("b", 2)
	filtered := FilterState(state, []string{"a", "c"})
	if len(filtered) != 1 || filtered["a"] != 1 {
		t.Fatalf("unexpected filtered state: %+v", filtered)
	}

	if got := taskInstruction(nil); got != "" {
		t.Fatalf("expected empty instruction, got %q", got)
	}
	if got := taskInstruction(&core.Task{Instruction: "go"}); got != "go" {
		t.Fatalf("unexpected instruction %q", got)
	}

	if got := linkFailurePolicy(Link{}); got != FailurePolicyRetry {
		t.Fatalf("expected default retry policy, got %s", got)
	}
	if got := linkFailurePolicy(Link{OnFailure: FailurePolicyFailFast}); got != FailurePolicyFailFast {
		t.Fatalf("unexpected failure policy %s", got)
	}

	if got, err := parseLinkResponse(Link{}, "raw"); err != nil || got != "raw" {
		t.Fatalf("unexpected parse result: %v %v", got, err)
	}
	if got, err := parseLinkResponse(Link{Parse: func(text string) (any, error) { return strings.ToUpper(text), nil }}, "raw"); err != nil || got != "RAW" {
		t.Fatalf("unexpected parsed result: %v %v", got, err)
	}

	rendered, err := renderLinkPrompt("{{.Instruction}}/{{.Input.a}}", "task", map[string]any{"a": "value"})
	if err != nil {
		t.Fatalf("renderLinkPrompt: %v", err)
	}
	if rendered != "task/value" {
		t.Fatalf("unexpected rendered prompt %q", rendered)
	}
	if _, err := renderLinkPrompt("{{.Instruction", "task", nil); err == nil {
		t.Fatal("expected template parse error")
	}
}

func TestChainRunner_RunPaths(t *testing.T) {
	task := &core.Task{Instruction: "Do the thing"}

	var nilRunner *chainRunner
	if err := nilRunner.Run(context.Background(), task, &Chain{Links: []Link{NewLink("one", "Prompt", nil, "out", nil)}}, core.NewContext()); err == nil || err.Error() != "chainer: model unavailable" {
		t.Fatalf("expected unavailable model error, got %v", err)
	}

	if err := (&chainRunner{}).Run(context.Background(), task, &Chain{Links: []Link{NewLink("one", "Prompt", nil, "out", nil)}}, core.NewContext()); err == nil {
		t.Fatal("expected unavailable model error")
	}

	if err := (&chainRunner{Model: &testLanguageModel{}}).Run(context.Background(), task, &Chain{Links: []Link{{OutputKey: "out"}}}, core.NewContext()); err == nil {
		t.Fatal("expected validation error")
	}

	modelErr := &testLanguageModel{chatErr: errors.New("chat failed")}
	err := (&chainRunner{Model: modelErr}).Run(context.Background(), task, &Chain{Links: []Link{NewLink("one", "Prompt", nil, "out", nil)}}, core.NewContext())
	if err == nil || !strings.Contains(err.Error(), "chat failed") {
		t.Fatalf("expected chat error, got %v", err)
	}

	parseErrModel := &testLanguageModel{chatResponses: []string{"bad"}}
	chain := &Chain{Links: []Link{{
		Name:         "one",
		SystemPrompt: "Prompt",
		OutputKey:    "out",
		OnFailure:    FailurePolicyFailFast,
		Parse:        func(string) (any, error) { return nil, errors.New("parse failed") },
		MaxRetries:   1,
	}}}
	err = (&chainRunner{Model: parseErrModel}).Run(context.Background(), task, chain, core.NewContext())
	if !errors.Is(err, ErrLinkParseFailure) {
		t.Fatalf("expected parse failure error, got %v", err)
	}

	retryModel := &testLanguageModel{chatResponses: []string{"bad", "good"}}
	retryState := core.NewContext()
	retryChain := &Chain{Links: []Link{{
		Name:         "retry",
		SystemPrompt: "Prompt",
		OutputKey:    "out",
		Parse: func(text string) (any, error) {
			if text == "bad" {
				return nil, errors.New("bad parse")
			}
			return text, nil
		},
		OnFailure:  FailurePolicyRetry,
		MaxRetries: 1,
	}}}
	if err := (&chainRunner{Model: retryModel}).Run(context.Background(), task, retryChain, retryState); err != nil {
		t.Fatalf("retry run: %v", err)
	}
	if got := retryState.GetString("out"); got != "good" {
		t.Fatalf("unexpected output %q", got)
	}

	successState := core.NewContext()
	successChain := &Chain{Links: []Link{NewLink("one", "Prompt {{.Instruction}}", nil, "out", nil)}}
	okModel := &testLanguageModel{chatResponses: []string{"hello"}}
	if err := RunChain(context.Background(), okModel, task, successChain, successState); err != nil {
		t.Fatalf("RunChain: %v", err)
	}
	if got := successState.GetString("out"); got != "hello" {
		t.Fatalf("unexpected output %q", got)
	}
	if okModel.chatCalls != 1 {
		t.Fatalf("expected one chat call, got %d", okModel.chatCalls)
	}
}

func TestChainerAgentOptionsAndLegacyExecution(t *testing.T) {
	chain := &Chain{Links: []Link{
		NewLink("first link", "first {{.Instruction}}", nil, "out.one", nil),
		NewLink("second-link", "second {{.Input.out.one}}", []string{"out.one"}, "out.two", nil),
	}}
	env := agentEnvForTests()

	agent := New(env, WithChain(chain), WithChainBuilder(func(task *core.Task) (*Chain, error) {
		return chain, nil
	}))
	if agent.Chain != chain {
		t.Fatal("expected chain option to be applied")
	}
	if agent.ChainBuilder == nil {
		t.Fatal("expected chain builder option")
	}
	if agent.Model == nil || agent.Tools == nil || agent.Config == nil {
		t.Fatal("expected environment fields to be initialized")
	}
	if got := agent.CapabilityRegistry(); got != agent.Tools {
		t.Fatalf("unexpected capability registry: %v", got)
	}
	var nilAgent *ChainerAgent
	if nilAgent.CapabilityRegistry() != nil {
		t.Fatal("expected nil capability registry for nil agent")
	}

	if err := agent.InitializeEnvironment(env); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	if err := agent.Initialize(&core.Config{Model: "test-model"}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{Instruction: "go"}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := state.GetString("out.one"); got != "first" {
		t.Fatalf("unexpected first output %q", got)
	}
	if got := state.GetString("out.two"); got != "second" {
		t.Fatalf("unexpected second output %q", got)
	}
	if got := state.GetString("chainer.links_executed"); got != "2" {
		t.Fatalf("unexpected links executed %q", got)
	}

	graph, err := agent.BuildGraph(&core.Task{Instruction: "go"})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if graph == nil {
		t.Fatal("expected graph")
	}
	if got := sanitizeLinkName("  My-Link  "); got != "my_link" {
		t.Fatalf("unexpected sanitized name %q", got)
	}
	if got := sanitizeLinkName(""); got != "link" {
		t.Fatalf("unexpected fallback name %q", got)
	}
}

func TestChainerAgentPipelineCheckpointAndResume(t *testing.T) {
	t.Run("save checkpoint and collect outputs", func(t *testing.T) {
		model := &testLanguageModel{generateResponses: []string{"pipeline output"}}
		store := &recordingCheckpointStore{}
		agent := &ChainerAgent{
			Model:                model,
			Chain:                &Chain{Links: []Link{NewLink("stage one", "Hello {{.Instruction}}", nil, "out", nil)}},
			CheckpointStore:      store,
			CheckpointAfterStage: true,
		}
		if err := agent.Initialize(&core.Config{Model: "test-model"}); err != nil {
			t.Fatalf("Initialize: %v", err)
		}

		state := core.NewContext()
		result, err := agent.Execute(context.Background(), &core.Task{ID: "task-save", Instruction: "do it"}, state)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !result.Success {
			t.Fatalf("unexpected result: %+v", result)
		}
		if model.generateCalls != 1 {
			t.Fatalf("expected one generate call, got %d", model.generateCalls)
		}
		if len(store.saves) != 1 {
			t.Fatalf("expected one checkpoint save, got %d", len(store.saves))
		}
		if got := state.GetString("out"); got != "pipeline output" {
			t.Fatalf("unexpected state output %q", got)
		}
		if got := result.Data["links_executed"]; got != 1 {
			t.Fatalf("unexpected links executed data: %+v", result.Data)
		}
		stageResults, ok := result.Data["stage_results"].([]frameworkpipeline.StageResult)
		if !ok || len(stageResults) != 1 {
			t.Fatalf("unexpected stage results: %+v", result.Data["stage_results"])
		}
	})

	t.Run("resume from checkpoint and record event", func(t *testing.T) {
		model := &testLanguageModel{}
		store := checkpoint.NewStore()
		resumeState := core.NewContext()
		resumeState.Set("out", "resumed output")
		cp := &frameworkpipeline.Checkpoint{
			CheckpointID: "cp-1",
			TaskID:       "task-resume",
			StageName:    "stage one",
			StageIndex:   0,
			CreatedAt:    time.Now().UTC(),
			Context:      resumeState,
			Result: frameworkpipeline.StageResult{
				StageName: "stage one",
				Transition: frameworkpipeline.StageTransition{
					Kind: frameworkpipeline.TransitionNext,
				},
			},
		}
		if err := store.Save(cp); err != nil {
			t.Fatalf("Save: %v", err)
		}

		recorder := chaintelemetry.NewEventRecorder()
		agent := &ChainerAgent{
			Model:                model,
			Chain:                &Chain{Links: []Link{NewLink("stage one", "Hello {{.Instruction}}", nil, "out", nil)}},
			CheckpointStore:      store,
			CheckpointAfterStage: true,
			EventRecorder:        recorder,
		}
		if err := agent.Initialize(&core.Config{Model: "test-model"}); err != nil {
			t.Fatalf("Initialize: %v", err)
		}

		state := core.NewContext()
		result, err := agent.Execute(context.Background(), &core.Task{ID: "task-resume", Instruction: "do it"}, state)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !result.Success {
			t.Fatalf("unexpected result: %+v", result)
		}
		if model.generateCalls != 0 {
			t.Fatalf("expected no model calls while resuming, got %d", model.generateCalls)
		}
		if got := state.GetString("out"); got != "resumed output" {
			t.Fatalf("unexpected resumed output %q", got)
		}
		if count, err := store.Count("task-resume"); err != nil || count != 0 {
			t.Fatalf("expected checkpoints to be cleared, count=%d err=%v", count, err)
		}
		events := agent.ExecutionEvents("task-resume")
		if len(events) != 1 || events[0].Kind != chaintelemetry.KindResumeEvent {
			t.Fatalf("unexpected events: %+v", events)
		}
		summary := agent.ExecutionSummary("task-resume")
		if summary == nil || summary.ResumeCount != 1 {
			t.Fatalf("unexpected summary: %+v", summary)
		}
	})
}

func TestChainerAgentResolveChainAndExecuteErrors(t *testing.T) {
	if _, err := (&ChainerAgent{}).resolveChain(&core.Task{}); err == nil {
		t.Fatal("expected resolveChain error")
	}

	builder := func(task *core.Task) (*Chain, error) {
		return &Chain{Links: []Link{NewLink("builder", "Prompt", nil, "out", nil)}}, nil
	}
	agent := &ChainerAgent{ChainBuilder: builder}
	if chain, err := agent.resolveChain(&core.Task{}); err != nil || chain == nil {
		t.Fatalf("resolveChain via builder: chain=%v err=%v", chain, err)
	}

	if _, err := agent.BuildGraph(&core.Task{}); err != nil {
		t.Fatalf("BuildGraph via builder: %v", err)
	}

	if _, err := (&ChainerAgent{}).Execute(context.Background(), &core.Task{Instruction: "noop"}, core.NewContext()); err == nil {
		t.Fatal("expected execute error without chain")
	}
}

func TestLinkStageContractDecodeValidateApply(t *testing.T) {
	link := &Link{
		Name:         "Stage One",
		SystemPrompt: "Hello {{.Instruction}} / {{.Input.value}}",
		InputKeys:    []string{"value"},
		OutputKey:    "out",
		MaxRetries:   2,
		Schema:       `{"type":"object"}`,
		Parse: func(text string) (any, error) {
			if text == "bad" {
				return nil, errors.New("decode failed")
			}
			return map[string]any{"value": text}, nil
		},
	}
	stage := NewLinkStageWithOptions(link, &testLanguageModel{}, &core.LLMOptions{Model: "test-model"})
	contract := stage.Contract()
	if contract.Name != "chainer.Stage One" {
		t.Fatalf("unexpected contract name %q", contract.Name)
	}
	if contract.Metadata.InputKey != "value" || contract.Metadata.OutputKey != "out" || contract.Metadata.SchemaVersion != "1.0" {
		t.Fatalf("unexpected contract metadata: %+v", contract.Metadata)
	}
	if contract.Metadata.RetryPolicy.MaxAttempts != 3 {
		t.Fatalf("unexpected max attempts: %+v", contract.Metadata.RetryPolicy)
	}

	ctx := core.NewContext()
	ctx.Set("value", "v")
	ctx.Set("__chainer_instruction", "instruction")
	prompt, err := stage.BuildPrompt(ctx)
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if prompt != "Hello instruction / v" {
		t.Fatalf("unexpected prompt %q", prompt)
	}

	if _, err := (&LinkStage{}).BuildPrompt(ctx); err == nil {
		t.Fatal("expected nil stage error")
	}
	if _, err := stage.Decode(nil); err == nil {
		t.Fatal("expected nil response error")
	}
	if got, err := stage.Decode(&core.LLMResponse{Text: "ok"}); err != nil || fmt.Sprint(got) != "map[value:ok]" {
		t.Fatalf("unexpected decode result: %v %v", got, err)
	}
	if _, err := stage.Decode(&core.LLMResponse{Text: "bad"}); err == nil {
		t.Fatal("expected decode error")
	}
	if err := stage.Validate(map[string]any{"value": "ok"}); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := stage.Validate("not an object"); err == nil {
		t.Fatal("expected validation error")
	}
	if err := stage.Apply(ctx, "result"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := ctx.GetString("out"); got != "result" {
		t.Fatalf("unexpected applied result %q", got)
	}
	if history := ctx.History(); len(history) == 0 {
		t.Fatal("expected interaction history")
	}

	var nilStage *LinkStage
	if nilStage.Name() != "" {
		t.Fatal("expected empty name for nil stage")
	}
	if contract := nilStage.Contract(); contract.Name != "" {
		t.Fatalf("unexpected nil contract: %+v", contract)
	}
	if _, err := nilStage.BuildPrompt(ctx); err == nil {
		t.Fatal("expected nil stage build prompt error")
	}
	if _, err := nilStage.Decode(&core.LLMResponse{Text: "x"}); err == nil {
		t.Fatal("expected nil stage decode error")
	}
	if err := nilStage.Validate("x"); err == nil {
		t.Fatal("expected nil stage validate error")
	}
	if err := nilStage.Apply(ctx, "x"); err == nil {
		t.Fatal("expected nil stage apply error")
	}
}

func TestChainerHelperEdgeCases(t *testing.T) {
	if _, err := renderPromptForStage("", "instruction", nil); err == nil {
		t.Fatal("expected empty template error")
	}

	recordInteractionForStage(nil, "assistant", "ignored", nil)

	node := &chainerLinkNode{id: "link-1", name: "visible-link"}
	state := core.NewContext()
	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("chainerLinkNode.Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected node result: %+v", result)
	}
	if got := state.GetString("chainer.inspect_link"); got != "visible-link" {
		t.Fatalf("unexpected inspected link %q", got)
	}
}

func agentEnvForTests() agentenv.AgentEnvironment {
	return agentenv.AgentEnvironment{
		Model:    &testLanguageModel{chatResponses: []string{"first", "second"}},
		Registry: capability.NewRegistry(),
		Config:   &core.Config{Model: "test-model"},
	}
}
