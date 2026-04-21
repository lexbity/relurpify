package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/pipeline"
)

type stubPipelineModel struct {
	responses []*core.LLMResponse
	calls     int
}

func (m *stubPipelineModel) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	if m.calls < len(m.responses) {
		resp := m.responses[m.calls]
		m.calls++
		return resp, nil
	}
	m.calls++
	return &core.LLMResponse{Text: "{}"}, nil
}

func (m *stubPipelineModel) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (m *stubPipelineModel) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *stubPipelineModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

type stubPipelineStage struct {
	name        string
	contract    pipeline.ContractDescriptor
	output      any
	decodeErr   error
	validateErr error
	applyErr    error
	applyFn     func(ctx *core.Context, output any)
}

func (s *stubPipelineStage) Name() string                          { return s.name }
func (s *stubPipelineStage) Contract() pipeline.ContractDescriptor { return s.contract }
func (s *stubPipelineStage) BuildPrompt(ctx *core.Context) (string, error) {
	return s.name + " prompt", nil
}
func (s *stubPipelineStage) Decode(resp *core.LLMResponse) (any, error) {
	if s.decodeErr != nil {
		return nil, s.decodeErr
	}
	return s.output, nil
}
func (s *stubPipelineStage) Validate(output any) error { return s.validateErr }
func (s *stubPipelineStage) Apply(ctx *core.Context, output any) error {
	if s.applyFn != nil {
		s.applyFn(ctx, output)
	}
	return s.applyErr
}

func makePipelineStage(name, inputKey, outputKey string, output any) *stubPipelineStage {
	return &stubPipelineStage{
		name: name,
		contract: pipeline.ContractDescriptor{
			Name: name + "-contract",
			Metadata: pipeline.ContractMetadata{
				InputKey:      inputKey,
				OutputKey:     outputKey,
				SchemaVersion: "v1",
			},
		},
		output: output,
	}
}

type pipelineToolStub struct {
	name string
}

func (t pipelineToolStub) Name() string        { return t.name }
func (t pipelineToolStub) Description() string { return "stub tool" }
func (t pipelineToolStub) Category() string    { return "testing" }
func (t pipelineToolStub) Parameters() []core.ToolParameter {
	return nil
}
func (t pipelineToolStub) Execute(context.Context, *core.Context, map[string]any) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t pipelineToolStub) IsAvailable(context.Context, *core.Context) bool { return true }
func (t pipelineToolStub) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{}}
}
func (t pipelineToolStub) Tags() []string { return []string{core.TagExecute} }

type taskTypeStageFactory struct{}

func (f taskTypeStageFactory) StagesForTask(task *core.Task) ([]pipeline.Stage, error) {
	if task != nil && task.Type == core.TaskTypeAnalysis {
		return []pipeline.Stage{makePipelineStage("analyze", "in", "out", map[string]any{"mode": "analysis"})}, nil
	}
	mode := ""
	if task != nil && task.Context != nil {
		mode = task.Context["mode"].(string)
	}
	return []pipeline.Stage{makePipelineStage("code-"+mode, "in", "out", map[string]any{"mode": mode})}, nil
}

func TestPipelineAgentExecuteRunsStages(t *testing.T) {
	model := &stubPipelineModel{responses: []*core.LLMResponse{{Text: "{}"}, {Text: "{}"}}}
	stage1 := makePipelineStage("explore", "in", "stage1.out", map[string]any{"files": 1})
	stage1.applyFn = func(ctx *core.Context, output any) { ctx.Set("stage1.out", output) }
	stage2 := makePipelineStage("analyze", "stage1.out", "stage2.out", map[string]any{"issues": 2})
	stage2.applyFn = func(ctx *core.Context, output any) { ctx.Set("stage2.out", output) }

	agent := &PipelineAgent{
		Model:  model,
		Stages: []pipeline.Stage{stage1, stage2},
	}
	requireNoError(t, agent.Initialize(&core.Config{Model: "test-model"}))

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{ID: "task-1", Instruction: "run pipeline"}, state)
	requireNoError(t, err)
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}
	raw, ok := state.Get("stage2.out")
	if !ok {
		t.Fatalf("expected stage2 output in state")
	}
	output, ok := raw.(map[string]any)
	if !ok || output["issues"] != 2 {
		t.Fatalf("expected stage2 output map, got %#v", raw)
	}
	if _, ok := result.Data["stage_results"]; !ok {
		t.Fatalf("expected stage results in response data")
	}
	if state.GetString("pipeline.run_id") != "" {
		t.Fatalf("did not expect persistence ids without workflow store")
	}
}

