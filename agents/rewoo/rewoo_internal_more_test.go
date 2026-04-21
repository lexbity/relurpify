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
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextmgr"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	frameworksearch "codeburg.org/lexbit/relurpify/framework/search"
)

type telemetryRecorder struct {
	lines []string
}

type coreTelemetryStub struct{}

func (coreTelemetryStub) Emit(core.Event) {}

type rewooStubTool struct {
	name string
}

func (t rewooStubTool) Name() string        { return t.name }
func (t rewooStubTool) Description() string { return "stub" }
func (t rewooStubTool) Category() string    { return "test" }
func (t rewooStubTool) Parameters() []core.ToolParameter {
	return nil
}
func (t rewooStubTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]any{}}, nil
}
func (t rewooStubTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t rewooStubTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t rewooStubTool) Tags() []string                                  { return nil }

func (r *telemetryRecorder) debugf(format string, args ...interface{}) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

func TestRewooConstructorAndCapabilityRegistry(t *testing.T) {
	var agent *RewooAgent
	if got := agent.CapabilityRegistry(); got != nil {
		t.Fatalf("expected nil registry on nil agent, got %#v", got)
	}

	env := agentenv.AgentEnvironment{
		Config:   &core.Config{Name: "rewoo"},
		Registry: capability.NewRegistry(),
	}
	constructed := New(env, WithMaxSteps(3), WithMaxReplanAttempts(2))
	if constructed == nil {
		t.Fatal("expected constructed agent")
	}
	if constructed.Config != env.Config || constructed.Tools == nil {
		t.Fatalf("unexpected constructed agent: %+v", constructed)
	}
	if constructed.Options.MaxSteps != 3 || constructed.Options.MaxReplanAttempts != 2 {
		t.Fatalf("expected options to be applied, got %+v", constructed.Options)
	}
	other := &RewooAgent{}
	if err := other.InitializeEnvironment(env); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	if other.Config != env.Config || other.Tools == nil {
		t.Fatalf("unexpected initialized agent: %+v", other)
	}
}

func TestRewooPhaseAndOptionsHelpers(t *testing.T) {
	if got := PhasePlan.String(); got != "plan" {
		t.Fatalf("unexpected phase string: %q", got)
	}
	if got := toDetailLevel(PhaseExecute); got != contextmgr.DetailConcise {
		t.Fatalf("unexpected detail level: %v", got)
	}
	if got := toDetailLevel(PhasePlan); got != contextmgr.DetailDetailed {
		t.Fatalf("unexpected detail level: %v", got)
	}

	agent := (&RewooAgent{Options: RewooOptions{MaxSteps: -1, MaxReplanAttempts: -2}})
	opts := agent.options()
	if opts.MaxSteps != 20 || opts.MaxReplanAttempts != 0 || opts.OnFailure != StepOnFailureSkip {
		t.Fatalf("unexpected normalized options: %+v", opts)
	}
}

func TestRewooTelemetry(t *testing.T) {
	recorder := &telemetryRecorder{}
	telemetry := NewRewooTelemetry(nil, recorder.debugf)
	telemetry.EmitPlanStart(context.Background(), "task-1")
	telemetry.EmitPlanComplete(context.Background(), "task-1", 2, 3, 150*time.Millisecond)
	telemetry.EmitStepStart(context.Background(), "task-1", "step-1", "tool")
	telemetry.EmitStepComplete(context.Background(), "task-1", "step-1", "tool", time.Second)
	telemetry.EmitStepFailed(context.Background(), "task-1", "step-1", "tool", "boom", time.Second)
	telemetry.EmitReplan(context.Background(), "task-1", 2, 0.5, "retry")
	telemetry.EmitSynthesisStart(context.Background(), "task-1", 3)
	telemetry.EmitSynthesisComplete(context.Background(), "task-1", 100, 2*time.Second)
	telemetry.EmitExecutionComplete(context.Background(), "task-1", 4, 3, 5*time.Second)
	telemetry.EmitCheckpoint(context.Background(), "task-1", "cp-1", "plan")
	if len(recorder.lines) == 0 {
		t.Fatal("expected telemetry output")
	}
	if !strings.Contains(strings.Join(recorder.lines, "\n"), "execution_complete") {
		t.Fatalf("expected execution_complete telemetry, got %#v", recorder.lines)
	}
}

