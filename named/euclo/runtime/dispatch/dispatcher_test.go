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

type stubInvocable struct {
	id    string
	mu    sync.Mutex
	calls int
}

func (s *stubInvocable) ID() string { return s.id }
func (s *stubInvocable) IsPrimary() bool { return true }
func (s *stubInvocable) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if in.InvokeSupporting == nil {
		return nil, fmt.Errorf("supporting routine executor missing")
	}
	return &core.Result{Success: true, Data: map[string]any{"capability": in.Work.PrimaryRelurpicCapabilityID}}, nil
}

type stubSupporting struct {
	id    string
	input execution.InvokeInput
}

func (s *stubSupporting) ID() string { return s.id }
func (s *stubSupporting) IsPrimary() bool { return false }
func (s *stubSupporting) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	s.input = in
	return &core.Result{Success: true, Data: map[string]any{"artifacts": []euclotypes.Artifact{{ID: "a", Kind: euclotypes.ArtifactKindTrace, Summary: "ok", Payload: "ok"}}}}, nil
}

func TestNewDispatcherRegistersPrimaryAndSupportingCapabilities(t *testing.T) {
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
		euclorelurpic.CapabilityDeferralsSurface,
		euclorelurpic.CapabilityLearningPromote,
	} {
		if _, ok := d.invocables[capabilityID]; !ok {
			t.Fatalf("expected invocable %q to be registered", capabilityID)
		}
	}
}

func TestExecuteDispatchesKnownInvocable(t *testing.T) {
	inv := &stubInvocable{id: "cap.test"}
	d := &Dispatcher{invocables: map[string]execution.Invocable{inv.id: inv}}

	result, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{PrimaryRelurpicCapabilityID: inv.id},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecuteDispatchesRecipeLikeInvocableViaRegistry(t *testing.T) {
	inv := &stubInvocable{id: "euclo:recipe.demo"}
	d := &Dispatcher{invocables: map[string]execution.Invocable{}}
	if err := d.Register(inv); err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{PrimaryRelurpicCapabilityID: inv.id},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
	if inv.calls != 1 {
		t.Fatalf("expected one invocation, got %d", inv.calls)
	}
}

func TestExecuteUnknownInvocableReturnsError(t *testing.T) {
	d := &Dispatcher{invocables: map[string]execution.Invocable{}}

	_, err := d.Execute(context.Background(), execution.ExecuteInput{
		Work: runtimepkg.UnitOfWork{PrimaryRelurpicCapabilityID: "missing"},
	})
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestInvokeSupportingPassesWorkContext(t *testing.T) {
	support := &stubSupporting{id: "routine.test"}
	d := &Dispatcher{invocables: map[string]execution.Invocable{support.id: support}}
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

	artifacts, err := d.ExecuteRoutine(context.Background(), support.id, task, state, work, testutil.Env(t), execution.ServiceBundle{})
	if err != nil {
		t.Fatalf("ExecuteRoutine: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected artifacts, got %+v", artifacts)
	}
	if support.input.Task != task || support.input.State != state {
		t.Fatal("expected task and state to be forwarded")
	}
	if support.input.Work.PrimaryRelurpicCapabilityID != "primary" {
		t.Fatalf("unexpected primary capability: %+v", support.input.Work)
	}
	if len(support.input.Work.SemanticInputs.PatternRefs) != 1 || support.input.Work.SemanticInputs.PatternRefs[0] != "pattern-1" {
		t.Fatalf("unexpected routine input: %+v", support.input.Work)
	}
}

func TestExecuteSequence_AND_SharedState(t *testing.T) {
	inv1 := &stubInvocable{id: "cap.test1"}
	inv2 := &stubInvocable{id: "cap.test2"}
	d := &Dispatcher{
		invocables: map[string]execution.Invocable{
			inv1.id: inv1,
			inv2.id: inv2,
		},
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
	if inv1.calls != 1 || inv2.calls != 1 {
		t.Fatalf("unexpected invocation counts: %d %d", inv1.calls, inv2.calls)
	}
	if _, ok := state.Get("euclo.sequence_step_1_completed"); !ok {
		t.Error("expected step 1 completion marker in state")
	}
	if _, ok := state.Get("euclo.sequence_step_2_completed"); !ok {
		t.Error("expected step 2 completion marker in state")
	}
}

func TestExecuteSequence_OR_RunsFirstOnly(t *testing.T) {
	inv1 := &stubInvocable{id: "cap.test1"}
	inv2 := &stubInvocable{id: "cap.test2"}
	d := &Dispatcher{
		invocables: map[string]execution.Invocable{
			inv1.id: inv1,
			inv2.id: inv2,
		},
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
	if inv1.calls != 1 || inv2.calls != 0 {
		t.Fatalf("unexpected invocation counts: %d %d", inv1.calls, inv2.calls)
	}
	if got, ok := state.Get("euclo.or_selected_capability"); !ok || got != "cap.test1" {
		t.Fatalf("unexpected selected capability: %#v", got)
	}
}
