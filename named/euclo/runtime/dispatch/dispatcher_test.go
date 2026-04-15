package dispatch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

type stubBehavior struct {
	id    string
	mu    sync.Mutex
	calls int
}

func (s *stubBehavior) ID() string { return s.id }
func (s *stubBehavior) Execute(_ context.Context, in execution.ExecuteInput) (*core.Result, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if in.RunSupportingRoutine == nil {
		return nil, fmt.Errorf("supporting routine executor missing")
	}
	return &core.Result{Success: true, Data: map[string]any{"capability": in.Work.PrimaryRelurpicCapabilityID}}, nil
}

type stubRoutine struct {
	id    string
	input euclorelurpic.RoutineInput
}

func (s *stubRoutine) ID() string { return s.id }
func (s *stubRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	s.input = in
	return []euclotypes.Artifact{{ID: "a", Kind: euclotypes.ArtifactKindTrace, Summary: "ok", Payload: "ok"}}, nil
}

func TestNewDispatcherRegistersPrimaryCapabilities(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})

	for _, capabilityID := range []string{
		euclorelurpic.CapabilityChatAsk,
		euclorelurpic.CapabilityChatInspect,
		euclorelurpic.CapabilityChatImplement,
		euclorelurpic.CapabilityDebugInvestigateRepair,
		euclorelurpic.CapabilityDebugRepairSimple,
		euclorelurpic.CapabilityArchaeologyExplore,
		euclorelurpic.CapabilityArchaeologyCompilePlan,
		euclorelurpic.CapabilityArchaeologyImplement,
	} {
		if _, ok := d.behaviors[capabilityID]; !ok {
			t.Fatalf("expected behavior %q to be registered", capabilityID)
		}
	}
	if len(d.routines) == 0 {
		t.Fatal("expected supporting routines to be registered")
	}
}

func TestExecuteDispatchesKnownBehavior(t *testing.T) {
	behavior := &stubBehavior{id: "cap.test"}
	d := &Dispatcher{behaviors: map[string]execution.Behavior{behavior.id: behavior}, routines: map[string]euclorelurpic.SupportingRoutine{}}

	result, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{PrimaryRelurpicCapabilityID: behavior.id},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecuteUnknownBehaviorReturnsError(t *testing.T) {
	d := &Dispatcher{behaviors: map[string]execution.Behavior{}, routines: map[string]euclorelurpic.SupportingRoutine{}}

	_, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{PrimaryRelurpicCapabilityID: "missing"},
	})
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestExecuteRoutinePassesWorkContext(t *testing.T) {
	routine := &stubRoutine{id: "routine.test"}
	d := &Dispatcher{behaviors: map[string]execution.Behavior{}, routines: map[string]euclorelurpic.SupportingRoutine{routine.id: routine}}
	task := &core.Task{ID: "task-1"}
	state := core.NewContext()
	work := runtimepkg.UnitOfWork{
		PrimaryRelurpicCapabilityID:     "primary",
		SupportingRelurpicCapabilityIDs: []string{"support"},
		SemanticInputs: runtimepkg.SemanticInputBundle{
			PatternRefs:           []string{"pattern-1"},
			TensionRefs:           []string{"tension-1"},
			ProspectiveRefs:       []string{"prospective-1"},
			ConvergenceRefs:       []string{"convergence-1"},
			RequestProvenanceRefs: []string{"request-1"},
		},
	}

	artifacts, err := d.ExecuteRoutine(context.Background(), routine.id, task, state, work, testutil.Env(t), execution.ServiceBundle{})
	if err != nil {
		t.Fatalf("ExecuteRoutine: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected artifacts, got %+v", artifacts)
	}
	if routine.input.Task != task || routine.input.State != state {
		t.Fatal("expected task and state to be forwarded")
	}
	if routine.input.Work.PrimaryCapabilityID != "primary" {
		t.Fatalf("unexpected primary capability: %+v", routine.input.Work)
	}
	if len(routine.input.Work.PatternRefs) != 1 || routine.input.Work.PatternRefs[0] != "pattern-1" {
		t.Fatalf("unexpected routine input: %+v", routine.input.Work)
	}
}

func TestExecuteRoutineUnknownReturnsError(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})

	artifacts, err := d.ExecuteRoutine(context.Background(), "missing", nil, nil, runtimepkg.UnitOfWork{}, agentenv.AgentEnvironment{}, execution.ServiceBundle{})
	if err == nil {
		t.Fatal("expected error for unknown routine, got nil")
	}
	if artifacts != nil {
		t.Fatalf("expected nil artifacts, got %+v", artifacts)
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' error, got: %v", err)
	}
}

// TestExecuteSequence_AND_SharedState verifies AND sequence executes all steps and accumulates state
func TestExecuteSequence_AND_SharedState(t *testing.T) {
	behavior1 := &stubBehavior{id: "cap.test1"}
	behavior2 := &stubBehavior{id: "cap.test2"}
	d := &Dispatcher{
		behaviors: map[string]execution.Behavior{
			behavior1.id: behavior1,
			behavior2.id: behavior2,
		},
		routines: map[string]euclorelurpic.SupportingRoutine{},
	}

	state := core.NewContext()
	result, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{
			CapabilityExecutionSequence: []string{"cap.test1", "cap.test2"},
			CapabilitySequenceOperator:  "AND",
		},
		State: state,
	})

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}

	// Verify both behaviors were called
	if behavior1.calls != 1 {
		t.Errorf("expected behavior1 to be called once, got %d", behavior1.calls)
	}
	if behavior2.calls != 1 {
		t.Errorf("expected behavior2 to be called once, got %d", behavior2.calls)
	}

	// Verify state was updated with step completion markers
	if _, ok := state.Get("euclo.sequence_step_1_completed"); !ok {
		t.Error("expected step 1 completion marker in state")
	}
	if _, ok := state.Get("euclo.sequence_step_2_completed"); !ok {
		t.Error("expected step 2 completion marker in state")
	}
}