func TestPreflightAndRecoveryHelpers(t *testing.T) {
	registry := capability.NewRegistry()
	_ = registry.Register(rewooStubTool{name: "tool"})
	plan := &RewooPlan{
		Goal: "goal",
		Steps: []RewooStep{
			{ID: "a", Tool: "tool"},
			{ID: "b", Tool: "missing", DependsOn: []string{"a"}, OnFailure: "invalid"},
		},
	}
	issues := PreflightCheck(context.Background(), registry, plan, nil)
	if len(issues) == 0 {
		t.Fatal("expected preflight issues")
	}
	if IsValidPlan(context.Background(), registry, plan, nil) {
		t.Fatal("expected invalid plan")
	}

	low := DiagnoseStepFailure(context.Background(), core.NewContext(), []RewooStepResult{
		{StepID: "a", Success: false, Error: "timeout"},
		{StepID: "b", Success: true},
		{StepID: "c", Success: true},
		{StepID: "d", Success: true},
	}, plan)
	if low == nil || low.RecommendedID == "" || low.RiskLevel != "low" {
		t.Fatalf("unexpected low diagnosis: %+v", low)
	}
	midState := core.NewContext()
	midState.Set("rewoo.checkpoint_id", "cp-1")
	mid := DiagnoseStepFailure(context.Background(), midState, []RewooStepResult{
		{StepID: "a", Success: false, Error: "timeout"},
		{StepID: "b", Success: true},
	}, plan)
	if mid == nil || mid.RecommendedID == "" || mid.RiskLevel != "low" && mid.RiskLevel != "medium" {
		t.Fatalf("unexpected medium diagnosis: %+v", mid)
	}
	high := DiagnoseStepFailure(context.Background(), core.NewContext(), []RewooStepResult{
		{StepID: "a", Success: false, Output: map[string]any{"x": 1}},
		{StepID: "b", Success: false, Output: map[string]any{"y": 2}},
	}, plan)
	if high == nil || high.RecommendedID == "" || high.RiskLevel != "high" {
		t.Fatalf("unexpected high diagnosis: %+v", high)
	}

	recoverState := core.NewContext()
	if err := RecoverStepFailure(context.Background(), recoverState, low, nil); err != nil {
		t.Fatalf("RecoverStepFailure(retry): %v", err)
	}
	if got := recoverState.GetString("rewoo.recovery_action"); got != "retry" {
		t.Fatalf("unexpected recovery action: %q", got)
	}
	recoverState = core.NewContext()
	if err := RecoverStepFailure(context.Background(), recoverState, high, nil); err != nil {
		t.Fatalf("RecoverStepFailure(high): %v", err)
	}
	if got := recoverState.GetString("rewoo.recovery_action"); got != "synthesize_from_results" {
		t.Fatalf("unexpected recovery action: %q", got)
	}
}

func TestGraphAndContextHelpers(t *testing.T) {
	g, err := BuildStaticGraph(nil, nil, &core.Task{Instruction: "do"}, nil, nil, nil, core.NewContext(), RewooOptions{}, nil, nil)
	if err != nil {
		t.Fatalf("BuildStaticGraph: %v", err)
	}
	if g == nil {
		t.Fatal("expected graph")
	}
	plan := &RewooPlan{
		Goal: "goal",
		Steps: []RewooStep{
			{ID: "a", Tool: "tool"},
			{ID: "b", Tool: "tool", DependsOn: []string{"a"}},
		},
	}
	_ = capability.NewRegistry()
	if err := InsertStepNodes(g, plan, nil, nil, RewooOptions{}, nil); err != nil {
		t.Fatalf("InsertStepNodes: %v", err)
	}

	task := &core.Task{ID: "task-1", Instruction: "build", Context: map[string]any{"x": "y"}}
	cloned := cloneTaskWithContext(task)
	if cloned == task || cloned == nil || cloned.Context == nil {
		t.Fatalf("unexpected cloned task: %+v", cloned)
	}
	if got := firstNonEmpty(" ", "alpha", "beta"); got != "alpha" {
		t.Fatalf("unexpected firstNonEmpty: %q", got)
	}
	if got := taskInstructionID(task); got != "task-1" {
		t.Fatalf("unexpected taskInstructionID: %q", got)
	}
	if got := timePtr(time.Time{}); got == nil {
		t.Fatal("expected time pointer")
	}
	if got := buildReplanContext(plan, []RewooStepResult{{StepID: "a", Tool: "tool", Success: false, Error: "boom"}}, errors.New("failed")); !strings.Contains(got, "Goal: goal") {
		t.Fatalf("unexpected replan context: %q", got)
	}
	if got := summarizeRewooStepResults(nil); got != "" {
		t.Fatalf("expected empty summary, got %q", got)
	}
	if got := summarizeRewooStepResults([]RewooStepResult{{StepID: "a", Success: true}, {StepID: "b", Success: false}}); !strings.Contains(got, "a [ok]") || !strings.Contains(got, "b [failed]") {
		t.Fatalf("unexpected summary: %q", got)
	}
	if got := compactRewooToolResultsState(nil); got["step_count"] != 0 {
		t.Fatalf("unexpected empty compact state: %#v", got)
	}
}

