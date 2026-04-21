package htn_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/agents/htn"
	agentpipeline "codeburg.org/lexbit/relurpify/agents/pipeline"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	frameworkmemory "codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	frameworkpipeline "codeburg.org/lexbit/relurpify/framework/pipeline"
)

// --- classifier tests -------------------------------------------------------

func TestClassifyTask_UsesExistingType(t *testing.T) {
	task := &core.Task{Type: core.TaskTypeReview, Instruction: "add a new function"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeReview {
		t.Errorf("expected %q, got %q", core.TaskTypeReview, got)
	}
}

func TestClassifyTask_KeywordReview(t *testing.T) {
	task := &core.Task{Instruction: "review this pull request"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeReview {
		t.Errorf("expected %q, got %q", core.TaskTypeReview, got)
	}
}

func TestClassifyTask_KeywordGenerate(t *testing.T) {
	task := &core.Task{Instruction: "create a new handler"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeCodeGeneration {
		t.Errorf("expected %q, got %q", core.TaskTypeCodeGeneration, got)
	}
}

func TestClassifyTask_KeywordFix(t *testing.T) {
	task := &core.Task{Instruction: "fix the bug in the parser"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeCodeModification {
		t.Errorf("expected %q, got %q", core.TaskTypeCodeModification, got)
	}
}

func TestClassifyTask_DefaultsToAnalysis(t *testing.T) {
	task := &core.Task{Instruction: "explain the architecture"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeAnalysis {
		t.Errorf("expected %q, got %q", core.TaskTypeAnalysis, got)
	}
}

// --- method library tests ---------------------------------------------------

func TestMethodLibrary_FindByTaskType(t *testing.T) {
	ml := htn.NewMethodLibrary()
	task := &core.Task{Type: core.TaskTypeCodeGeneration, Instruction: "build X"}
	m := ml.Find(task)
	if m == nil {
		t.Fatal("expected method, got nil")
	}
	if m.TaskType != core.TaskTypeCodeGeneration {
		t.Errorf("expected TaskTypeCodeGeneration, got %q", m.TaskType)
	}
	if len(m.Subtasks) == 0 {
		t.Error("method has no subtasks")
	}
}

func TestMethodLibrary_FindReturnsNilForUnknownType(t *testing.T) {
	ml := htn.NewMethodLibrary()
	task := &core.Task{Type: "unknown_type_xyz", Instruction: "do something"}
	m := ml.Find(task)
	if m != nil {
		t.Errorf("expected nil for unknown type, got %+v", m)
	}
}

func TestMethodLibrary_RegisterOverridesExisting(t *testing.T) {
	ml := htn.NewMethodLibrary()
	override := htn.Method{
		Name:     "code-new",
		TaskType: core.TaskTypeCodeGeneration,
		Priority: 100,
		Subtasks: []htn.SubtaskSpec{
			{Name: "custom-step", Type: core.TaskTypeCodeGeneration, Instruction: "custom"},
		},
	}
	ml.Register(override)

	task := &core.Task{Type: core.TaskTypeCodeGeneration, Instruction: "build X"}
	m := ml.Find(task)
	if m == nil {
		t.Fatal("expected method after override")
	}
	if len(m.Subtasks) != 1 || m.Subtasks[0].Name != "custom-step" {
		t.Errorf("override not applied; subtasks: %+v", m.Subtasks)
	}
}

func TestResolveMethod_DefaultRuntimeSpecs(t *testing.T) {
	resolved := htn.ResolveMethod(htn.Method{
		Name:     "code-new",
		TaskType: core.TaskTypeCodeGeneration,
		Priority: 7,
		Subtasks: []htn.SubtaskSpec{
			{Name: "plan", Type: core.TaskTypePlanning},
			{Name: "code", Type: core.TaskTypeCodeGeneration, Executor: "pipeline"},
		},
	})
	if resolved.Spec.Name != "code-new" {
		t.Fatalf("expected method name, got %q", resolved.Spec.Name)
	}
	if resolved.Spec.OperatorCount != 2 {
		t.Fatalf("expected operator count 2, got %d", resolved.Spec.OperatorCount)
	}
	if len(resolved.Spec.RequiredCapabilities) == 0 {
		t.Fatal("expected required capability selectors")
	}
	if got := resolved.Operators[0].Executor; got != "react" {
		t.Fatalf("expected default executor react, got %q", got)
	}
	if got := resolved.Operators[1].Executor; got != "pipeline" {
		t.Fatalf("expected explicit executor pipeline, got %q", got)
	}
	if len(resolved.Operators[1].RequiredCapabilities) == 0 {
		t.Fatal("expected operator required capabilities")
	}
}

func TestMethodLibrary_FindResolvedBreaksPriorityTiesByName(t *testing.T) {
	ml := &htn.MethodLibrary{}
	ml.Register(htn.Method{
		Name:     "z-method",
		TaskType: core.TaskTypeAnalysis,
		Priority: 5,
		Subtasks: []htn.SubtaskSpec{{Name: "one", Type: core.TaskTypeAnalysis}},
	})
	ml.Register(htn.Method{
		Name:     "a-method",
		TaskType: core.TaskTypeAnalysis,
		Priority: 5,
		Subtasks: []htn.SubtaskSpec{{Name: "one", Type: core.TaskTypeAnalysis}},
	})
	resolved := ml.FindResolved(&core.Task{Type: core.TaskTypeAnalysis, Instruction: "explain"})
	if resolved == nil {
		t.Fatal("expected resolved method")
	}
	if resolved.Spec.Name != "a-method" {
		t.Fatalf("expected stable name tie-break, got %q", resolved.Spec.Name)
	}
}

func TestMethodLibrary_PreconditionFilters(t *testing.T) {
	ml := htn.NewMethodLibrary()
	// Register a method with a precondition that always fails.
	ml.Register(htn.Method{
		Name:         "code-new-never",
		TaskType:     core.TaskTypeCodeGeneration,
		Priority:     50,
		Precondition: func(_ *core.Task) bool { return false },
		Subtasks: []htn.SubtaskSpec{
			{Name: "step", Type: core.TaskTypeCodeGeneration},
		},
	})
	task := &core.Task{Type: core.TaskTypeCodeGeneration}
	m := ml.Find(task)
	// Should match the default code-new method, not the never-matching one.
	if m == nil {
		t.Fatal("expected a matching method")
	}
	if m.Name == "code-new-never" {
		t.Error("precondition-failed method should not be selected")
	}
}

// --- decompose tests --------------------------------------------------------

func TestDecompose_ProducesCorrectPlanStructure(t *testing.T) {
	ml := htn.NewMethodLibrary()
	task := &core.Task{Type: core.TaskTypeCodeGeneration, Instruction: "build a server"}
	method := ml.Find(task)
	if method == nil {
		t.Fatal("no method found")
	}

	plan, err := htn.Decompose(task, method)
	if err != nil {
		t.Fatalf("Decompose error: %v", err)
	}
	if plan == nil {
		t.Fatal("nil plan")
	}
	if len(plan.Steps) != len(method.Subtasks) {
		t.Errorf("expected %d steps, got %d", len(method.Subtasks), len(plan.Steps))
	}
	for _, step := range plan.Steps {
		if step.ID == "" {
			t.Error("plan step has empty ID")
		}
	}
}

func TestDecompose_WiresDependencies(t *testing.T) {
	method := &htn.Method{
		Name:     "test-method",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []htn.SubtaskSpec{
			{Name: "a", Type: core.TaskTypeAnalysis},
			{Name: "b", Type: core.TaskTypeCodeGeneration, DependsOn: []string{"a"}},
		},
	}
	task := &core.Task{Type: core.TaskTypeCodeGeneration, Instruction: "test"}
	plan, err := htn.Decompose(task, method)
	if err != nil {
		t.Fatalf("Decompose error: %v", err)
	}
	stepBID := "test-method.b"
	deps, ok := plan.Dependencies[stepBID]
	if !ok {
		t.Fatalf("expected dependency entry for %q", stepBID)
	}
	if len(deps) != 1 || deps[0] != "test-method.a" {
		t.Errorf("unexpected deps: %v", deps)
	}
}

func TestDecomposeResolved_PublishesOperatorMetadata(t *testing.T) {
	resolved := htn.ResolveMethod(htn.Method{
		Name:     "code-new",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []htn.SubtaskSpec{
			{Name: "plan", Type: core.TaskTypePlanning},
			{Name: "code", Type: core.TaskTypeCodeGeneration, Executor: "pipeline", DependsOn: []string{"plan"}},
		},
	})
	plan, err := htn.DecomposeResolved(&core.Task{
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	}, &resolved)
	if err != nil {
		t.Fatalf("DecomposeResolved: %v", err)
	}
	if got := plan.Steps[0].Tool; got != "react" {
		t.Fatalf("expected first step tool react, got %q", got)
	}
	if got := plan.Steps[1].Tool; got != "pipeline" {
		t.Fatalf("expected second step tool pipeline, got %q", got)
	}
	if _, ok := plan.Steps[1].Params["required_capabilities"]; !ok {
		t.Fatal("expected required_capabilities in step params")
	}
}

// --- HTNAgent interface tests -----------------------------------------------

func TestHTNAgent_ImplementsGraphAgent(t *testing.T) {
	a := &htn.HTNAgent{
		Config: &core.Config{MaxIterations: 8},
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	caps := a.Capabilities()
	if len(caps) == 0 {
		t.Error("Capabilities returned empty slice")
	}
	g, err := a.BuildGraph(&core.Task{Type: core.TaskTypeCodeGeneration})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if g == nil {
		t.Error("BuildGraph returned nil graph")
	}
}

func TestHTNAgent_ExecuteWithNoopPrimitive(t *testing.T) {
	a := &htn.HTNAgent{
		Config: &core.Config{MaxIterations: 8},
		// PrimitiveExec left nil — noopAgent fallback used.
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "test-task",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "generate a hello world function",
	}
	result, err := a.Execute(context.Background(), task, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestHTNAgent_UnknownTypeDelegatesToPrimitive(t *testing.T) {
	var delegated bool
	stub := &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
		delegated = true
		return &core.Result{Success: true, Data: map[string]any{}}, nil
	}}

	a := &htn.HTNAgent{
		Config:        &core.Config{MaxIterations: 8},
		PrimitiveExec: stub,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "test-task",
		Type:        "totally_unknown_type",
		Instruction: "do something unusual",
	}
	result, err := a.Execute(context.Background(), task, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if !delegated {
		t.Error("expected delegation to primitive executor for unknown task type")
	}
}

func TestHTNAgent_ExecuteRoutesPrimitiveStepsThroughCapabilities(t *testing.T) {
	registry := capability.NewRegistry()
	var capabilityCalls int
	requireNoErr(t, registry.RegisterInvocableCapability(&stubInvocableCapability{
		desc: capabilityDescriptor("agent:react"),
		invoke: func(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			capabilityCalls++
			return &core.CapabilityExecutionResult{
				Success: true,
				Data:    map[string]interface{}{"text": fmt.Sprint(args["instruction"])},
			}, nil
		},
	}))

	var fallbackCalls int
	agent := &htn.HTNAgent{
		Tools:  registry,
		Config: &core.Config{MaxIterations: 8},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			fallbackCalls++
			return &core.Result{Success: true}, nil
		}},
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-capability-steps",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if capabilityCalls == 0 {
		t.Fatal("expected capability-routed step execution")
	}
	if fallbackCalls != 0 {
		t.Fatalf("expected no fallback primitive calls, got %d", fallbackCalls)
	}
	lastDispatch, ok := state.Get("htn.execution.last_dispatch")
	if !ok {
		t.Fatal("expected last dispatch state")
	}
	dispatch, ok := lastDispatch.(map[string]any)
	if !ok {
		t.Fatalf("expected dispatch map, got %T", lastDispatch)
	}
	if dispatch["mode"] != "capability" {
		t.Fatalf("expected capability dispatch mode, got %v", dispatch["mode"])
	}
	if dispatch["target"] != "agent:react" {
		t.Fatalf("expected agent:react dispatch target, got %v", dispatch["target"])
	}
	if dispatch["requested_target"] != "agent:react" {
		t.Fatalf("expected requested target agent:react, got %v", dispatch["requested_target"])
	}
	if dispatch["resolved_target"] != "agent:react" {
		t.Fatalf("expected resolved target agent:react, got %v", dispatch["resolved_target"])
	}
	if dispatch["reason"] != "explicit_capability" {
		t.Fatalf("expected explicit capability reason, got %v", dispatch["reason"])
	}
	if dispatch["operator"] != "verify" {
		t.Fatalf("expected last operator verify, got %v", dispatch["operator"])
	}
	preflightValue, ok := state.Get("htn.preflight.report")
	if !ok {
		t.Fatal("expected preflight report in state")
	}
	report, ok := preflightValue.(*graph.PreflightReport)
	if !ok {
		t.Fatalf("expected *graph.PreflightReport, got %T", preflightValue)
	}
	if report.HasBlockingIssues() {
		t.Fatalf("expected non-blocking preflight report, got %+v", report.Issues)
	}
	if state.GetString("htn.preflight.error") != "" {
		t.Fatalf("expected empty preflight error, got %q", state.GetString("htn.preflight.error"))
	}
}

func TestHTNAgent_ExecutePropagatesCapabilityResultErrors(t *testing.T) {
	registry := capability.NewRegistry()
	requireNoErr(t, registry.RegisterInvocableCapability(&stubInvocableCapability{
		desc: capabilityDescriptor("agent:react"),
		invoke: func(_ context.Context, _ *core.Context, _ map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			return &core.CapabilityExecutionResult{
				Success: false,
				Error:   "delegated step incomplete",
			}, nil
		},
	}))

	agent := &htn.HTNAgent{
		Tools:  registry,
		Config: &core.Config{MaxIterations: 8},
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-capability-error",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	}, core.NewContext())
	if err == nil {
		t.Fatal("expected delegated capability failure")
	}
	if !strings.Contains(err.Error(), "delegated step incomplete") {
		t.Fatalf("expected propagated capability error, got %v", err)
	}
}

func TestHTNAgent_ExecuteBindsOperatorMetadataOntoStepTask(t *testing.T) {
	var seenTypes []core.TaskType
	var seenOperatorTypes []string
	var seenExecutors []string
	var seenOperatorNames []string
	var seenRequired [][]core.CapabilitySelector
	agent := &htn.HTNAgent{
		Config: &core.Config{MaxIterations: 8},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, task *core.Task, _ *core.Context) (*core.Result, error) {
			if task != nil {
				seenTypes = append(seenTypes, task.Type)
				if task.Context != nil {
					seenOperatorTypes = append(seenOperatorTypes, fmt.Sprint(task.Context["operator_task_type"]))
					seenExecutors = append(seenExecutors, fmt.Sprint(task.Context["operator_executor"]))
					seenOperatorNames = append(seenOperatorNames, fmt.Sprint(task.Context["operator_name"]))
					if raw, ok := task.Context["required_capabilities"]; ok {
						var selectors []core.CapabilitySelector
						if data, err := json.Marshal(raw); err == nil && json.Unmarshal(data, &selectors) == nil {
							seenRequired = append(seenRequired, selectors)
						}
					}
				}
			}
			return &core.Result{Success: true, Data: map[string]any{}}, nil
		}},
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-operator-bindings",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(seenTypes) == 0 {
		t.Fatal("expected primitive steps to execute")
	}
	if seenTypes[0] != core.TaskTypeAnalysis {
		t.Fatalf("expected first step task type %q, got %q", core.TaskTypeAnalysis, seenTypes[0])
	}
	if seenOperatorTypes[0] != string(core.TaskTypeAnalysis) {
		t.Fatalf("expected first operator task type %q, got %q", core.TaskTypeAnalysis, seenOperatorTypes[0])
	}
	if seenExecutors[0] != htn.ExecutorReact {
		t.Fatalf("expected first operator executor %q, got %q", htn.ExecutorReact, seenExecutors[0])
	}
	if seenOperatorNames[0] != "explore" {
		t.Fatalf("expected first operator name %q, got %q", "explore", seenOperatorNames[0])
	}
	if len(seenRequired) == 0 || len(seenRequired[0]) == 0 {
		t.Fatal("expected required capabilities on step task")
	}
	if seenRequired[0][0].Name != "agent:react" {
		t.Fatalf("expected first required capability %q, got %q", "agent:react", seenRequired[0][0].Name)
	}
}

func TestHTNAgent_UnknownTypeDelegatesThroughCapabilityWhenAvailable(t *testing.T) {
	registry := capability.NewRegistry()
	var capabilityCalls int
	requireNoErr(t, registry.RegisterInvocableCapability(&stubInvocableCapability{
		desc: capabilityDescriptor("agent:react"),
		invoke: func(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			capabilityCalls++
			return &core.CapabilityExecutionResult{
				Success: true,
				Data:    map[string]interface{}{"instruction": args["instruction"]},
			}, nil
		},
	}))

	var fallbackCalls int
	agent := &htn.HTNAgent{
		Tools:  registry,
		Config: &core.Config{MaxIterations: 8},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			fallbackCalls++
			return &core.Result{Success: true}, nil
		}},
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-capability-delegate",
		Type:        "unknown_type_xyz",
		Instruction: "do something unusual",
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if capabilityCalls != 1 {
		t.Fatalf("expected one capability delegation, got %d", capabilityCalls)
	}
	if fallbackCalls != 0 {
		t.Fatalf("expected no fallback calls, got %d", fallbackCalls)
	}
}

func TestHTNAgent_ExplicitCustomExecutorDispatchesToCapability(t *testing.T) {
	registry := capability.NewRegistry()
	var customCalls int
	requireNoErr(t, registry.RegisterInvocableCapability(&stubInvocableCapability{
		desc: capabilityDescriptor("agent:reviewer"),
		invoke: func(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			customCalls++
			return &core.CapabilityExecutionResult{
				Success: true,
				Data:    map[string]interface{}{"operator": fmt.Sprint(args["instruction"])},
			}, nil
		},
	}))

	methods := &htn.MethodLibrary{}
	methods.Register(htn.Method{
		Name:     "custom-review",
		TaskType: core.TaskTypeReview,
		Subtasks: []htn.SubtaskSpec{
			{Name: "report", Type: core.TaskTypeReview, Executor: "agent:reviewer", Instruction: "Report on: {{.Instruction}}"},
		},
	})

	var fallbackCalls int
	agent := &htn.HTNAgent{
		Tools:   registry,
		Config:  &core.Config{MaxIterations: 8},
		Methods: methods,
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			fallbackCalls++
			return &core.Result{Success: true}, nil
		}},
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	state := core.NewContext()
	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-custom-dispatch",
		Type:        core.TaskTypeReview,
		Instruction: "review this change",
	}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if customCalls != 1 {
		t.Fatalf("expected one custom capability call, got %d", customCalls)
	}
	if fallbackCalls != 0 {
		t.Fatalf("expected no fallback calls, got %d", fallbackCalls)
	}
	lastDispatch, ok := state.Get("htn.execution.last_dispatch")
	if !ok {
		t.Fatal("expected last dispatch state")
	}
	dispatch, ok := lastDispatch.(map[string]any)
	if !ok {
		t.Fatalf("expected dispatch map, got %T", lastDispatch)
	}
	if dispatch["requested_target"] != "agent:reviewer" {
		t.Fatalf("expected requested target agent:reviewer, got %v", dispatch["requested_target"])
	}
	if dispatch["resolved_target"] != "agent:reviewer" {
		t.Fatalf("expected resolved target agent:reviewer, got %v", dispatch["resolved_target"])
	}
	if dispatch["reason"] != "explicit_capability" {
		t.Fatalf("expected explicit capability reason, got %v", dispatch["reason"])
	}
	if dispatch["operator"] != "report" {
		t.Fatalf("expected operator report, got %v", dispatch["operator"])
	}
}

func TestHTNAgent_FallbackDispatchRecordsReasonWhenCapabilityUnavailable(t *testing.T) {
	var fallbackCalls int
	agent := &htn.HTNAgent{
		Config:  &core.Config{MaxIterations: 8},
		Methods: &htn.MethodLibrary{},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			fallbackCalls++
			return &core.Result{Success: true}, nil
		}},
	}
	agent.Methods.Register(htn.Method{
		Name:     "custom-review",
		TaskType: core.TaskTypeReview,
		Subtasks: []htn.SubtaskSpec{
			{Name: "report", Type: core.TaskTypeReview, Executor: "agent:reviewer", Instruction: "Report on: {{.Instruction}}"},
		},
	})
	requireNoErr(t, agent.Initialize(agent.Config))

	state := core.NewContext()
	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-fallback-dispatch",
		Type:        core.TaskTypeReview,
		Instruction: "review this change",
	}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if fallbackCalls != 1 {
		t.Fatalf("expected one fallback call, got %d", fallbackCalls)
	}
	lastDispatch, ok := state.Get("htn.execution.last_dispatch")
	if !ok {
		t.Fatal("expected last dispatch state")
	}
	dispatch, ok := lastDispatch.(map[string]any)
	if !ok {
		t.Fatalf("expected dispatch map, got %T", lastDispatch)
	}
	if dispatch["mode"] != "fallback" {
		t.Fatalf("expected fallback mode, got %v", dispatch["mode"])
	}
	if dispatch["reason"] != "capability_unresolved" {
		t.Fatalf("expected capability_unresolved reason, got %v", dispatch["reason"])
	}
	if dispatch["requested_target"] != "agent:reviewer" {
		t.Fatalf("expected requested target agent:reviewer, got %v", dispatch["requested_target"])
	}
	if dispatch["operator"] != "report" {
		t.Fatalf("expected operator report, got %v", dispatch["operator"])
	}
}

