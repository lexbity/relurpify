package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
)

type htnCapabilityHandler struct {
	desc        core.CapabilityDescriptor
	invocations int
	lastArgs    map[string]any
}

func (h *htnCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return h.desc
}

func (h *htnCapabilityHandler) Invoke(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	h.invocations++
	h.lastArgs = map[string]any{}
	for k, v := range args {
		h.lastArgs[k] = v
	}
	return &core.CapabilityExecutionResult{Success: true, Data: map[string]any{"handled": true}}, nil
}

type htnFallbackExecutor struct {
	executed int
	initCfg  *core.Config
	builds   int
}

func (e *htnFallbackExecutor) Initialize(cfg *core.Config) error {
	e.initCfg = cfg
	return nil
}

func (e *htnFallbackExecutor) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	e.executed++
	return &core.Result{Success: true, Data: map[string]any{"fallback": true}}, nil
}

func (e *htnFallbackExecutor) Capabilities() []core.Capability {
	return []core.Capability{core.CapabilityExecute}
}

func (e *htnFallbackExecutor) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	e.builds++
	g := graph.NewGraph()
	done := graph.NewTerminalNode("done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	if err := g.SetStart(done.ID()); err != nil {
		return nil, err
	}
	return g, nil
}

type branchExecutorProvider struct {
	exec graph.WorkflowExecutor
}

func (p *branchExecutorProvider) Initialize(*core.Config) error { return nil }
func (p *branchExecutorProvider) Execute(context.Context, *core.Task, *core.Context) (*core.Result, error) {
	return &core.Result{Success: true}, nil
}
func (p *branchExecutorProvider) Capabilities() []core.Capability             { return nil }
func (p *branchExecutorProvider) BuildGraph(*core.Task) (*graph.Graph, error) { return nil, nil }
func (p *branchExecutorProvider) BranchExecutor() (graph.WorkflowExecutor, error) {
	return p.exec, nil
}

func TestClassifyTask(t *testing.T) {
	if got := ClassifyTask(nil); got != core.TaskTypeAnalysis {
		t.Fatalf("unexpected nil classification %q", got)
	}
	if got := ClassifyTask(&core.Task{Type: core.TaskTypeReview}); got != core.TaskTypeReview {
		t.Fatalf("expected explicit type to win, got %q", got)
	}
	cases := map[string]core.TaskType{
		"please review this":       core.TaskTypeReview,
		"design the plan":          core.TaskTypePlanning,
		"generate a new file":      core.TaskTypeCodeGeneration,
		"fix and improve behavior": core.TaskTypeCodeModification,
		"just explain the thing":   core.TaskTypeAnalysis,
	}
	for instruction, expected := range cases {
		if got := ClassifyTask(&core.Task{Instruction: instruction}); got != expected {
			t.Fatalf("%q => %q, want %q", instruction, got, expected)
		}
	}
}