func TestOptionSettersAndPermissionDefaults(t *testing.T) {
	ctxPolicy := &contextmgr.ContextPolicy{}
	pm := &authorization.PermissionManager{}
	indexMgr := &ast.IndexManager{}
	searchEngine := &frameworksearch.SearchEngine{}
	telemetry := coreTelemetryStub{}
	agent := &RewooAgent{}
	for _, opt := range []Option{
		WithContextPolicy(ctxPolicy),
		WithPermissionManager(pm),
		WithIndexManager(indexMgr),
		WithSearchEngine(searchEngine),
		WithTelemetry(telemetry),
		WithContextConfig(RewooContextConfig{StrategyName: "adaptive"}),
		WithPermissionConfig(RewooPermissionConfig{DefaultPolicy: "allow"}),
		WithGraphConfig(RewooGraphConfig{MaxParallelSteps: 2}),
		WithMaxReplanAttempts(3),
		WithMaxSteps(7),
		WithOnFailure(StepOnFailureAbort),
		WithMaxParallelSteps(4),
		WithCheckpointInterval(5),
		WithParallelExecutionEnabled(true),
	} {
		opt(agent)
	}
	if agent.ContextPolicy != ctxPolicy || agent.PermissionManager != pm || agent.IndexManager != indexMgr || agent.SearchEngine != searchEngine || agent.Telemetry != telemetry {
		t.Fatalf("unexpected agent option wiring: %+v", agent)
	}
	if agent.Options.MaxReplanAttempts != 3 || agent.Options.MaxSteps != 7 || agent.Options.OnFailure != StepOnFailureAbort {
		t.Fatalf("unexpected scalar options: %+v", agent.Options)
	}
	if agent.Options.GraphConfig.MaxParallelSteps != 4 || agent.Options.GraphConfig.CheckpointInterval != 5 || !agent.Options.GraphConfig.EnableParallelExecution {
		t.Fatalf("unexpected graph options: %+v", agent.Options.GraphConfig)
	}

	registry := capability.NewRegistry()
	_ = registry.Register(rewooStubTool{name: "tool-a"})
	defaultPerm := DefaultPermissionSet(registry, "/workspace")
	if len(defaultPerm.Capabilities) != 1 || defaultPerm.Capabilities[0].Capability != "tool-a" {
		t.Fatalf("unexpected default permission set: %+v", defaultPerm)
	}
	restricted := RestrictedPermissionSet("/workspace", []string{"tool-a", "tool-b"})
	if len(restricted.Capabilities) != 2 {
		t.Fatalf("unexpected restricted permission set: %+v", restricted)
	}
	readOnly := ReadOnlyPermissionSet("/workspace")
	if len(readOnly.FileSystem) != 1 || readOnly.FileSystem[0].Action != core.FileSystemRead {
		t.Fatalf("unexpected readonly permission set: %+v", readOnly)
	}
}