func TestHTNAgent_RetriesFailedStepWithRecoveryContext(t *testing.T) {
	var attempts int
	var retryAttemptValues []string
	var recoveryDiagnoses []string
	var recoveryNotesSeen bool
	agent := &htn.HTNAgent{
		Config: &core.Config{MaxIterations: 8},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, task *core.Task, _ *core.Context) (*core.Result, error) {
			attempts++
			if task != nil && task.Context != nil {
				retryAttemptValues = append(retryAttemptValues, fmt.Sprint(task.Context["retry_attempt"]))
				recoveryDiagnoses = append(recoveryDiagnoses, fmt.Sprint(task.Context["recovery_diagnosis"]))
				if _, ok := task.Context["recovery_notes"]; ok {
					recoveryNotesSeen = true
				}
			}
			if attempts == 1 {
				return nil, fmt.Errorf("transient failure")
			}
			return &core.Result{Success: true, Data: map[string]any{"text": "recovered"}}, nil
		}},
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-retry-recovery",
		Type:        core.TaskTypePlanning,
		Instruction: "plan this implementation",
	}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success after retry")
	}
	if attempts < 2 {
		t.Fatalf("expected retry attempt, got %d attempts", attempts)
	}
	if len(retryAttemptValues) < 2 || retryAttemptValues[1] != "1" {
		t.Fatalf("expected second attempt retry_attempt=1, got %v", retryAttemptValues)
	}
	if len(recoveryDiagnoses) < 2 || strings.TrimSpace(recoveryDiagnoses[1]) == "" || recoveryDiagnoses[1] == "<nil>" {
		t.Fatalf("expected recovery diagnosis on retry, got %v", recoveryDiagnoses)
	}
	if !recoveryNotesSeen {
		t.Fatal("expected recovery notes in retry context")
	}
	if state.GetString("htn.last_recovery_diagnosis") == "" {
		t.Fatal("expected htn.last_recovery_diagnosis in state")
	}
	if state.GetString("htn.last_failure_error") == "" {
		t.Fatal("expected htn.last_failure_error in state")
	}
	if state.GetString("htn.last_failed_step") == "" {
		t.Fatal("expected htn.last_failed_step in state")
	}
}

