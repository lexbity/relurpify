package rewoo_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/agents/rewoo"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkmemory "github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
)

type stubTool struct {
	name    string
	execFn  func(map[string]interface{}) (*core.ToolResult, error)
	callLog *[]string
}

func (t stubTool) Name() string        { return t.name }
func (t stubTool) Description() string { return "stub" }
func (t stubTool) Category() string    { return "testing" }
func (t stubTool) Parameters() []core.ToolParameter {
	return nil
}
func (t stubTool) Execute(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	if t.callLog != nil {
		*t.callLog = append(*t.callLog, t.name)
	}
	if t.execFn != nil {
		return t.execFn(args)
	}
	return &core.ToolResult{Success: true, Data: map[string]any{"ok": true}}, nil
}
func (t stubTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t stubTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{}}
}
func (t stubTool) Tags() []string { return nil }

type stubModel struct {
	responses []string
	calls     int
	messages  [][]core.Message
}

func (m *stubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}
func (m *stubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}
func (m *stubModel) Chat(ctx context.Context, msgs []core.Message, _ *core.LLMOptions) (*core.LLMResponse, error) {
	return m.chat(ctx, msgs)
}
func (m *stubModel) chat(_ context.Context, msgs []core.Message) (*core.LLMResponse, error) {
	m.messages = append(m.messages, append([]core.Message{}, msgs...))
	text := ""
	if m.calls < len(m.responses) {
		text = m.responses[m.calls]
	}
	m.calls++
	return &core.LLMResponse{Text: text}, nil
}
func (m *stubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func TestRewooAgent_ImplementsGraphAgent(t *testing.T) {
	agent := &rewoo.RewooAgent{}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if len(agent.Capabilities()) == 0 {
		t.Fatal("expected capabilities")
	}
	g, err := agent.BuildGraph(nil)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if g == nil {
		t.Fatal("expected graph")
	}
}

func TestRewooExecutor_DependencyOrder(t *testing.T) {
	registry := capability.NewRegistry()
	var order []string
	mustRegister(t, registry, stubTool{name: "first", callLog: &order})
	mustRegister(t, registry, stubTool{name: "second", callLog: &order})

	plan := &rewoo.RewooPlan{
		Goal: "order",
		Steps: []rewoo.RewooStep{
			{ID: "a", Tool: "first"},
			{ID: "b", Tool: "second", DependsOn: []string{"a"}},
		},
	}
	_, err := rewoo.ExecutePlan(context.Background(), registry, plan, core.NewContext(), rewoo.RewooOptions{})
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestRewooExecutor_SkipOnFailure(t *testing.T) {
	registry := capability.NewRegistry()
	mustRegister(t, registry, stubTool{
		name: "fail",
		execFn: func(map[string]interface{}) (*core.ToolResult, error) {
			return nil, errors.New("boom")
		},
	})

	results, err := rewoo.ExecutePlan(context.Background(), registry, &rewoo.RewooPlan{
		Goal:  "skip",
		Steps: []rewoo.RewooStep{{ID: "a", Tool: "fail", OnFailure: rewoo.StepOnFailureSkip}},
	}, core.NewContext(), rewoo.RewooOptions{})
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}
	if len(results) != 1 || results[0].Success {
		t.Fatalf("expected failed recorded result, got %+v", results)
	}
}

func TestRewooExecutor_AbortOnFailure(t *testing.T) {
	registry := capability.NewRegistry()
	mustRegister(t, registry, stubTool{
		name: "fail",
		execFn: func(map[string]interface{}) (*core.ToolResult, error) {
			return nil, errors.New("boom")
		},
	})

	_, err := rewoo.ExecutePlan(context.Background(), registry, &rewoo.RewooPlan{
		Goal:  "abort",
		Steps: []rewoo.RewooStep{{ID: "a", Tool: "fail", OnFailure: rewoo.StepOnFailureAbort}},
	}, core.NewContext(), rewoo.RewooOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRewooPlan_ParseRoundtrip(t *testing.T) {
	raw := `{"goal":"g","steps":[{"id":"a","description":"d","tool":"tool","params":{"x":1},"depends_on":["z"],"on_failure":"skip"}]}`
	plan, err := rewoo.ParsePlan(raw)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if plan.Goal != "g" || len(plan.Steps) != 1 || plan.Steps[0].Tool != "tool" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestRewooAgent_ExecuteWithStubModel(t *testing.T) {
	registry := capability.NewRegistry()
	mustRegister(t, registry, stubTool{name: "tool"})
	model := &stubModel{responses: []string{
		`{"goal":"g","steps":[{"id":"a","description":"d","tool":"tool","params":{},"depends_on":[],"on_failure":"skip"}]}`,
		"final answer",
	}}
	agent := &rewoo.RewooAgent{Model: model, Tools: registry}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{Instruction: "do it"}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := state.GetString("rewoo.synthesis"); got != "final answer" {
		t.Fatalf("unexpected synthesis: %q", got)
	}
}

func TestRewooAgent_HydratesWorkflowRetrievalAndPersistsResults(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErr(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "workflow-rewoo",
		TaskID:      "seed-task",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "seed",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, workflowStore.PutKnowledge(context.Background(), frameworkmemory.KnowledgeRecord{
		RecordID:   "seed",
		WorkflowID: "workflow-rewoo",
		Kind:       frameworkmemory.KnowledgeKindFact,
		Title:      "Prior result",
		Content:    "Known API constraint",
		Status:     "accepted",
	}))

	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("runtime store: %v", err)
	}
	t.Cleanup(func() { _ = runtimeStore.Close() })

	composite := frameworkmemory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)
	registry := capability.NewRegistry()
	mustRegister(t, registry, stubTool{name: "tool"})
	model := &stubModel{responses: []string{
		`{"goal":"g","steps":[{"id":"a","description":"d","tool":"tool","params":{},"depends_on":[],"on_failure":"skip"}]}`,
		"final answer",
	}}
	agent := &rewoo.RewooAgent{Model: model, Tools: registry, Memory: composite}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	task := &core.Task{
		ID:          "rewoo-task",
		Instruction: "do it",
		Context:     map[string]any{"workflow_id": "workflow-rewoo"},
	}
	if _, err := agent.Execute(context.Background(), task, state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if applied, ok := state.Get("rewoo.retrieval_applied"); !ok || applied != true {
		t.Fatalf("expected retrieval flag in state, got %v", applied)
	}
	if len(model.messages) == 0 || !strings.Contains(model.messages[0][0].Content, "Known API constraint") {
		t.Fatalf("expected planner prompt to include workflow retrieval, got %+v", model.messages)
	}

	records, err := workflowStore.ListKnowledge(context.Background(), "workflow-rewoo", "", false)
	if err != nil {
		t.Fatalf("ListKnowledge: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected persisted workflow knowledge, got %d records", len(records))
	}
	var foundSynthesis bool
	for _, record := range records {
		if strings.Contains(record.Title, "ReWOO synthesis") && strings.Contains(record.Content, "Step results:") {
			foundSynthesis = true
			break
		}
	}
	if !foundSynthesis {
		t.Fatal("expected synthesis knowledge record to include step results")
	}

	declarative, err := runtimeStore.SearchDeclarative(context.Background(), frameworkmemory.DeclarativeMemoryQuery{
		WorkflowID: "workflow-rewoo",
		Limit:      16,
	})
	if err != nil {
		t.Fatalf("SearchDeclarative: %v", err)
	}
	if len(declarative) == 0 {
		t.Fatal("expected runtime memory records")
	}
}

func mustRegister(t *testing.T, registry *capability.Registry, tool core.Tool) {
	t.Helper()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
