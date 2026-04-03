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
	d := NewDispatcher()

	for _, capabilityID := range []string{
		euclorelurpic.CapabilityChatAsk,
		euclorelurpic.CapabilityChatInspect,
		euclorelurpic.CapabilityChatImplement,
		euclorelurpic.CapabilityDebugInvestigate,
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

func TestExecuteRoutineUnknownReturnsNilArtifacts(t *testing.T) {
	d := &Dispatcher{behaviors: map[string]execution.Behavior{}, routines: map[string]euclorelurpic.SupportingRoutine{}}

	artifacts, err := d.ExecuteRoutine(context.Background(), "missing", nil, nil, runtimepkg.UnitOfWork{}, agentenv.AgentEnvironment{}, execution.ServiceBundle{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artifacts != nil {
		t.Fatalf("expected nil artifacts, got %+v", artifacts)
	}
}