func TestHTNAgent_PersistentFailureRecordsRecoveryState(t *testing.T) {
	agent := &htn.HTNAgent{
		Config: &core.Config{MaxIterations: 8},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			return nil, fmt.Errorf("persistent failure")
		}},
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	state := core.NewContext()
	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-persistent-failure",
		Type:        core.TaskTypePlanning,
		Instruction: "plan this implementation",
	}, state)
	if err == nil {
		t.Fatal("expected failure")
	}
	if state.GetString("htn.last_recovery_diagnosis") == "" {
		t.Fatal("expected recovery diagnosis in state")
	}
	if state.GetString("htn.last_failure_error") == "" {
		t.Fatal("expected failure error in state")
	}
	lastNotes, ok := state.Get("htn.last_recovery_notes")
	if !ok {
		t.Fatal("expected recovery notes in state")
	}
	notes, ok := lastNotes.([]string)
	if !ok || len(notes) == 0 {
		t.Fatalf("expected non-empty recovery notes, got %T %v", lastNotes, lastNotes)
	}
}

func TestHTNAgent_ExecutesIndependentStepsInParallel(t *testing.T) {
	methods := &htn.MethodLibrary{}
	methods.Register(htn.Method{
		Name:     "parallel-analysis",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []htn.SubtaskSpec{
			{Name: "inspect-a", Type: core.TaskTypeAnalysis, Instruction: "Inspect A for {{.Instruction}}"},
			{Name: "inspect-b", Type: core.TaskTypeAnalysis, Instruction: "Inspect B for {{.Instruction}}"},
			{Name: "summarize", Type: core.TaskTypeAnalysis, Instruction: "Summarize {{.Instruction}}", DependsOn: []string{"inspect-a", "inspect-b"}},
		},
	})

	var current int32
	var maxSeen int32
	agent := &htn.HTNAgent{
		Config:  &core.Config{MaxIterations: 8},
		Methods: methods,
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			active := atomic.AddInt32(&current, 1)
			for {
				seen := atomic.LoadInt32(&maxSeen)
				if active <= seen || atomic.CompareAndSwapInt32(&maxSeen, seen, active) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&current, -1)
			return &core.Result{Success: true, Data: map[string]any{}}, nil
		}},
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-parallel",
		Type:        core.TaskTypeAnalysis,
		Instruction: "analyze this design",
	}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if atomic.LoadInt32(&maxSeen) < 2 {
		t.Fatalf("expected parallel execution, max concurrency=%d", atomic.LoadInt32(&maxSeen))
	}
	completedValue, ok := state.Get("htn.execution.completed_steps")
	if !ok {
		t.Fatal("expected completed steps in state")
	}
	completedSteps, ok := completedValue.([]string)
	if !ok {
		t.Fatalf("expected []string completed steps, got %T", completedValue)
	}
	if len(completedSteps) != 3 {
		t.Fatalf("expected 3 completed steps, got %d", len(completedSteps))
	}
}