func TestMethodValidationAndResolution(t *testing.T) {
	if err := (Method{}).Validate(); err == nil {
		t.Fatal("expected empty method to fail")
	}
	if err := (Method{Name: "m"}).Validate(); err == nil {
		t.Fatal("expected missing task type to fail")
	}
	if err := (Method{Name: "m", TaskType: core.TaskTypeAnalysis}).Validate(); err == nil {
		t.Fatal("expected missing subtasks to fail")
	}
	if err := (Method{
		Name:     "m",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{{Name: "x", Type: core.TaskTypeAnalysis}, {Name: "x", Type: core.TaskTypeAnalysis}},
	}).Validate(); err == nil {
		t.Fatal("expected duplicate subtask to fail")
	}
	if err := (Method{
		Name:     "m",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{{Name: "x", Type: core.TaskTypeAnalysis, DependsOn: []string{"x"}}},
	}).Validate(); err == nil {
		t.Fatal("expected self dependency to fail")
	}
	if err := (Method{
		Name:     "m",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{{Name: "x", Type: core.TaskTypeAnalysis, DependsOn: []string{"missing"}}},
	}).Validate(); err == nil {
		t.Fatal("expected unknown dependency to fail")
	}
	if err := (Method{
		Name:     "m",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{{Name: "x", Type: core.TaskTypeAnalysis, Executor: "bad executor"}},
	}).Validate(); err == nil {
		t.Fatal("expected whitespace executor to fail")
	}

	method := Method{
		Name:     "code-new",
		TaskType: core.TaskTypeCodeGeneration,
		Priority: 5,
		Subtasks: []SubtaskSpec{
			{Name: "explore", Type: core.TaskTypeAnalysis, Instruction: "Explore {{.Instruction}}"},
			{Name: "code", Type: core.TaskTypeCodeGeneration, Instruction: "Code {{.Instruction}}", DependsOn: []string{"explore"}, Executor: ExecutorPipeline},
		},
	}
	if err := method.Validate(); err != nil {
		t.Fatalf("method validate: %v", err)
	}
	resolved := ResolveMethod(method)
	if resolved.Spec.Name != method.Name || resolved.Spec.TaskType != method.TaskType {
		t.Fatalf("unexpected resolved spec: %+v", resolved.Spec)
	}
	if len(resolved.Operators) != 2 {
		t.Fatalf("unexpected operator count %d", len(resolved.Operators))
	}
	if resolved.Operators[0].Executor != ExecutorReact {
		t.Fatalf("expected default executor, got %q", resolved.Operators[0].Executor)
	}
	if resolved.Operators[1].Executor != ExecutorPipeline {
		t.Fatalf("unexpected executor %q", resolved.Operators[1].Executor)
	}
	if len(resolved.Spec.RequiredCapabilities) != 2 {
		t.Fatalf("unexpected required capabilities: %+v", resolved.Spec.RequiredCapabilities)
	}
	if got := dedupeSelectors([]core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: "a"}, {Kind: core.CapabilityKindTool, Name: "a"}}); len(got) != 1 {
		t.Fatalf("expected dedupe, got %+v", got)
	}
}

func TestResolvedValidation(t *testing.T) {
	method := Method{
		Name:     "m",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{{Name: "a", Type: core.TaskTypeAnalysis}},
	}
	resolved := ResolveMethod(method)
	if err := resolved.Validate(); err != nil {
		t.Fatalf("unexpected valid resolved method error: %v", err)
	}

	cases := []ResolvedMethod{
		{},
		{Method: &Method{}},
		{Method: &Method{Name: "m", TaskType: core.TaskTypeAnalysis}},
		{Method: &method, Spec: MethodSpec{Name: "", TaskType: core.TaskTypeAnalysis}},
		{Method: &method, Spec: MethodSpec{Name: "m", TaskType: ""}},
		{Method: &Method{Name: "m", TaskType: core.TaskTypeAnalysis, Subtasks: []SubtaskSpec{{Name: "a", Type: core.TaskTypeAnalysis}}}, Spec: MethodSpec{Name: "other", TaskType: core.TaskTypeAnalysis}},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected invalid resolved method: %+v", tc)
		}
	}

	badCount := resolved
	badCount.Operators = badCount.Operators[:0]
	if err := badCount.Validate(); err == nil {
		t.Fatal("expected operator count mismatch")
	}

	badName := resolved
	badName.Operators[0].Name = ""
	if err := badName.Validate(); err == nil {
		t.Fatal("expected operator name error")
	}
}