// TestExecuteSequence_OR_RunsFirstOnly verifies OR sequence executes only the first step
func TestExecuteSequence_OR_RunsFirstOnly(t *testing.T) {
	behavior1 := &stubBehavior{id: "cap.test1"}
	behavior2 := &stubBehavior{id: "cap.test2"}
	d := &Dispatcher{
		behaviors: map[string]execution.Behavior{
			behavior1.id: behavior1,
			behavior2.id: behavior2,
		},
		routines: map[string]euclorelurpic.SupportingRoutine{},
	}

	state := core.NewContext()
	result, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{
			CapabilityExecutionSequence: []string{"cap.test1", "cap.test2"},
			CapabilitySequenceOperator:  "OR",
		},
		State: state,
	})

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}

	// Verify only first behavior was called
	if behavior1.calls != 1 {
		t.Errorf("expected behavior1 to be called once, got %d", behavior1.calls)
	}
	if behavior2.calls != 0 {
		t.Errorf("expected behavior2 to NOT be called, got %d", behavior2.calls)
	}

	// Verify OR selection was recorded in state
	selected := state.GetString("euclo.or_selected_capability")
	if selected != "cap.test1" {
		t.Errorf("expected OR selected capability marker 'cap.test1', got %v", selected)
	}
}

// TestExecuteSequence_AND_StopsOnFailure verifies AND sequence aborts on first failure
func TestExecuteSequence_AND_StopsOnFailure(t *testing.T) {
	behavior1 := &stubBehavior{id: "cap.test1"}
	failingBehavior := &stubFailingBehavior{id: "cap.failing"}
	d := &Dispatcher{
		behaviors: map[string]execution.Behavior{
			behavior1.id:       behavior1,
			failingBehavior.id: failingBehavior,
		},
		routines: map[string]euclorelurpic.SupportingRoutine{},
	}

	state := core.NewContext()
	_, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{
			CapabilityExecutionSequence: []string{"cap.test1", "cap.failing", "cap.test2"},
			CapabilitySequenceOperator:  "AND",
		},
		State: state,
	})

	if err == nil {
		t.Fatal("expected error from failing step, got nil")
	}
	if !strings.Contains(err.Error(), "cap.failing") {
		t.Errorf("expected error to mention failing capability, got: %v", err)
	}

	// Verify first behavior was called but sequence stopped before step 3
	if behavior1.calls != 1 {
		t.Errorf("expected behavior1 to be called once, got %d", behavior1.calls)
	}
	if _, ok := state.Get("euclo.sequence_step_1_completed"); !ok {
		t.Error("expected step 1 completion marker in state")
	}
	if _, ok := state.Get("euclo.sequence_step_2_completed"); ok {
		t.Error("step 2 should NOT have completed marker (it failed)")
	}
}

// TestExecuteSequence_SingleElement_CompatPath verifies single element uses existing Execute path
func TestExecuteSequence_SingleElement_CompatPath(t *testing.T) {
	behavior := &stubBehavior{id: "cap.single"}
	d := &Dispatcher{
		behaviors: map[string]execution.Behavior{behavior.id: behavior},
		routines:  map[string]euclorelurpic.SupportingRoutine{},
	}

	result, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{
			CapabilityExecutionSequence: []string{"cap.single"},
			CapabilitySequenceOperator:  "AND",
			PrimaryRelurpicCapabilityID: "cap.single",
		},
	})

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
	if behavior.calls != 1 {
		t.Errorf("expected behavior to be called once, got %d", behavior.calls)
	}
}

// TestExecuteSequence_EmptySequence_ReturnsError verifies empty sequence returns error
func TestExecuteSequence_EmptySequence_ReturnsError(t *testing.T) {
	d := &Dispatcher{
		behaviors: map[string]execution.Behavior{},
		routines:  map[string]euclorelurpic.SupportingRoutine{},
	}

	_, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{
			CapabilityExecutionSequence: []string{},
			CapabilitySequenceOperator:  "AND",
		},
	})

	if err == nil {
		t.Fatal("expected error for empty sequence, got nil")
	}
	if !strings.Contains(err.Error(), "no capability ID") {
		t.Errorf("expected 'no capability ID' error, got: %v", err)
	}
}

// TestExecuteSequence_UnknownCapability_ReturnsError verifies unknown capability returns error
func TestExecuteSequence_UnknownCapability_ReturnsError(t *testing.T) {
	behavior1 := &stubBehavior{id: "cap.known"}
	d := &Dispatcher{
		behaviors: map[string]execution.Behavior{
			behavior1.id: behavior1,
		},
		routines: map[string]euclorelurpic.SupportingRoutine{},
	}

	_, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{
			CapabilityExecutionSequence: []string{"cap.known", "cap.unknown"},
			CapabilitySequenceOperator:  "AND",
		},
	})

	if err == nil {
		t.Fatal("expected error for unknown capability, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' error, got: %v", err)
	}
}

type stubFailingBehavior struct {
	id string
}

func (s *stubFailingBehavior) ID() string { return s.id }
func (s *stubFailingBehavior) Execute(_ context.Context, in execution.ExecuteInput) (*core.Result, error) {
	return nil, fmt.Errorf("intentional failure for %s", s.id)
}