func TestHTNAgent_ParallelMergeRejectsUnsafeBranchStateWrites(t *testing.T) {
	methods := &htn.MethodLibrary{}
	methods.Register(htn.Method{
		Name:     "parallel-unsafe",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []htn.SubtaskSpec{
			{Name: "inspect-a", Type: core.TaskTypeAnalysis, Instruction: "Inspect A for {{.Instruction}}"},
			{Name: "inspect-b", Type: core.TaskTypeAnalysis, Instruction: "Inspect B for {{.Instruction}}"},
		},
	})

	var mu sync.Mutex
	var call int
	agent := &htn.HTNAgent{
		Config:  &core.Config{MaxIterations: 8},
		Methods: methods,
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, state *core.Context) (*core.Result, error) {
			mu.Lock()
			call++
			value := call
			mu.Unlock()
			if state != nil {
				state.Set("unsafe.branch.key", value)
			}
			time.Sleep(10 * time.Millisecond)
			return &core.Result{Success: true, Data: map[string]any{}}, nil
		}},
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-parallel-unsafe",
		Type:        core.TaskTypeAnalysis,
		Instruction: "analyze this design",
	}, core.NewContext())
	if err == nil {
		t.Fatal("expected unsafe branch merge failure")
	}
	if !strings.Contains(err.Error(), "outside merge policy") {
		t.Fatalf("expected merge policy error, got %v", err)
	}
}