func TestReplanNodeAndExecutionSpecs(t *testing.T) {
	node := NewReplanNode("replan", 2)
	if node.ID() != "replan" || node.Type() != graph.NodeTypeConditional {
		t.Fatalf("unexpected node identity: %+v", node)
	}
	state := core.NewContext()
	result, err := node.Execute(context.Background(), state)
	if err != nil || result == nil || result.Data["next_node"] != "synthesize" {
		t.Fatalf("unexpected no-results execution: result=%+v err=%v", result, err)
	}
	state.Set("rewoo.tool_results", []RewooStepResult{{StepID: "a", Success: false, Error: "boom"}})
	node.SetAttempt(1)
	node.SetThreshold(0.5)
	result, err = node.Execute(context.Background(), state)
	if err != nil || result == nil || result.Data["next_node"] != "plan" {
		t.Fatalf("unexpected replan execution: result=%+v err=%v", result, err)
	}
	if got := state.GetString("rewoo.replan_context"); !strings.Contains(got, "Goal:") {
		t.Fatalf("expected replan context, got %q", got)
	}

	if got := executionModelToolSpecs(nil, nil); got != nil {
		t.Fatalf("expected nil tool specs for nil inputs, got %#v", got)
	}
	registry := capability.NewRegistry()
	_ = registry.Register(rewooStubTool{name: "tool-b"})
	if got := executionModelToolSpecs(registry, nil); len(got) == 0 {
		t.Fatalf("expected tool specs from registry, got %#v", got)
	}
}

func TestNodeConstructorsAndErrorPaths(t *testing.T) {
	planNode := NewPlanNode("plan", nil, &core.Task{}, nil, nil, nil, core.NewContext())
	if planNode.ID() != "plan" || planNode.Type() != graph.NodeTypeLLM {
		t.Fatalf("unexpected plan node identity: %+v", planNode)
	}
	if _, err := planNode.Execute(context.Background(), core.NewContext()); err == nil {
		t.Fatal("expected plan node to fail without model")
	}

	stepNode := NewStepNode("step", RewooStep{ID: "a", Tool: "tool"}, nil, StepOnFailureSkip)
	if stepNode.ID() != "step" || stepNode.Type() != graph.NodeTypeTool {
		t.Fatalf("unexpected step node identity: %+v", stepNode)
	}
	if _, err := stepNode.Execute(context.Background(), core.NewContext()); err == nil {
		t.Fatal("expected step node to fail without registry")
	}

	synthNode := NewSynthesisNode("synth", nil, &core.Task{}, nil, nil, core.NewContext())
	if synthNode.ID() != "synth" || synthNode.Type() != graph.NodeTypeLLM {
		t.Fatalf("unexpected synthesis node identity: %+v", synthNode)
	}
	if _, err := synthNode.Execute(context.Background(), core.NewContext()); err == nil {
		t.Fatal("expected synthesis node to fail without model and results")
	}

	if got := workflowutil.PayloadKey("test"); got != "test_payload" {
		t.Fatalf("unexpected helper reuse check: %q", got)
	}
}

func TestNoopAgentAndReplanPersistence(t *testing.T) {
	noop := &noopAgent{}
	if err := noop.Initialize(&core.Config{}); err != nil {
		t.Fatalf("noop Initialize: %v", err)
	}
	if len(noop.Capabilities()) != 0 {
		t.Fatalf("expected empty noop capabilities")
	}
	if graph, err := noop.BuildGraph(nil); err != nil || graph == nil {
		t.Fatalf("noop BuildGraph: graph=%#v err=%v", graph, err)
	}
	if result, err := noop.Execute(context.Background(), nil, nil); err != nil || result == nil || !result.Success {
		t.Fatalf("noop Execute: result=%+v err=%v", result, err)
	}

	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "do",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	agent := &RewooAgent{}
	agent.persistReplanSignal(context.Background(), workflowutil.RuntimeSurfaces{Workflow: nil}, "", "", "ctx", 1)
	agent.persistReplanSignal(context.Background(), workflowutil.RuntimeSurfaces{Workflow: store}, "wf-1", "run-1", "ctx", 1)
	events, err := store.ListEvents(context.Background(), "wf-1", 10)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected replan event, got %d", len(events))
	}
	if events[0].EventType != "replan_required" {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}