func TestPipelineAgentBuildGraphUsesResolvedStages(t *testing.T) {
	agent := &PipelineAgent{
		StageFactory: taskTypeStageFactory{},
	}
	task := &core.Task{Type: core.TaskTypeAnalysis, Context: map[string]any{"mode": "debug"}}
	g, err := agent.BuildGraph(task)
	requireNoError(t, err)
	if err := g.Validate(); err != nil {
		t.Fatalf("expected valid graph, got %v", err)
	}
	res, err := g.Execute(context.Background(), core.NewContext())
	requireNoError(t, err)
	if res == nil || res.NodeID != "pipeline_done" {
		t.Fatalf("expected pipeline_done result, got %+v", res)
	}
}

func TestPipelineAgentSelectsStagesByTaskTypeAndMode(t *testing.T) {
	agent := &PipelineAgent{
		Model:        &stubPipelineModel{responses: []*core.LLMResponse{{Text: "{}"}}},
		StageFactory: taskTypeStageFactory{},
	}
	requireNoError(t, agent.Initialize(&core.Config{}))

	state := core.NewContext()
	_, err := agent.Execute(context.Background(), &core.Task{
		ID:      "task-2",
		Type:    core.TaskTypeAnalysis,
		Context: map[string]any{"mode": "debug"},
	}, state)
	requireNoError(t, err)
	resultsRaw, ok := state.Get("pipeline.results")
	if !ok {
		t.Fatalf("expected pipeline results in state")
	}
	results, ok := resultsRaw.([]pipeline.StageResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one stage result, got %#v", resultsRaw)
	}
	if results[0].StageName != "analyze" {
		t.Fatalf("expected analysis stage, got %s", results[0].StageName)
	}
}

func TestPipelineAgentNewDoesNotInjectCodingStagesByDefault(t *testing.T) {
	agent := New(agentenv.AgentEnvironment{
		Model:  &stubPipelineModel{responses: []*core.LLMResponse{{Text: "{}"}}},
		Config: &core.Config{Model: "test-model"},
	})

	_, err := agent.Execute(context.Background(), &core.Task{ID: "task-generic", Instruction: "run pipeline"}, core.NewContext())
	if err == nil {
		t.Fatal("expected missing stages error")
	}
	if !strings.Contains(err.Error(), "pipeline stages not configured") {
		t.Fatalf("expected missing stages error, got %v", err)
	}
}

func TestPipelineAgentAvailableToolsUsesExecutionCatalogSnapshot(t *testing.T) {
	registry := capability.NewRegistry()
	requireNoError(t, registry.Register(pipelineToolStub{name: "cli_go_test"}))

	agent := &PipelineAgent{Tools: registry}
	agent.executionCatalog = registry.CaptureExecutionCatalogSnapshot()
	defer func() { agent.executionCatalog = nil }()

	registry.UseAgentSpec("agent", &core.AgentRuntimeSpec{
		ExposurePolicies: []core.CapabilityExposurePolicy{{
			Selector: core.CapabilitySelector{Name: "cli_go_test"},
			Access:   core.CapabilityExposureInspectable,
		}},
	})

	tools := agent.availableTools()
	if len(tools) != 1 || tools[0].Name() != "cli_go_test" {
		t.Fatalf("expected execution snapshot tools, got %#v", tools)
	}
}