func TestHTNAgent_PrefightFailsWhenRequiredCapabilityMissing(t *testing.T) {
	registry := capability.NewRegistry()
	requireNoErr(t, registry.RegisterInvocableCapability(&stubInvocableCapability{
		desc: capabilityDescriptor("agent:react"),
		invoke: func(_ context.Context, _ *core.Context, _ map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			return &core.CapabilityExecutionResult{Success: true}, nil
		},
	}))

	methods := &htn.MethodLibrary{}
	methods.Register(htn.Method{
		Name:     "custom-review",
		TaskType: core.TaskTypeReview,
		Subtasks: []htn.SubtaskSpec{
			{Name: "report", Type: core.TaskTypeReview, Executor: "agent:reviewer", Instruction: "Report on: {{.Instruction}}"},
		},
	})

	agent := &htn.HTNAgent{
		Tools:   registry,
		Config:  &core.Config{MaxIterations: 8},
		Methods: methods,
	}
	requireNoErr(t, agent.Initialize(agent.Config))

	state := core.NewContext()
	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-preflight-missing-capability",
		Type:        core.TaskTypeReview,
		Instruction: "review this change",
	}, state)
	if err == nil {
		t.Fatal("expected preflight failure")
	}
	if !strings.Contains(err.Error(), "preflight failed") {
		t.Fatalf("expected preflight failure error, got %v", err)
	}
	if state.GetString("htn.preflight.error") == "" {
		t.Fatal("expected preflight error state")
	}
	preflightValue, ok := state.Get("htn.preflight.report")
	if !ok {
		t.Fatal("expected preflight report in state")
	}
	report, ok := preflightValue.(*graph.PreflightReport)
	if !ok {
		t.Fatalf("expected *graph.PreflightReport, got %T", preflightValue)
	}
	if !report.HasBlockingIssues() {
		t.Fatalf("expected blocking preflight issues, got %+v", report.Issues)
	}
}