func TestMethodLibrary(t *testing.T) {
	lib := NewMethodLibrary()
	if lib == nil {
		t.Fatal("expected method library")
	}
	if got := lib.Find(nil); got != nil {
		t.Fatal("expected nil find")
	}
	if got := lib.FindAll(nil); got != nil {
		t.Fatal("expected nil find all")
	}
	if got := lib.FindByName("code-new"); got == nil {
		t.Fatal("expected default method")
	}
	if got := lib.FindResolved(&core.Task{Type: core.TaskTypeCodeGeneration}); got == nil {
		t.Fatal("expected resolved method")
	}

	lib.Register(Method{Name: "priority-low", TaskType: core.TaskTypeAnalysis, Priority: 1, Subtasks: []SubtaskSpec{{Name: "a", Type: core.TaskTypeAnalysis}}})
	lib.Register(Method{Name: "priority-high", TaskType: core.TaskTypeAnalysis, Priority: 9, Precondition: func(*core.Task) bool { return true }, Subtasks: []SubtaskSpec{{Name: "b", Type: core.TaskTypeAnalysis}}})
	lib.Register(Method{Name: "priority-equal-a", TaskType: core.TaskTypeAnalysis, Priority: 9, Subtasks: []SubtaskSpec{{Name: "c", Type: core.TaskTypeAnalysis}}})
	lib.Register(Method{Name: "priority-equal-b", TaskType: core.TaskTypeAnalysis, Priority: 9, Subtasks: []SubtaskSpec{{Name: "d", Type: core.TaskTypeAnalysis}}})

	all := lib.All()
	if len(all) == 0 {
		t.Fatal("expected methods")
	}
	all[0].Name = "mutated"
	if lib.All()[0].Name == "mutated" {
		t.Fatal("All should return a copy")
	}

	task := &core.Task{Type: core.TaskTypeAnalysis}
	found := lib.Find(task)
	if found == nil || found.Name != "priority-equal-a" {
		t.Fatalf("unexpected priority result: %+v", found)
	}
	allFound := lib.FindAll(task)
	if len(allFound) < 3 {
		t.Fatalf("unexpected find all results: %+v", allFound)
	}
	if allFound[0].Name != "priority-equal-a" {
		t.Fatalf("expected sorted results, got %+v", allFound)
	}

	lib.Register(Method{Name: "priority-high", TaskType: core.TaskTypeAnalysis, Priority: 20, Subtasks: []SubtaskSpec{{Name: "x", Type: core.TaskTypeAnalysis}}})
	if got := lib.FindByName("priority-high"); got == nil || got.Priority != 20 {
		t.Fatalf("expected replacement method, got %+v", got)
	}
}

func TestDecomposeAndHelpers(t *testing.T) {
	task := &core.Task{Instruction: "Build it"}
	if _, err := Decompose(task, nil); err == nil {
		t.Fatal("expected nil method error")
	}
	if _, err := Decompose(task, &Method{Name: "m"}); err == nil {
		t.Fatal("expected empty subtasks error")
	}

	method := &Method{
		Name:     "m",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []SubtaskSpec{
			{Name: "explore", Type: core.TaskTypeAnalysis, Instruction: "Explore {{.Instruction}}"},
			{Name: "code", Type: core.TaskTypeCodeGeneration, Instruction: "Code {{.Instruction}}", DependsOn: []string{"explore"}},
		},
	}
	plan, err := Decompose(task, method)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if plan.Goal != "Build it" || len(plan.Steps) != 2 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.Steps[0].ID != "m.explore" || !strings.Contains(plan.Steps[0].Description, "Build it") {
		t.Fatalf("unexpected first step: %+v", plan.Steps[0])
	}
	if got := plan.Dependencies["m.code"]; len(got) != 1 || got[0] != "m.explore" {
		t.Fatalf("unexpected dependencies: %+v", plan.Dependencies)
	}

	resolved := ResolveMethod(*method)
	decomposed, err := DecomposeResolved(task, &resolved)
	if err != nil {
		t.Fatalf("DecomposeResolved: %v", err)
	}
	if decomposed.Steps[0].Tool != ExecutorReact || decomposed.Steps[1].Tool != ExecutorReact {
		t.Fatalf("unexpected tools: %+v", decomposed.Steps)
	}
	if decomposed.Steps[0].Params["operator_name"] != "explore" {
		t.Fatalf("unexpected params: %+v", decomposed.Steps[0].Params)
	}

	if _, err := DecomposeResolved(task, nil); err == nil {
		t.Fatal("expected nil resolved error")
	}
	if _, err := DecomposeResolved(task, &ResolvedMethod{Method: &Method{Name: "m"}}); err == nil {
		t.Fatal("expected no operators error")
	}

	if got := expandInstruction("x {{.Instruction}} y", "demo"); got != "x demo y" {
		t.Fatalf("unexpected expanded instruction %q", got)
	}
}

