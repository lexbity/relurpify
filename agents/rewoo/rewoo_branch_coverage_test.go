package rewoo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/agents/internal/workflowutil"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextmgr"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworkmemory "codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
)

type branchModelStub struct {
	responses []string
	calls     [][]core.Message
	chatErr   error
}

func (m *branchModelStub) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *branchModelStub) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (m *branchModelStub) Chat(ctx context.Context, msgs []core.Message, _ *core.LLMOptions) (*core.LLMResponse, error) {
	m.calls = append(m.calls, append([]core.Message{}, msgs...))
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	if len(m.responses) == 0 {
		return &core.LLMResponse{Text: ""}, nil
	}
	text := m.responses[0]
	m.responses = m.responses[1:]
	return &core.LLMResponse{Text: text}, nil
}

func (m *branchModelStub) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

type branchTool struct {
	name   string
	result *core.ToolResult
	err    error
}

func (t branchTool) Name() string        { return t.name }
func (t branchTool) Description() string { return "branch tool" }
func (t branchTool) Category() string    { return "test" }
func (t branchTool) Parameters() []core.ToolParameter {
	return nil
}
func (t branchTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	if t.err != nil {
		return nil, t.err
	}
	if t.result != nil {
		return t.result, nil
	}
	return &core.ToolResult{Success: true, Data: map[string]any{"ok": true}}, nil
}
func (t branchTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t branchTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t branchTool) Tags() []string                                  { return nil }

type auditStoreStub struct {
	records []memory.DeclarativeMemoryRecord
}

func (s *auditStoreStub) PutDeclarative(_ context.Context, record memory.DeclarativeMemoryRecord) error {
	s.records = append(s.records, record)
	return nil
}

func (s *auditStoreStub) GetDeclarative(context.Context, string) (*memory.DeclarativeMemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *auditStoreStub) SearchDeclarative(context.Context, memory.DeclarativeMemoryQuery) ([]memory.DeclarativeMemoryRecord, error) {
	return nil, nil
}

func (s *auditStoreStub) PutProcedural(context.Context, memory.ProceduralMemoryRecord) error {
	return nil
}

func (s *auditStoreStub) GetProcedural(context.Context, string) (*memory.ProceduralMemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *auditStoreStub) SearchProcedural(context.Context, memory.ProceduralMemoryQuery) ([]memory.ProceduralMemoryRecord, error) {
	return nil, nil
}