func TestHTNAgent_PersistsPrimitiveStepResultsToWorkflowMemory(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	composite := frameworkmemory.NewCompositeRuntimeStore(workflowStore, nil, nil)
	agent := &htn.HTNAgent{
		Memory: composite,
		Config: &core.Config{MaxIterations: 4},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			return &core.Result{Success: true, Data: map[string]any{"text": "implemented subtask"}}, nil
		}},
	}
	if err := agent.Initialize(agent.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "htn-persist",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement the feature",
		Context:     map[string]any{"workflow_id": "workflow-htn"},
	}
	if _, err := agent.Execute(context.Background(), task, core.NewContext()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	records, err := workflowStore.ListKnowledge(context.Background(), "workflow-htn", "", false)
	if err != nil {
		t.Fatalf("ListKnowledge: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected workflow knowledge records")
	}

	declarative, err := runtimeStore.SearchDeclarative(context.Background(), frameworkmemory.DeclarativeMemoryQuery{
		WorkflowID: "workflow-htn",
		Limit:      16,
	})
	if err != nil {
		t.Fatalf("SearchDeclarative: %v", err)
	}
	if len(declarative) == 0 {
		t.Fatal("expected runtime declarative records")
	}
}

func TestHTNAgent_HydratesWorkflowRetrievalAndSetsStateFlag(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErr(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "workflow-htn",
		TaskID:      "seed-task",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "seed",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, workflowStore.PutKnowledge(context.Background(), frameworkmemory.KnowledgeRecord{
		RecordID:   "seed",
		WorkflowID: "workflow-htn",
		Kind:       frameworkmemory.KnowledgeKindFact,
		Title:      "Prior result",
		Content:    "Known API constraint",
		Status:     "accepted",
	}))

	var seenRetrieval string
	var seenMode string
	var sawPayload bool
	composite := frameworkmemory.NewCompositeRuntimeStore(workflowStore, nil, nil)
	agent := &htn.HTNAgent{
		Memory: composite,
		Config: &core.Config{MaxIterations: 4},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, task *core.Task, _ *core.Context) (*core.Result, error) {
			if task != nil && task.Context != nil {
				seenRetrieval = fmt.Sprint(task.Context["workflow_retrieval"])
				seenMode = fmt.Sprint(task.Context["mode"])
				_, sawPayload = task.Context["workflow_retrieval_payload"]
			}
			return &core.Result{Success: true, Data: map[string]any{"text": "implemented subtask"}}, nil
		}},
	}
	if err := agent.Initialize(agent.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	task := &core.Task{
		ID:          "htn-retrieval",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement the feature",
		Context: map[string]any{
			"workflow_id": "workflow-htn",
			"mode":        "debug",
		},
	}
	if _, err := agent.Execute(context.Background(), task, state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if applied, ok := state.Get("htn.retrieval_applied"); !ok || applied != true {
		t.Fatalf("expected retrieval flag in state, got %v", applied)
	}
	if rawPayload, ok := state.Get("htn.workflow_retrieval_payload"); !ok {
		t.Fatal("expected htn.workflow_retrieval_payload in state")
	} else if payload, ok := rawPayload.(map[string]any); !ok || strings.TrimSpace(fmt.Sprint(payload["summary"])) == "" {
		t.Fatalf("expected structured workflow retrieval payload, got %#v", rawPayload)
	}
	taskStateValue, ok := state.Get("htn.task")
	if !ok {
		t.Fatal("expected htn.task in state")
	}
	taskState, ok := taskStateValue.(htn.TaskState)
	if !ok {
		t.Fatalf("expected htn.TaskState, got %T", taskStateValue)
	}
	if taskState.Type != core.TaskTypeCodeGeneration {
		t.Fatalf("expected task type %q, got %q", core.TaskTypeCodeGeneration, taskState.Type)
	}
	methodStateValue, ok := state.Get("htn.selected_method")
	if !ok {
		t.Fatal("expected htn.selected_method in state")
	}
	methodState, ok := methodStateValue.(htn.MethodState)
	if !ok {
		t.Fatalf("expected htn.MethodState, got %T", methodStateValue)
	}
	if methodState.Name == "" {
		t.Fatal("expected selected method name")
	}
	if methodState.OperatorCount == 0 {
		t.Fatal("expected selected method operator count")
	}
	if len(methodState.RequiredCapabilities) == 0 {
		t.Fatal("expected selected method required capabilities")
	}
	planValue, ok := state.Get("htn.plan")
	if !ok {
		t.Fatal("expected htn.plan in state")
	}
	planState, ok := planValue.(*core.Plan)
	if !ok {
		t.Fatalf("expected *core.Plan, got %T", planValue)
	}
	if len(planState.Steps) == 0 {
		t.Fatal("expected planned steps in htn.plan")
	}
	executionValue, ok := state.Get("htn.execution")
	if !ok {
		t.Fatal("expected htn.execution in state")
	}
	executionState, ok := executionValue.(htn.ExecutionState)
	if !ok {
		t.Fatalf("expected htn.ExecutionState, got %T", executionValue)
	}
	if executionState.PlannedStepCount != len(planState.Steps) {
		t.Fatalf("expected planned step count %d, got %d", len(planState.Steps), executionState.PlannedStepCount)
	}
	if executionState.CompletedStepCount != len(planState.Steps) {
		t.Fatalf("expected completed step count %d, got %d", len(planState.Steps), executionState.CompletedStepCount)
	}
	if got := state.GetString("htn.termination"); got != "completed" {
		t.Fatalf("expected completed termination, got %q", got)
	}
	rawRunSummaryRef, ok := state.Get("htn.run_summary_ref")
	if !ok {
		t.Fatal("expected htn.run_summary_ref in state")
	}
	runSummaryRef, ok := rawRunSummaryRef.(core.ArtifactReference)
	if !ok {
		t.Fatalf("expected core.ArtifactReference for htn.run_summary_ref, got %T", rawRunSummaryRef)
	}
	if runSummaryRef.Kind != "htn_run_summary" {
		t.Fatalf("expected htn_run_summary ref, got %q", runSummaryRef.Kind)
	}
	if strings.TrimSpace(state.GetString("htn.run_summary_summary")) == "" {
		t.Fatal("expected htn.run_summary_summary in state")
	}
	rawMetricsRef, ok := state.Get("htn.execution_metrics_ref")
	if !ok {
		t.Fatal("expected htn.execution_metrics_ref in state")
	}
	metricsRef, ok := rawMetricsRef.(core.ArtifactReference)
	if !ok {
		t.Fatalf("expected core.ArtifactReference for htn.execution_metrics_ref, got %T", rawMetricsRef)
	}
	if metricsRef.Kind != "htn_execution_metrics" {
		t.Fatalf("expected htn_execution_metrics ref, got %q", metricsRef.Kind)
	}
	if strings.TrimSpace(state.GetString("htn.execution_metrics_summary")) == "" {
		t.Fatal("expected htn.execution_metrics_summary in state")
	}
	if seenRetrieval == "" || seenRetrieval != "Prior result: Known API constraint" {
		t.Fatalf("expected primitive step to receive retrieval text, got %q", seenRetrieval)
	}
	if seenMode != "debug" {
		t.Fatalf("expected primitive step to preserve mode, got %q", seenMode)
	}
	if !sawPayload {
		t.Fatal("expected primitive step to preserve workflow_retrieval_payload")
	}
}

func TestHTNAgent_ResumesFromSQLiteCheckpoint(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErr(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "workflow-htn",
		TaskID:      "htn-resume",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "resume work",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))

	methods := htn.NewMethodLibrary()
	task := &core.Task{
		ID:          "htn-resume",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement the feature",
		Context:     map[string]any{"workflow_id": "workflow-htn"},
	}
	method := methods.Find(task)
	if method == nil {
		t.Fatal("expected default method")
	}
	plan, err := htn.Decompose(task, method)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(plan.Steps) < 2 {
		t.Fatalf("expected multi-step plan, got %d steps", len(plan.Steps))
	}
	checkpointState := core.NewContext()
	checkpointState.Set("plan.completed_steps", []string{plan.Steps[0].ID})
	checkpointAdapter := agentpipeline.NewSQLitePipelineCheckpointStore(workflowStore, "workflow-htn", "seed-run")
	requireNoErr(t, checkpointAdapter.Save(&frameworkpipeline.Checkpoint{
		CheckpointID: "cp-seeded",
		TaskID:       task.ID,
		StageName:    plan.Steps[0].ID,
		StageIndex:   0,
		CreatedAt:    time.Now().UTC(),
		Context:      checkpointState,
		Result: frameworkpipeline.StageResult{
			StageName:    plan.Steps[0].ID,
			ValidationOK: true,
			Transition: frameworkpipeline.StageTransition{
				Kind: frameworkpipeline.TransitionNext,
			},
		},
	}))

	var calls int
	agent := &htn.HTNAgent{
		Memory:  frameworkmemory.NewCompositeRuntimeStore(workflowStore, nil, nil),
		Config:  &core.Config{MaxIterations: 4},
		Methods: methods,
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			calls++
			return &core.Result{Success: true, Data: map[string]any{"text": "implemented subtask"}}, nil
		}},
	}
	if err := agent.Initialize(agent.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	if _, err := agent.Execute(context.Background(), task, state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if calls != len(plan.Steps)-1 {
		t.Fatalf("expected resumed run to execute %d steps, got %d", len(plan.Steps)-1, calls)
	}
	if got := state.GetString("htn.resume_checkpoint_id"); got != "cp-seeded" {
		t.Fatalf("expected resume checkpoint id, got %q", got)
	}
	completedValue, ok := state.Get("htn.execution.completed_steps")
	if !ok {
		t.Fatal("expected htn.execution.completed_steps in state")
	}
	completedSteps, ok := completedValue.([]string)
	if !ok {
		t.Fatalf("expected []string completed steps, got %T", completedValue)
	}
	if len(completedSteps) != len(plan.Steps) {
		t.Fatalf("expected %d completed steps, got %d", len(plan.Steps), len(completedSteps))
	}
	executionValue, ok := state.Get("htn.execution")
	if !ok {
		t.Fatal("expected htn.execution in state")
	}
	executionState, ok := executionValue.(htn.ExecutionState)
	if !ok {
		t.Fatalf("expected htn.ExecutionState, got %T", executionValue)
	}
	if !executionState.Resumed {
		t.Fatal("expected execution state to mark resumed")
	}
	if executionState.ResumeCheckpointID != "cp-seeded" {
		t.Fatalf("expected resume checkpoint id %q, got %q", "cp-seeded", executionState.ResumeCheckpointID)
	}
}

func TestLoadStateFromContext_RejectsCompletedStepOutsidePlan(t *testing.T) {
	state := core.NewContext()
	state.Set("htn.task", htn.TaskState{
		ID:          "task-1",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	})
	state.Set("htn.selected_method", htn.MethodState{
		Name:         "code-new",
		TaskType:     core.TaskTypeCodeGeneration,
		SubtaskCount: 1,
	})
	state.Set("htn.plan", &core.Plan{
		Goal: "implement feature",
		Steps: []core.PlanStep{
			{ID: "code-new.plan", Description: "plan"},
		},
	})
	state.Set("htn.execution", htn.ExecutionState{
		PlannedStepCount:   1,
		CompletedSteps:     []string{"code-new.unknown"},
		CompletedStepCount: 1,
	})
	state.Set("htn.execution.completed_steps", []string{"code-new.unknown"})

	_, loaded, err := htn.LoadStateFromContext(state)
	if !loaded {
		t.Fatal("expected state to load")
	}
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestHTNAgent_RejectsInvalidCheckpointState(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErr(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "workflow-htn-invalid",
		TaskID:      "htn-invalid",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "resume invalid work",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))

	checkpointState := core.NewContext()
	checkpointState.Set("htn.task", htn.TaskState{
		ID:          "htn-invalid",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "resume invalid work",
	})
	checkpointState.Set("htn.selected_method", htn.MethodState{
		Name:         "code-new",
		TaskType:     core.TaskTypeCodeGeneration,
		SubtaskCount: 1,
	})
	checkpointState.Set("htn.plan", &core.Plan{
		Goal: "resume invalid work",
		Steps: []core.PlanStep{
			{ID: "code-new.plan", Description: "plan"},
		},
	})
	checkpointState.Set("htn.execution", htn.ExecutionState{
		PlannedStepCount:   1,
		CompletedSteps:     []string{"code-new.missing"},
		CompletedStepCount: 1,
	})
	checkpointState.Set("htn.execution.completed_steps", []string{"code-new.missing"})
	checkpointState.Set("plan.completed_steps", []string{"code-new.missing"})
	checkpointAdapter := agentpipeline.NewSQLitePipelineCheckpointStore(workflowStore, "workflow-htn-invalid", "seed-run")
	requireNoErr(t, checkpointAdapter.Save(&frameworkpipeline.Checkpoint{
		CheckpointID: "cp-invalid",
		TaskID:       "htn-invalid",
		StageName:    "code-new.plan",
		StageIndex:   0,
		CreatedAt:    time.Now().UTC(),
		Context:      checkpointState,
		Result: frameworkpipeline.StageResult{
			StageName:    "code-new.plan",
			ValidationOK: true,
			Transition: frameworkpipeline.StageTransition{
				Kind: frameworkpipeline.TransitionNext,
			},
		},
	}))

	agent := &htn.HTNAgent{
		Memory: frameworkmemory.NewCompositeRuntimeStore(workflowStore, nil, nil),
		Config: &core.Config{MaxIterations: 4},
	}
	if err := agent.Initialize(agent.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "htn-invalid",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "resume invalid work",
		Context:     map[string]any{"workflow_id": "workflow-htn-invalid"},
	}, core.NewContext())
	if err == nil {
		t.Fatal("expected invalid checkpoint state error")
	}
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// stubAgent is a test helper implementing graph.WorkflowExecutor.
type stubAgent struct {
	onExecute func(context.Context, *core.Task, *core.Context) (*core.Result, error)
}

func (s *stubAgent) Initialize(_ *core.Config) error { return nil }
func (s *stubAgent) Capabilities() []core.Capability { return nil }
func (s *stubAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("stub_done")
	_ = g.AddNode(done)
	_ = g.SetStart("stub_done")
	return g, nil
}
func (s *stubAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	return s.onExecute(ctx, task, state)
}

type stubInvocableCapability struct {
	desc   core.CapabilityDescriptor
	invoke func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error)
}

func (s *stubInvocableCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return s.desc
}

func (s *stubInvocableCapability) Invoke(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	return s.invoke(ctx, state, args)
}

func capabilityDescriptor(name string) core.CapabilityDescriptor {
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            name,
		Name:          name,
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Source:        core.CapabilitySource{Scope: core.CapabilityScopeBuiltin},
		TrustClass:    core.TrustClassBuiltinTrusted,
		Availability:  core.AvailabilitySpec{Available: true},
	})
}