func TestDispatchHelpersAndPreflight(t *testing.T) {
	task := &core.Task{
		ID:          "task-1",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "Generate stuff",
		Metadata:    map[string]string{"source": "tests"},
		Context: map[string]any{
			"current_step": core.PlanStep{
				ID:   "step.one",
				Tool: "pipeline",
				Params: map[string]any{
					"operator_executor": "htn",
					"operator_name":     "step-name",
					"required_capabilities": []core.CapabilitySelector{
						{Kind: core.CapabilityKindTool, Name: "agent:htn"},
					},
				},
			},
		},
	}
	target, selectors, args := dispatchMetadata(task)
	if target != "agent:htn" {
		t.Fatalf("unexpected target %q", target)
	}
	if len(selectors) != 1 || selectors[0].Name != "agent:htn" {
		t.Fatalf("unexpected selectors: %+v", selectors)
	}
	if args["task_id"] != "task-1" || args["instruction"] != "Generate stuff" {
		t.Fatalf("unexpected args: %+v", args)
	}
	if got := operatorExecutor(core.PlanStep{Tool: "react", Params: map[string]any{"operator_executor": "pipeline"}}); got != "pipeline" {
		t.Fatalf("unexpected operator executor %q", got)
	}
	if got := operatorName(core.PlanStep{ID: "a.b.c"}); got != "c" {
		t.Fatalf("unexpected operator name %q", got)
	}
	if got := operatorNameFromTask(task); got != "step-name" {
		t.Fatalf("unexpected operator name from task %q", got)
	}
	if got := capabilityTargetForOperator("react"); got != "agent:react" {
		t.Fatalf("unexpected target %q", got)
	}
	if got := capabilityTargetForOperator("custom"); got != "custom" {
		t.Fatalf("unexpected custom target %q", got)
	}
	if got := selectorsFromStep(core.PlanStep{Tool: "react"}); len(got) != 1 || got[0].Name != "agent:react" {
		t.Fatalf("unexpected selectors from step: %+v", got)
	}
	if got := sortedCapabilities([]core.CapabilityDescriptor{{ID: "b"}, {ID: "a"}}); got[0].ID != "a" {
		t.Fatalf("unexpected sort order: %+v", got)
	}
	if got := capabilitySortKey(core.CapabilityDescriptor{Name: "  Z "}); got != "z" {
		t.Fatalf("unexpected sort key %q", got)
	}
	if got := cloneAnyMap(nil); got != nil {
		t.Fatalf("expected nil clone, got %+v", got)
	}
	if got := cloneAnyMap(map[string]any{"a": 1}); got["a"] != 1 {
		t.Fatalf("unexpected cloned map %+v", got)
	}

	reg := capability.NewRegistry()
	handler := &htnCapabilityHandler{
		desc: core.CapabilityDescriptor{
			ID:   "agent:htn",
			Kind: core.CapabilityKindTool,
			Name: "agent:htn",
		},
	}
	if err := reg.RegisterInvocableCapability(handler); err != nil {
		t.Fatalf("register handler: %v", err)
	}

	if target, reason := resolveDispatchTarget(nil, "agent:htn", nil); target != "" || reason != "registry_unavailable" {
		t.Fatalf("unexpected unresolved registry result %q %q", target, reason)
	}
	if target, reason := resolveDispatchTarget(reg, "agent:htn", nil); target != "agent:htn" || reason != "explicit_capability" {
		t.Fatalf("unexpected explicit resolution %q %q", target, reason)
	}
	if target, reason := resolveDispatchTarget(reg, "", []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: "agent:htn"}}); target != "agent:htn" || reason != "selector_capability" {
		t.Fatalf("unexpected selector resolution %q %q", target, reason)
	}
	if target, reason := resolveDispatchTarget(reg, "missing", nil); target != "" || reason != "explicit_target_unresolved" {
		t.Fatalf("unexpected unresolved target %q %q", target, reason)
	}

	plan := &core.Plan{Steps: []core.PlanStep{{ID: "agent:htn.step", Tool: "agent:htn"}}}
	report, err := planPreflight(plan, reg)
	if err != nil {
		t.Fatalf("planPreflight unexpected error: %v", err)
	}
	if report == nil || report.HasBlockingIssues() {
		t.Fatalf("expected clean preflight, got %+v", report)
	}
	missingPlan := &core.Plan{Steps: []core.PlanStep{{ID: "missing", Tool: "missing"}}}
	report, err = planPreflight(missingPlan, reg)
	if err == nil || report == nil || !report.HasBlockingIssues() {
		t.Fatalf("expected blocking preflight issue, report=%+v err=%v", report, err)
	}
	optionalPlan := &core.Plan{Steps: []core.PlanStep{{ID: "opt", Tool: "missing", Params: map[string]any{"optional": true}}}}
	report, err = planPreflight(optionalPlan, reg)
	if err != nil || report == nil {
		t.Fatalf("expected optional preflight success, report=%+v err=%v", report, err)
	}

	if got := isOptionalCapabilityTarget("go_test", core.PlanStep{}); !got {
		t.Fatal("expected built-in optional target")
	}
	if got := capabilityTargetForStep(core.PlanStep{Tool: "react"}); got != "agent:react" {
		t.Fatalf("unexpected capability target %q", got)
	}
}