func TestRewooGraphMaterializerBranches(t *testing.T) {
	task := &core.Task{Instruction: "plan this"}
	var debugLines []string
	debugf := func(format string, args ...interface{}) {
		debugLines = append(debugLines, fmt.Sprintf(format, args...))
	}
	baseGraph, err := BuildStaticGraph(nil, nil, task, nil, nil, nil, core.NewContext(), RewooOptions{}, nil, nil)
	if err != nil {
		t.Fatalf("BuildStaticGraph: %v", err)
	}
	if err := MaterializePlanGraph(baseGraph, nil, nil, nil, RewooOptions{}, debugf); err != nil {
		t.Fatalf("MaterializePlanGraph(nil): %v", err)
	}

	plan := &RewooPlan{
		Goal: "goal",
		Steps: []RewooStep{
			{ID: "a", Tool: "tool"},
			{ID: "b", Tool: "tool", DependsOn: []string{"a"}},
			{ID: "c", Tool: "tool", DependsOn: []string{"a"}},
			{ID: "d", Tool: "tool", DependsOn: []string{"b", "c"}},
		},
	}
	depMap := map[string]map[string]bool{
		"a": map[string]bool{},
		"b": map[string]bool{"a": true},
		"c": map[string]bool{"a": true},
		"d": map[string]bool{"b": true, "c": true},
	}
	depths := computeDepths(plan.Steps, depMap)
	if depths["a"] != 0 || depths["b"] != 1 || depths["c"] != 1 || depths["d"] != 2 {
		t.Fatalf("unexpected depths: %#v", depths)
	}
	if groups := DetectParallelGroups(plan); len(groups) != 3 {
		t.Fatalf("expected three parallel groups, got %#v", groups)
	}

	planGraph, err := BuildStaticGraph(nil, nil, task, nil, nil, nil, core.NewContext(), RewooOptions{}, nil, nil)
	if err != nil {
		t.Fatalf("BuildStaticGraph(plan): %v", err)
	}
	permMgr, err := authorization.NewPermissionManager("/ws", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/ws/**"}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	if err := MaterializePlanGraph(planGraph, plan, capability.NewRegistry(), permMgr, RewooOptions{}, debugf); err != nil {
		t.Fatalf("MaterializePlanGraph(plan): %v", err)
	}

	cpStore := NewRewooCheckpointStore(nil, nil)
	_, err = buildStaticGraphWithCheckpoints(nil, nil, task, nil, nil, nil, core.NewContext(), RewooOptions{}, nil, cpStore, debugf)
	if err != nil {
		t.Fatalf("buildStaticGraphWithCheckpoints: %v", err)
	}
	if len(debugLines) == 0 {
		t.Fatal("expected debug output")
	}
}

func TestRewooGraphBuilderAndNodeErrorBranches(t *testing.T) {
	cpStore := NewRewooCheckpointStore(nil, nil)
	g, err := buildStaticGraphWithCheckpoints(nil, nil, &core.Task{Instruction: "task"}, nil, nil, nil, core.NewContext(), RewooOptions{GraphConfig: RewooGraphConfig{MaxParallelSteps: 2}}, nil, cpStore, nil)
	if err != nil {
		t.Fatalf("buildStaticGraphWithCheckpoints: %v", err)
	}
	if g == nil {
		t.Fatal("expected graph")
	}
	g2, err := BuildStaticGraph(nil, nil, &core.Task{Instruction: "task"}, nil, nil, nil, core.NewContext(), RewooOptions{}, nil, nil)
	if err != nil {
		t.Fatalf("BuildStaticGraph: %v", err)
	}
	if err := InsertStepNodes(g2, nil, nil, nil, RewooOptions{}, nil); err != nil {
		t.Fatalf("InsertStepNodes(nil): %v", err)
	}
	g3, err := BuildStaticGraph(nil, nil, &core.Task{Instruction: "task"}, nil, nil, nil, core.NewContext(), RewooOptions{}, nil, nil)
	if err != nil {
		t.Fatalf("BuildStaticGraph(second): %v", err)
	}
	if err := InsertStepNodes(g3, &RewooPlan{Goal: "goal", Steps: []RewooStep{{ID: "a", Tool: "tool"}, {ID: "b", Tool: "tool", DependsOn: []string{"a"}}}}, capability.NewRegistry(), nil, RewooOptions{}, nil); err != nil {
		t.Fatalf("InsertStepNodes(plan): %v", err)
	}
	pm, err := authorization.NewPermissionManager("/ws", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/ws/**"}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	if err := InsertStepNodes(g3, &RewooPlan{Goal: "goal", Steps: []RewooStep{{ID: "x", Tool: "tool"}}}, capability.NewRegistry(), pm, RewooOptions{}, func(string, ...interface{}) {}); err != nil {
		t.Fatalf("InsertStepNodes(plan with perm): %v", err)
	}

	planNode := NewPlanNode("plan", nil, nil, nil, nil, nil, nil)
	if _, err := planNode.Execute(context.Background(), core.NewContext()); err == nil || !strings.Contains(err.Error(), "model unavailable") {
		t.Fatalf("expected model unavailable error, got %v", err)
	}
	planNode = NewPlanNode("plan", &branchModelStub{responses: []string{`{"goal":"goal","steps":[]}`}}, nil, nil, nil, nil, core.NewContext())
	if _, err := planNode.Execute(context.Background(), core.NewContext()); err == nil || !strings.Contains(err.Error(), "task unavailable") {
		t.Fatalf("expected task unavailable error, got %v", err)
	}

	replan := NewReplanNode("replan", 1)
	replan.SetThreshold(2)
	if replan.ReplanThreshold != 0.5 {
		t.Fatalf("expected invalid threshold to be ignored, got %f", replan.ReplanThreshold)
	}
	state := core.NewContext()
	state.Set("rewoo.tool_results", "bad type")
	if result, err := replan.Execute(context.Background(), state); err != nil || result.Data["next_node"] != "synthesize" {
		t.Fatalf("unexpected replan type-mismatch result: result=%+v err=%v", result, err)
	}

	synth := NewSynthesisNode("synth", &branchModelStub{responses: []string{"final"}}, &core.Task{Instruction: "task"}, nil, nil, core.NewContext())
	state = core.NewContext()
	state.Set("rewoo.tool_results", "bad type")
	if _, err := synth.Execute(context.Background(), state); err == nil || !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("expected synthesis type mismatch, got %v", err)
	}
}

func TestRewooPreflightRecoveryPlannerAndExecutorBranches(t *testing.T) {
	registry := capability.NewRegistry()
	if err := registry.Register(rewooStubTool{name: "good"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	pm, err := authorization.NewPermissionManager("/ws", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/ws/**"}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}

	if issues := PreflightCheck(context.Background(), registry, nil, pm); len(issues) != 0 {
		t.Fatalf("expected no issues for nil plan, got %#v", issues)
	}
	if issues := PreflightCheck(context.Background(), registry, &RewooPlan{}, pm); len(issues) != 1 || issues[0].Severity != "warning" {
		t.Fatalf("expected warning for empty plan, got %#v", issues)
	}

	plan := &RewooPlan{
		Goal: "goal",
		Steps: []RewooStep{
			{ID: "", Tool: "good"},
			{ID: "missing-tool", Tool: ""},
			{ID: "depends", Tool: "good", DependsOn: []string{"unknown"}, OnFailure: "invalid"},
			{ID: "denied", Tool: "good"},
		},
	}
	issues := PreflightCheck(context.Background(), registry, plan, pm)
	var sawEmptyID, sawEmptyTool, sawUnknownDep, sawInvalidFailure, sawPermissionDenied bool
	for _, issue := range issues {
		switch {
		case strings.Contains(issue.Message, "empty ID"):
			sawEmptyID = true
		case strings.Contains(issue.Message, "empty tool name"):
			sawEmptyTool = true
		case strings.Contains(issue.Message, "unknown step"):
			sawUnknownDep = true
		case strings.Contains(issue.Message, "invalid on_failure mode"):
			sawInvalidFailure = true
		case strings.Contains(issue.Message, "permission denied"):
			sawPermissionDenied = true
		}
	}
	if !sawEmptyID || !sawEmptyTool || !sawUnknownDep || !sawInvalidFailure || !sawPermissionDenied {
		t.Fatalf("missing expected preflight issues: %#v", issues)
	}
	if IsValidPlan(context.Background(), registry, plan, pm) {
		t.Fatal("expected plan to be invalid")
	}

	results := []RewooStepResult{
		{StepID: "a", Success: false, Error: "timeout"},
		{StepID: "b", Success: false, Error: "tool_not_found"},
		{StepID: "c", Success: false, Error: "permission_denied"},
		{StepID: "d", Success: false, Error: "something else"},
		{StepID: "e", Success: true},
	}
	patterns := analyzeErrorPatterns(results)
	if patterns["timeout"] != 1 || patterns["tool_not_found"] != 1 || patterns["permission_denied"] != 1 || patterns["other"] != 1 {
		t.Fatalf("unexpected patterns: %#v", patterns)
	}

	diag := &DiagnosisResult{
		IsRecoverable: true,
		RecommendedID: "scenario-1",
		Scenarios: []RecoveryScenario{{
			ScenarioID:      "scenario-1",
			SuggestedAction: "mystery",
		}},
	}
	if err := RecoverStepFailure(context.Background(), core.NewContext(), diag, nil); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("expected unknown action error, got %v", err)
	}
	if err := RecoverStepFailure(context.Background(), core.NewContext(), &DiagnosisResult{IsRecoverable: false}, nil); err == nil {
		t.Fatal("expected non-recoverable error")
	}

	execRegistry := capability.NewRegistry()
	if err := execRegistry.Register(branchTool{name: "ok"}); err != nil {
		t.Fatalf("register ok tool: %v", err)
	}
	if err := execRegistry.Register(branchTool{name: "fail", result: &core.ToolResult{Success: false, Error: "boom"}}); err != nil {
		t.Fatalf("register fail tool: %v", err)
	}
	executor := &rewooExecutor{
		Registry:          execRegistry,
		PermissionManager: pm,
	}
	_, err = executor.executeStep(context.Background(), core.NewContext(), RewooStep{ID: "a", Tool: "ok"})
	if err == nil {
		t.Fatal("expected permission error with denied capability")
	}
	executor.OnPermissionDenied = StepOnFailureReplan
	_, err = executor.executeStep(context.Background(), core.NewContext(), RewooStep{ID: "a", Tool: "ok"})
	if !errors.Is(err, rewooErrReplanRequired) {
		t.Fatalf("expected replan required, got %v", err)
	}
	executor.OnPermissionDenied = StepOnFailureSkip
	result, err := executor.executeStep(context.Background(), core.NewContext(), RewooStep{ID: "a", Tool: "ok"})
	if err != nil || result.Success {
		t.Fatalf("expected skipped permission failure, got result=%+v err=%v", result, err)
	}

	executor = &rewooExecutor{Registry: execRegistry}
	result, err = executor.executeStep(context.Background(), core.NewContext(), RewooStep{ID: "b", Tool: "fail"})
	if err != nil || result.Success || result.Error == "" {
		t.Fatalf("expected tool result failure to be recorded, got result=%+v err=%v", result, err)
	}
	executor.OnFailure = StepOnFailureAbort
	_, err = executor.executeStep(context.Background(), core.NewContext(), RewooStep{ID: "b", Tool: "fail"})
	if err == nil {
		t.Fatal("expected abort error")
	}
	executor.OnFailure = StepOnFailureReplan
	_, err = executor.executeStep(context.Background(), core.NewContext(), RewooStep{ID: "b", Tool: "fail"})
	if !errors.Is(err, rewooErrReplanRequired) {
		t.Fatalf("expected replan required for tool failure, got %v", err)
	}

	_, err = executor.Execute(context.Background(), &RewooPlan{
		Goal: "deadlock",
		Steps: []RewooStep{
			{ID: "a", Tool: "ok", DependsOn: []string{"b"}},
			{ID: "b", Tool: "ok", DependsOn: []string{"a"}},
		},
	}, core.NewContext())
	if err == nil || !strings.Contains(err.Error(), "deadlock") {
		t.Fatalf("expected deadlock error, got %v", err)
	}
	_, err = (&rewooExecutor{Registry: execRegistry, MaxSteps: 1}).Execute(context.Background(), &RewooPlan{
		Goal: "too many",
		Steps: []RewooStep{
			{ID: "a", Tool: "ok"},
			{ID: "b", Tool: "ok"},
		},
	}, core.NewContext())
	if err == nil || !strings.Contains(err.Error(), "max steps") {
		t.Fatalf("expected max steps error, got %v", err)
	}
}

func TestRewooPlannerAndNodeBranches(t *testing.T) {
	if got := taskInstruction(nil); got != "" {
		t.Fatalf("expected empty task instruction, got %q", got)
	}
	if got := plannerContextBlock(nil, nil, nil); got != "None." {
		t.Fatalf("expected default planner context, got %q", got)
	}
	if got := plannerContextBlock(&core.Task{Context: map[string]any{
		"workflow_retrieval":   "raw retrieval",
		"rewoo_replan_context": "replan please",
	}}, nil, nil); !strings.Contains(got, "raw retrieval") || !strings.Contains(got, "Replan context:") {
		t.Fatalf("expected raw workflow retrieval fallback, got %q", got)
	}

	policy := contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
		Budget:         core.NewContextBudget(8000),
		ContextManager: contextmgr.NewContextManager(core.NewContextBudget(8000)),
	}, nil)
	state := core.NewContext()
	shared := core.NewSharedContext(core.NewContext(), core.NewContextBudget(8000), &core.SimpleSummarizer{})
	if _, err := shared.AddFile("notes.txt", "summary text", "txt", core.DetailSummary); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	model := &branchModelStub{responses: []string{`{"goal":"goal","steps":[{"id":"a","tool":"good","params":{},"depends_on":[],"on_failure":"skip"}]}`}}
	planner := &rewooPlannerNode{
		Model:         model,
		ContextPolicy: policy,
		SharedContext: shared,
		State:         state,
	}
	plan, err := planner.Plan(context.Background(), &core.Task{Instruction: "plan this"}, []core.LLMToolSpec{{Name: "good"}})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan == nil || plan.Goal != "goal" || len(model.calls) != 1 {
		t.Fatalf("unexpected plan result: %+v calls=%d", plan, len(model.calls))
	}
	if len(model.calls[0]) == 0 || model.calls[0][0].Role != "system" {
		t.Fatalf("expected planner system message, got %#v", model.calls)
	}

	model = &branchModelStub{responses: []string{"not json"}}
	planner.Model = model
	if _, err := planner.Plan(context.Background(), &core.Task{Instruction: "plan this"}, nil); err == nil {
		t.Fatal("expected parse error")
	}

	planNode := NewPlanNode("plan", &branchModelStub{responses: []string{`{"goal":"goal","steps":[]}`}}, &core.Task{Instruction: "do it"}, nil, policy, shared, state)
	planNode.Graph = nil
	planNode.Registry = nil
	if result, err := planNode.Execute(context.Background(), core.NewContext()); err != nil || !result.Success {
		t.Fatalf("PlanNode.Execute: result=%+v err=%v", result, err)
	}

	agg := NewAggregateNode("agg", nil)
	if _, err := agg.Execute(context.Background(), core.NewContext()); err == nil {
		t.Fatal("expected aggregate node to fail without a plan")
	}
	aggState := core.NewContext()
	aggState.Set("rewoo.plan", &RewooPlan{Goal: "goal", Steps: []RewooStep{{ID: "a", Tool: "good"}}})
	if result, err := agg.Execute(context.Background(), aggState); err != nil || !result.Success {
		t.Fatalf("AggregateNode.Execute: result=%+v err=%v", result, err)
	}

	cpNoop := NewCheckpointNode("cp", "execute", nil)
	if result, err := cpNoop.Execute(context.Background(), core.NewContext()); err != nil || result.Data["checkpoint_skipped"] != true {
		t.Fatalf("CheckpointNode nil store: result=%+v err=%v", result, err)
	}
	cpStore := NewRewooCheckpointStore(nil, nil)
	cpState := core.NewContext()
	cpState.Set("rewoo.attempt", 2)
	cpState.Set("rewoo.workflow_id", "wf")
	cpState.Set("rewoo.run_id", "run")
	cpState.Set("rewoo.plan", &RewooPlan{Goal: "goal"})
	if result, err := NewCheckpointNode("cp", "execute", cpStore).Execute(context.Background(), cpState); err != nil || !result.Success {
		t.Fatalf("CheckpointNode.Execute: result=%+v err=%v", result, err)
	}

	synth := NewSynthesisNode("synth", &branchModelStub{responses: []string{"final answer"}}, &core.Task{Instruction: "do it"}, policy, shared, state)
	if _, err := synth.Execute(context.Background(), core.NewContext()); err == nil {
		t.Fatal("expected synthesis node to fail without tool results")
	}
	synthState := core.NewContext()
	synthState.Set("rewoo.tool_results", []RewooStepResult{{StepID: "a", Success: true}})
	if result, err := synth.Execute(context.Background(), synthState); err != nil || !result.Success {
		t.Fatalf("SynthesisNode.Execute: result=%+v err=%v", result, err)
	}
}

func TestRewooAgentPersistenceBranches(t *testing.T) {
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("runtime store: %v", err)
	}
	t.Cleanup(func() { _ = runtimeStore.Close() })

	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	if err := workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "wf",
		TaskID:      "task",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "instruction",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := workflowStore.CreateRun(context.Background(), frameworkmemory.WorkflowRunRecord{
		RunID:      "run",
		WorkflowID: "wf",
		Status:     frameworkmemory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	agent := &RewooAgent{}
	surfaces := workflowutil.RuntimeSurfaces{Runtime: runtimeStore, Workflow: workflowStore}
	agent.persistPlan(context.Background(), surfaces, "wf", "run", nil, 0)
	plan := &RewooPlan{Goal: "goal", Steps: []RewooStep{{ID: "a", Description: "step a", Tool: "tool"}}}
	agent.persistPlan(context.Background(), surfaces, "wf", "run", plan, 1)
	agent.persistStepResults(context.Background(), surfaces, "wf", &core.Task{ID: "task"}, plan, []RewooStepResult{{StepID: "a", Tool: "tool", Success: false, Error: "boom"}}, 2)
	if got := agent.persistSynthesis(context.Background(), surfaces, "wf", "run", &core.Task{ID: "task"}, "", nil); got != nil {
		t.Fatalf("expected nil synthesis ref for empty synthesis, got %#v", got)
	}
	ref := agent.persistSynthesis(context.Background(), surfaces, "wf", "run", &core.Task{ID: "task"}, "final answer", []RewooStepResult{{StepID: "a", Tool: "tool", Success: true}})
	if ref == nil {
		t.Fatal("expected synthesis artifact reference")
	}
	if got := agent.persistToolResultsArtifact(context.Background(), surfaces, "wf", "run", nil); got != nil {
		t.Fatalf("expected nil ref for empty results, got %#v", got)
	}

	decl, err := runtimeStore.SearchDeclarative(context.Background(), memory.DeclarativeMemoryQuery{WorkflowID: "wf", Limit: 20})
	if err != nil {
		t.Fatalf("SearchDeclarative: %v", err)
	}
	if len(decl) < 2 {
		t.Fatalf("expected persisted declarative records, got %d", len(decl))
	}
	artifacts, err := workflowStore.ListWorkflowArtifacts(context.Background(), "wf", "run")
	if err != nil {
		t.Fatalf("ListWorkflowArtifacts: %v", err)
	}
	if len(artifacts) == 0 {
		t.Fatal("expected persisted workflow artifacts")
	}
}

func TestRewooAgentExecuteFailureBranch(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow-fail.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })

	registry := capability.NewRegistry()
	_ = registry.Register(rewooStubTool{name: "present"})
	agent := &RewooAgent{
		Model:  &branchModelStub{responses: []string{`{"goal":"goal","steps":[{"id":"a","description":"step a","tool":"missing","params":{},"depends_on":[],"on_failure":"abort"}]}`}},
		Tools:  registry,
		Memory: frameworkmemory.NewCompositeRuntimeStore(workflowStore, nil, nil),
		Options: RewooOptions{
			MaxSteps: 5,
		},
	}
	state := core.NewContext()
	if _, err := agent.Execute(context.Background(), &core.Task{ID: "task", Instruction: "do it"}, state); err == nil {
		t.Fatal("expected execute failure for missing tool")
	}
}

func TestRewooTinyHelpersBranches(t *testing.T) {
	agent := &RewooAgent{Tools: capability.NewRegistry()}
	if got := agent.CapabilityRegistry(); got == nil {
		t.Fatal("expected capability registry")
	}
	if got := (&RewooAgent{}).CapabilityRegistry(); got != nil {
		t.Fatalf("expected nil registry on empty agent, got %#v", got)
	}
	if got := NewRewooTelemetry(nil, nil); got == nil {
		t.Fatal("expected telemetry wrapper")
	}
	if g, err := agent.BuildGraph(nil); err != nil || g == nil {
		t.Fatalf("BuildGraph: graph=%#v err=%v", g, err)
	}
	if _, err := synthesize(context.Background(), nil, &core.Task{Instruction: "task"}, nil, nil, nil, nil); err == nil {
		t.Fatal("expected nil-model synthesis error")
	}
	if got, err := synthesize(context.Background(), &branchModelStub{responses: []string{"answer"}}, &core.Task{Instruction: "task"}, []RewooStepResult{{StepID: "a", Success: true}}, nil, nil, nil); err != nil || got != "answer" {
		t.Fatalf("synthesize success: got=%q err=%v", got, err)
	}

	policy := contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
		Budget:         core.NewContextBudget(8000),
		ContextManager: contextmgr.NewContextManager(core.NewContextBudget(8000)),
	}, nil)
	shared := core.NewSharedContext(core.NewContext(), core.NewContextBudget(8000), &core.SimpleSummarizer{})
	if _, err := shared.AddFile("notes.txt", "shared details", "txt", core.DetailSummary); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if _, err := synthesize(context.Background(), &branchModelStub{responses: []string{"shared answer"}}, &core.Task{Instruction: "task"}, []RewooStepResult{{StepID: "a", Success: true}}, policy, shared, core.NewContext()); err != nil {
		t.Fatalf("synthesize with shared context: %v", err)
	}

	if got := buildReplanContext(nil, []RewooStepResult{{StepID: "a", Success: true}}, errors.New("failed")); !strings.Contains(got, "failed") || !strings.Contains(got, "Goal:") {
		t.Fatalf("unexpected replan context fallback: %q", got)
	}
	if cloned := cloneTaskWithContext(nil); cloned == nil || cloned.Context == nil {
		t.Fatalf("expected cloned empty task, got %#v", cloned)
	}
	if cloned := cloneTaskWithContext(&core.Task{ID: "task"}); cloned == nil || cloned.Context == nil {
		t.Fatalf("expected cloned task context, got %#v", cloned)
	}
	if got := firstNonEmpty(" ", "\t", "alpha", "beta"); got != "alpha" {
		t.Fatalf("unexpected firstNonEmpty result: %q", got)
	}
	if got := taskInstructionID(nil); got != "" {
		t.Fatalf("expected empty taskInstructionID for nil task, got %q", got)
	}
	if got := taskInstructionID(&core.Task{ID: "  "}); got != "" {
		t.Fatalf("expected empty taskInstructionID for blank id, got %q", got)
	}

	planner := &rewooPlannerNode{Model: nil}
	if _, err := planner.Plan(context.Background(), &core.Task{Instruction: "task"}, nil); err == nil || !strings.Contains(err.Error(), "planner model unavailable") {
		t.Fatalf("expected planner model unavailable error, got %v", err)
	}
	planner = &rewooPlannerNode{Model: &branchModelStub{chatErr: errors.New("chat failed")}}
	if _, err := planner.Plan(context.Background(), &core.Task{Instruction: "task"}, nil); err == nil || !strings.Contains(err.Error(), "planning failed") {
		t.Fatalf("expected chat failure, got %v", err)
	}
}

func TestRewooInitializationAndAuditBranches(t *testing.T) {
	agent := &RewooAgent{
		Tools: NewCapabilityRegistry(),
		Options: RewooOptions{
			ContextConfig: RewooContextConfig{
				StrategyName:         "conservative",
				PreferredDetailLevel: "minimal",
				MinHistorySize:       9,
				CompressionThreshold: 0.5,
				BudgetSystemTokens:   100,
				BudgetToolTokens:     200,
				BudgetOutputTokens:   300,
			},
			PermConfig: RewooPermissionConfig{
				DefaultPolicy: "deny",
				EnableHITL:    true,
			},
		},
	}
	if err := agent.initializeContextPolicy(&core.Config{}); err != nil {
		t.Fatalf("initializeContextPolicy: %v", err)
	}
	if agent.ContextPolicy == nil {
		t.Fatal("expected context policy")
	}
	if err := agent.initializePermissionManager(&core.Config{}); err != nil {
		t.Fatalf("initializePermissionManager: %v", err)
	}
	if agent.PermissionManager == nil {
		t.Fatal("expected permission manager")
	}
	if err := agent.PermissionManager.CheckCapability(context.Background(), "rewoo", "missing"); err == nil {
		t.Fatal("expected denied capability")
	}
	if reg := NewCapabilityRegistry(); reg == nil {
		t.Fatal("expected capability registry")
	}

	no := &noopAuditLogger{}
	if got, err := no.Query(context.Background(), core.AuditQuery{}); err != nil || got != nil {
		t.Fatalf("noop audit query: got=%#v err=%v", got, err)
	}

	stub := &auditStoreStub{}
	logger := NewRewooAuditLogger(stub)
	record := core.AuditRecord{
		Timestamp:  time.Unix(100, 0).UTC(),
		AgentID:    "agent-1",
		Action:     "tool",
		Type:       "capability",
		Permission: "rewoo",
		Result:     "granted",
	}
	if err := logger.Log(context.Background(), record); err != nil {
		t.Fatalf("audit Log: %v", err)
	}
	if got, err := logger.Query(context.Background(), core.AuditQuery{}); err != nil || got != nil {
		t.Fatalf("audit Query: got=%#v err=%v", got, err)
	}

	if err := NewRewooAuditLogger(nil).Log(context.Background(), record); err != nil {
		t.Fatalf("nil-store audit Log: %v", err)
	}
}