func TestPipelineAgentPersistsStageResults(t *testing.T) {
	model := &stubPipelineModel{responses: []*core.LLMResponse{{Text: "{}"}}}
	stage := makePipelineStage("explore", "in", "out", map[string]any{"files": 1})
	dbPath := filepath.Join(t.TempDir(), "workflow.db")

	agent := &PipelineAgent{
		Model:             model,
		Stages:            []pipeline.Stage{stage},
		WorkflowStatePath: dbPath,
	}
	requireNoError(t, agent.Initialize(&core.Config{Model: "test-model"}))

	state := core.NewContext()
	_, err := agent.Execute(context.Background(), &core.Task{ID: "task-3", Instruction: "persist pipeline"}, state)
	requireNoError(t, err)

	store, err := db.NewSQLiteWorkflowStateStore(dbPath)
	requireNoError(t, err)
	defer store.Close()

	results, err := store.ListStageResults(context.Background(), "pipeline-task-3", state.GetString("pipeline.run_id"))
	requireNoError(t, err)
	if len(results) != 1 {
		t.Fatalf("expected one persisted result, got %d", len(results))
	}
	if results[0].StageName != "explore" {
		t.Fatalf("expected explore stage, got %s", results[0].StageName)
	}
	rawRef, ok := state.Get("pipeline.final_output_ref")
	if !ok {
		t.Fatal("expected pipeline.final_output_ref in state")
	}
	ref, ok := rawRef.(core.ArtifactReference)
	if !ok {
		t.Fatalf("expected ArtifactReference, got %#v", rawRef)
	}
	if ref.Kind != "pipeline_final_output" {
		t.Fatalf("expected pipeline_final_output ref, got %#v", ref)
	}
	if !strings.Contains(ref.URI, "workflow://artifact/") {
		t.Fatalf("expected workflow artifact uri, got %#v", ref)
	}
	rawFinal, ok := state.Get("pipeline.final_output")
	if !ok {
		t.Fatal("expected compact pipeline.final_output in state")
	}
	finalValue, ok := rawFinal.(map[string]any)
	if !ok {
		t.Fatalf("expected compact pipeline.final_output map, got %#v", rawFinal)
	}
	if _, exists := finalValue["decoded_output"]; exists {
		t.Fatalf("did not expect decoded_output inline once ref exists: %#v", finalValue)
	}
	if !strings.Contains(fmt.Sprint(finalValue["summary"]), "explore [ok]") {
		t.Fatalf("expected compact final output summary, got %#v", finalValue)
	}
	rawResultsRef, ok := state.Get("pipeline.results_ref")
	if !ok {
		t.Fatal("expected pipeline.results_ref in state")
	}
	resultsRef, ok := rawResultsRef.(core.ArtifactReference)
	if !ok {
		t.Fatalf("expected ArtifactReference, got %#v", rawResultsRef)
	}
	if resultsRef.Kind != "pipeline_stage_results" {
		t.Fatalf("expected pipeline_stage_results ref, got %#v", resultsRef)
	}
	if summary := state.GetString("pipeline.results_summary"); !strings.Contains(summary, "explore [ok]") {
		t.Fatalf("unexpected results summary: %q", summary)
	}
	resultsRaw, ok := state.Get("pipeline.results")
	if !ok {
		t.Fatal("expected compact pipeline.results in state")
	}
	resultsValue, ok := resultsRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected compact pipeline.results map, got %#v", resultsRaw)
	}
	if got := fmt.Sprint(resultsValue["stage_count"]); got != "1" {
		t.Fatalf("expected compact stage_count=1, got %#v", resultsValue)
	}
	if _, ok := resultsValue["last_stage"].(map[string]any); !ok {
		t.Fatalf("expected compact last_stage summary, got %#v", resultsValue["last_stage"])
	}
}