func TestStatePublicationAndLoad(t *testing.T) {
	task := &core.Task{ID: "task", Type: core.TaskTypeAnalysis, Instruction: "Inspect", Metadata: map[string]string{"k": "v"}}
	method := &Method{
		Name:     "m",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{{Name: "s", Type: core.TaskTypeAnalysis}},
	}
	resolved := ResolveMethod(*method)
	plan := &core.Plan{
		Goal:  "goal",
		Steps: []core.PlanStep{{ID: "m.s", Description: "step"}},
	}

	state := core.NewContext()
	publishTaskState(state, task)
	publishMethodState(state, method)
	publishResolvedMethodState(state, &resolved)
	publishPlanState(state, plan)
	publishWorkflowRetrieval(state, map[string]any{"payload": true}, true)
	publishPreflightState(state, &graph.PreflightReport{GeneratedAt: time.Now().UTC()}, errors.New("bad"))
	publishResumeState(state, "checkpoint-1")
	publishTerminationState(state, "done")
	appendCompletedStep(state, "m.s")
	publishExecutionState(state, ExecutionState{
		WorkflowID:         "wf",
		RunID:              "run",
		CompletedSteps:     []string{"m.s"},
		LastCompletedStep:  "",
		PlannedStepCount:   1,
		CompletedStepCount: 0,
		Resumed:            true,
		ResumeCheckpointID: "checkpoint-1",
	})

	if got := completedStepsFromContext(state); !reflect.DeepEqual(got, []string{"m.s"}) {
		t.Fatalf("unexpected completed steps %+v", got)
	}
	if got := mapsClone(map[string]string{"a": "b"}); got["a"] != "b" {
		t.Fatalf("unexpected map clone %+v", got)
	}
	var decoded core.Task
	if !decodeContextValue(task, &decoded) || decoded.ID != "task" {
		t.Fatalf("unexpected decode value %+v", decoded)
	}
	if !reflect.DeepEqual(MethodStateFromResolved(resolved), MethodState{
		Name:                 resolved.Spec.Name,
		TaskType:             resolved.Spec.TaskType,
		Priority:             resolved.Spec.Priority,
		SubtaskCount:         len(resolved.Method.Subtasks),
		OperatorCount:        len(resolved.Operators),
		RequiredCapabilities: dedupeSelectors(resolved.Spec.RequiredCapabilities),
	}) {
		t.Fatal("method state mismatch")
	}
	if got := MethodStateFromResolved(ResolvedMethod{}); got.SubtaskCount != 0 || got.OperatorCount != 0 {
		t.Fatalf("expected zero-value resolved method to serialize safely, got %+v", got)
	}
	if loaded := LoadExecutionState(state); loaded.ResumeCheckpointID != "checkpoint-1" {
		t.Fatalf("unexpected loaded execution: %+v", loaded)
	}
	if snapshot, loaded, err := LoadStateFromContext(state); err != nil || !loaded || snapshot == nil {
		t.Fatalf("expected snapshot, snapshot=%+v loaded=%v err=%v", snapshot, loaded, err)
	}
	if stateValue, ok := state.Get(contextKeyState); !ok || stateValue == nil {
		t.Fatal("expected published htn state")
	}

	empty := core.NewContext()
	if snapshot, loaded, err := LoadStateFromContext(empty); snapshot != nil || loaded || err != nil {
		t.Fatalf("expected empty load, snapshot=%+v loaded=%v err=%v", snapshot, loaded, err)
	}

	snapshot := &HTNState{
		Task: TaskState{Type: core.TaskTypeAnalysis},
		Method: MethodState{
			Name:     "m",
			TaskType: core.TaskTypeAnalysis,
		},
		Plan: &core.Plan{Steps: []core.PlanStep{{ID: "m.s"}}},
		Execution: ExecutionState{
			CompletedSteps: []string{"m.s"},
		},
	}
	NormalizeHTNState(snapshot)
	if snapshot.SchemaVersion != htnSchemaVersion || snapshot.Metrics.CompletedStepCount != 1 {
		t.Fatalf("unexpected normalized snapshot: %+v", snapshot)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("unexpected valid snapshot error: %v", err)
	}
	snapshot.Execution.CompletedSteps = []string{"dup", "dup"}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("expected duplicate completed step error")
	}
	if err := validatePlanShape(&core.Plan{Steps: []core.PlanStep{{ID: "a"}, {ID: "a"}}}); err == nil {
		t.Fatal("expected duplicate plan step error")
	}
	if err := validatePlanShape(&core.Plan{Steps: []core.PlanStep{{ID: "a"}}, Dependencies: map[string][]string{"b": []string{"a"}}}); err == nil {
		t.Fatal("expected unknown dependency error")
	}
}

func TestStateExportWrappersAndCheckpointLoad(t *testing.T) {
	state := core.NewContext()
	PublishExecutionState(state, ExecutionState{
		WorkflowID:         "wf",
		RunID:              "run",
		CompletedSteps:     []string{"one"},
		ResumeCheckpointID: "checkpoint-2",
	})
	if got := LoadExecutionState(state); got.WorkflowID != "wf" || got.ResumeCheckpointID != "checkpoint-2" {
		t.Fatalf("unexpected execution state: %+v", got)
	}
	if got := DecodeContextValue(map[string]any{"a": 1}, &map[string]any{}); !got {
		t.Fatal("expected decode wrapper to succeed")
	}
	if got := MapsClone(map[string]string{"a": "b"}); got["a"] != "b" {
		t.Fatalf("unexpected maps clone: %+v", got)
	}
	if got := CompletedStepsFromContext(state); !reflect.DeepEqual(got, []string{"one"}) {
		t.Fatalf("unexpected completed steps: %+v", got)
	}

	PublishTaskState(state, &core.Task{ID: "task", Type: core.TaskTypeAnalysis, Instruction: "inspect"})
	PublishResolvedMethodState(state, &ResolvedMethod{Method: &Method{Name: "m", TaskType: core.TaskTypeAnalysis, Subtasks: []SubtaskSpec{{Name: "s", Type: core.TaskTypeAnalysis}}}, Spec: MethodSpec{Name: "m", TaskType: core.TaskTypeAnalysis}, Operators: []OperatorSpec{{Name: "s", TaskType: core.TaskTypeAnalysis, Executor: ExecutorReact}}})
	PublishPreflightState(state, &graph.PreflightReport{GeneratedAt: time.Now().UTC()}, nil)
	PublishResumeState(state, "checkpoint-2")
	PublishWorkflowRetrieval(state, map[string]any{"payload": true}, true)
	PublishPlanState(state, &core.Plan{Steps: []core.PlanStep{{ID: "m.s"}}})
	PublishTerminationState(state, "done")
	PublishExecutionState(state, ExecutionState{CompletedSteps: []string{"m.s"}, PlannedStepCount: 1})
	if _, loaded, err := LoadStateFromContext(state); err != nil || !loaded {
		t.Fatalf("expected state export load success, loaded=%v err=%v", loaded, err)
	}

	checkpointState := &CheckpointState{
		SchemaVersion: htnSchemaVersion,
		CheckpointID:  "cp-1",
		Snapshot: &HTNState{
			Task: TaskState{Type: core.TaskTypeAnalysis},
		},
	}
	state.Set(contextKeyCheckpoint, checkpointState)
	loadedCheckpoint, ok := loadCheckpointState(state)
	if !ok || loadedCheckpoint == nil || loadedCheckpoint.CheckpointID != "cp-1" {
		t.Fatalf("unexpected loaded checkpoint: %+v ok=%v", loadedCheckpoint, ok)
	}

	PublishPlanState(nil, nil)
	PublishResolvedMethodState(nil, nil)
	PublishPreflightState(nil, nil, nil)
	PublishResumeState(nil, "")
	PublishWorkflowRetrieval(nil, nil, false)
	PublishTerminationState(nil, "")
}