func TestPipelineAgentHydratesWorkflowRetrievalIntoState(t *testing.T) {
	model := &stubPipelineModel{responses: []*core.LLMResponse{{Text: "{}"}}}
	stage := makePipelineStage("explore", "in", "out", map[string]any{"files": 1})
	dbPath := filepath.Join(t.TempDir(), "workflow_retrieval.db")

	store, err := db.NewSQLiteWorkflowStateStore(dbPath)
	requireNoError(t, err)
	requireNoError(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "pipeline-task-rt",
		TaskID:      "task-rt",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "workflow history should be searchable through retrieval",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	requireNoError(t, store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "seed-run",
		WorkflowID: "pipeline-task-rt",
		Status:     memory.WorkflowRunStatusRunning,
	}))
	requireNoError(t, store.PutKnowledge(context.Background(), memory.KnowledgeRecord{
		RecordID:   "pipeline-knowledge-1",
		WorkflowID: "pipeline-task-rt",
		Kind:       memory.KnowledgeKindFact,
		Title:      "Prior workflow fact",
		Content:    "Workflow history should be searchable through retrieval.",
		Status:     "accepted",
	}))
	requireNoError(t, store.Close())

	agent := &PipelineAgent{
		Model:             model,
		Stages:            []pipeline.Stage{stage},
		WorkflowStatePath: dbPath,
	}
	requireNoError(t, agent.Initialize(&core.Config{Model: "test-model"}))

	state := core.NewContext()
	_, err = agent.Execute(context.Background(), &core.Task{ID: "task-rt", Instruction: "workflow history should be searchable through retrieval"}, state)
	requireNoError(t, err)

	raw, ok := state.Get("pipeline.workflow_retrieval")
	if !ok {
		t.Fatal("expected pipeline workflow retrieval state")
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected workflow retrieval payload, got %#v", raw)
	}
	rawPayload, ok := state.Get("pipeline.workflow_retrieval_payload")
	if !ok {
		t.Fatal("expected pipeline workflow retrieval payload state")
	}
	if !reflect.DeepEqual(rawPayload, payload) {
		t.Fatalf("expected payload mirror, got %#v want %#v", rawPayload, payload)
	}
	summary, _ := payload["summary"].(string)
	if summary == "" || !strings.Contains(summary, "searchable through retrieval") {
		t.Fatalf("expected workflow retrieval summary, got %#v", payload)
	}
}

func TestPipelineAgentExecutePropagatesStageFailure(t *testing.T) {
	model := &stubPipelineModel{responses: []*core.LLMResponse{{Text: "{}"}}}
	stage := makePipelineStage("analyze", "in", "out", map[string]any{"issues": 0})
	stage.validateErr = errors.New("missing issues")

	agent := &PipelineAgent{
		Model:  model,
		Stages: []pipeline.Stage{stage},
	}
	requireNoError(t, agent.Initialize(&core.Config{Model: "test-model"}))

	_, err := agent.Execute(context.Background(), &core.Task{ID: "task-4", Instruction: "fail pipeline"}, core.NewContext())
	if err == nil {
		t.Fatal("expected validation failure")
	}
}

func TestPipelineAgentPrefersStageBuilderOverFactoryAndStages(t *testing.T) {
	agent := &PipelineAgent{
		Model:        &stubPipelineModel{responses: []*core.LLMResponse{{Text: "{}"}}},
		Stages:       []pipeline.Stage{makePipelineStage("static", "in", "out", map[string]any{"mode": "static"})},
		StageFactory: taskTypeStageFactory{},
		StageBuilder: func(task *core.Task) ([]pipeline.Stage, error) {
			return []pipeline.Stage{makePipelineStage("builder", "in", "out", map[string]any{"mode": "builder"})}, nil
		},
	}
	requireNoError(t, agent.Initialize(&core.Config{Model: "test-model"}))

	state := core.NewContext()
	_, err := agent.Execute(context.Background(), &core.Task{ID: "task-builder", Context: map[string]any{"mode": "debug"}}, state)
	requireNoError(t, err)

	resultsRaw, ok := state.Get("pipeline.results")
	if !ok {
		t.Fatalf("expected pipeline results in state")
	}
	results, ok := resultsRaw.([]pipeline.StageResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one stage result, got %#v", resultsRaw)
	}
	if results[0].StageName != "builder" {
		t.Fatalf("expected builder stage, got %s", results[0].StageName)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