func TestMergeHTNBranches(t *testing.T) {
	if err := MergeHTNBranches(nil, nil); err != nil {
		t.Fatalf("nil merge should be no-op, got %v", err)
	}

	parent := core.NewContext()
	parent.Set(contextKeyPlan, &core.Plan{
		Steps: []core.PlanStep{{ID: "one"}, {ID: "two"}},
	})
	parent.Set(contextKeyCompletedSteps, []string{"one"})
	branchState := core.NewContext()
	branchState.Set(contextKeyLastDispatch, map[string]any{"mode": "capability"})
	branchState.SetKnowledge(contextKnowledgeSummary, "summary")
	branchState.Set("htn.current_step", "one")
	branchState.Set(contextKeyCompletedSteps, []string{"one", "two"})
	branch := graph.BranchExecutionResult{
		Step:  core.PlanStep{ID: "two"},
		State: branchState,
		Delta: core.BranchContextDelta{
			StateWrites: map[string]any{
				contextKeyLastDispatch: map[string]any{"mode": "capability"},
				"htn.current_step":     "one",
			},
			SideEffects: core.BranchContextSideEffects{
				KnowledgeWrites: map[string]any{
					contextKnowledgeSummary: "summary",
				},
			},
		},
	}
	if err := MergeHTNBranches(parent, []graph.BranchExecutionResult{branch}); err != nil {
		t.Fatalf("MergeHTNBranches: %v", err)
	}
	if got := completedStepsFromContext(parent); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("unexpected merged steps %+v", got)
	}

	conflict := graph.BranchExecutionResult{
		Step:  core.PlanStep{ID: "three"},
		State: core.NewContext(),
		Delta: core.BranchContextDelta{
			SideEffects: core.BranchContextSideEffects{
				VariableWrites: map[string]any{"x": 1},
			},
		},
	}
	if err := MergeHTNBranches(parent, []graph.BranchExecutionResult{conflict}); err == nil {
		t.Fatal("expected merge conflict")
	}
}

func TestPrimitiveDispatcherFallbackAndBranchExecutor(t *testing.T) {
	var nilDispatcher *primitiveDispatcher
	branch, err := nilDispatcher.BranchExecutor()
	if err != nil || branch == nil {
		t.Fatalf("expected nil branch executor fallback, got %v %v", branch, err)
	}
	if err := nilDispatcher.Initialize(&core.Config{}); err != nil {
		t.Fatalf("nil dispatcher init should no-op, got %v", err)
	}
	if got := nilDispatcher.Capabilities(); got != nil {
		t.Fatalf("expected nil capabilities, got %+v", got)
	}
	if g, err := nilDispatcher.BuildGraph(nil); err != nil || g == nil {
		t.Fatalf("expected trivial graph, got %v %v", g, err)
	}

	fallback := &htnFallbackExecutor{}
	dispatcher := NewPrimitiveDispatcher(nil, fallback)
	if dispatcher == nil {
		t.Fatal("expected dispatcher")
	}
	if err := dispatcher.Initialize(&core.Config{Name: "cfg"}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if fallback.initCfg == nil || fallback.initCfg.Name != "cfg" {
		t.Fatalf("expected fallback initialize, got %+v", fallback.initCfg)
	}
	if got := dispatcher.Capabilities(); len(got) != 1 || got[0] != core.CapabilityExecute {
		t.Fatalf("unexpected capabilities %+v", got)
	}
	if _, err := dispatcher.BuildGraph(nil); err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if result, err := dispatcher.Execute(context.Background(), &core.Task{Instruction: "fallback"}, core.NewContext()); err != nil || result == nil || !result.Success {
		t.Fatalf("unexpected dispatcher execute result=%+v err=%v", result, err)
	}
	if fallback.executed != 1 {
		t.Fatalf("expected fallback execution, got %d", fallback.executed)
	}

	provider := &branchExecutorProvider{exec: fallback}
	branchDispatcher := &primitiveDispatcher{fallback: provider}
	exec, err := branchDispatcher.BranchExecutor()
	if err != nil || exec == nil {
		t.Fatalf("expected branch executor, got %v %v", exec, err)
	}

	if _, _, ok, err := branchDispatcher.invokeCapability(context.Background(), core.NewContext(), "", "", nil, nil); err != nil || ok {
		t.Fatalf("expected unresolved capability path, ok=%v err=%v", ok, err)
	}
}

func TestDispatchTaskAndCapabilityPath(t *testing.T) {
	handler := &htnCapabilityHandler{
		desc: core.CapabilityDescriptor{
			ID:   "agent:htn",
			Kind: core.CapabilityKindTool,
			Name: "agent:htn",
		},
	}
	reg := capability.NewRegistry()
	if err := reg.RegisterInvocableCapability(handler); err != nil {
		t.Fatalf("register capability: %v", err)
	}
	state := core.NewContext()
	task := &core.Task{
		ID:          "task-1",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "do it",
		Context: map[string]any{
			"current_step": core.PlanStep{ID: "step.1", Tool: "htn"},
		},
	}
	result, err := DispatchTask(context.Background(), reg, nil, task, state)
	if err != nil || result == nil || !result.Success {
		t.Fatalf("expected capability dispatch, result=%+v err=%v", result, err)
	}
	if handler.invocations == 0 {
		t.Fatal("expected capability invocation")
	}
	if dispatch, ok := state.Get(contextKeyLastDispatch); !ok || dispatch == nil {
		t.Fatal("expected dispatch metadata")
	}

	fallback := &htnFallbackExecutor{}
	result, err = DispatchTask(context.Background(), nil, fallback, task, core.NewContext())
	if err != nil || result == nil || !result.Success {
		t.Fatalf("expected fallback dispatch, result=%+v err=%v", result, err)
	}
	if fallback.executed != 1 {
		t.Fatalf("expected fallback execution, got %d", fallback.executed)
	}
}
