package orchestrate

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/interaction/gate"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

type stubCap struct {
	id        string
	eligible  bool
	status    euclotypes.ExecutionStatus
	summary   string
	artifacts []euclotypes.Artifact
	contract  euclotypes.ArtifactContract
	hint      *euclotypes.RecoveryHint
	executeFn func(context.Context, euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult
}

func (s *stubCap) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{ID: s.id, Name: s.id}
}

func (s *stubCap) Contract() euclotypes.ArtifactContract {
	if len(s.contract.ProducedOutputs) == 0 && len(s.contract.RequiredInputs) == 0 {
		return euclotypes.ArtifactContract{ProducedOutputs: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze}}
	}
	return s.contract
}

func (s *stubCap) Eligible(euclotypes.ArtifactState, euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	return euclotypes.EligibilityResult{Eligible: s.eligible}
}

func (s *stubCap) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	if s.executeFn != nil {
		return s.executeFn(ctx, env)
	}
	return euclotypes.ExecutionResult{
		Status:       s.status,
		Summary:      s.summary,
		Artifacts:    s.artifacts,
		RecoveryHint: s.hint,
	}
}

type stubRegistry struct {
	byProfile map[string][]CapabilityI
}

func (r stubRegistry) Lookup(id string) (CapabilityI, bool) {
	for _, caps := range r.byProfile {
		for _, c := range caps {
			if c.Descriptor().ID == id {
				return c, true
			}
		}
	}
	return nil, false
}

func (r stubRegistry) ForProfile(profileID string) []CapabilityI {
	if r.byProfile == nil {
		return nil
	}
	return append([]CapabilityI(nil), r.byProfile[profileID]...)
}

func testEnvelope() euclotypes.ExecutionEnvelope {
	reg := capability.NewRegistry()
	env := testutil.EnvMinimal()
	env.Registry = reg
	return euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "t-orc", Instruction: "analyze the failure"},
		State:       core.NewContext(),
		Registry:    reg,
		Environment: env,
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "phase_orchestrate_test"},
	}
}

func TestExecuteProfile_SucceedsWhenFirstEligibleCapabilityCompletes(t *testing.T) {
	saved := defaultSnapshotFunc
	defaultSnapshotFunc = func(reg interface{}) euclotypes.CapabilitySnapshot {
		if r, ok := reg.(*capability.Registry); ok {
			return eucloruntime.SnapshotCapabilities(r)
		}
		return euclotypes.CapabilitySnapshot{}
	}
	t.Cleanup(func() { defaultSnapshotFunc = saved })

	capOK := &stubCap{
		id:       "euclo:stub.analyze",
		eligible: true,
		status:   euclotypes.ExecutionStatusCompleted,
		summary:  "analysis ok",
		artifacts: []euclotypes.Artifact{{
			ID: "a1", Kind: euclotypes.ArtifactKindAnalyze, Summary: "ok",
			Payload: map[string]any{"finding": "x"}, ProducerID: "euclo:stub.analyze", Status: "produced",
		}},
	}
	pc := NewProfileController(
		stubRegistry{byProfile: map[string][]CapabilityI{"phase_orchestrate_test": {capOK}}},
		map[string][]gate.PhaseGate{},
		testutil.EnvMinimal(),
		euclotypes.DefaultExecutionProfileRegistry(),
		nil,
	)
	env := testEnvelope()
	res, detail, err := pc.ExecuteProfile(context.Background(),
		euclotypes.ExecutionProfileSelection{
			ProfileID:   "phase_orchestrate_test",
			PhaseRoutes: map[string]string{"analyze": "next"},
		},
		euclotypes.ModeResolution{ModeID: "debug"},
		env,
	)
	if err != nil {
		t.Fatalf("ExecuteProfile: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	if detail == nil || len(detail.CapabilityIDs) != 1 || detail.CapabilityIDs[0] != capOK.id {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	raw, ok := env.State.Get("euclo.artifacts")
	if !ok {
		t.Fatal("expected artifacts merged into state")
	}
	arts, ok := raw.([]euclotypes.Artifact)
	if !ok || len(arts) == 0 {
		t.Fatalf("expected artifact list in state, got %#v", raw)
	}
}

func TestProfileExecutionEngine_Execute_SucceedsWhenFirstEligibleCapabilityCompletes(t *testing.T) {
	saved := defaultSnapshotFunc
	defaultSnapshotFunc = func(reg interface{}) euclotypes.CapabilitySnapshot {
		if r, ok := reg.(*capability.Registry); ok {
			return eucloruntime.SnapshotCapabilities(r)
		}
		return euclotypes.CapabilitySnapshot{}
	}
	t.Cleanup(func() { defaultSnapshotFunc = saved })

	capOK := &stubCap{
		id:       "euclo:stub.analyze",
		eligible: true,
		status:   euclotypes.ExecutionStatusCompleted,
		summary:  "analysis ok",
		artifacts: []euclotypes.Artifact{{
			ID: "a1", Kind: euclotypes.ArtifactKindAnalyze, Summary: "ok",
			Payload: map[string]any{"finding": "x"}, ProducerID: "euclo:stub.analyze", Status: "produced",
		}},
	}
	pc := NewProfileController(
		stubRegistry{byProfile: map[string][]CapabilityI{"phase_orchestrate_test": {capOK}}},
		map[string][]gate.PhaseGate{},
		testutil.EnvMinimal(),
		euclotypes.DefaultExecutionProfileRegistry(),
		nil,
	)
	env := testEnvelope()
	res, detail, err := newProfileExecutionEngine(pc).Execute(context.Background(),
		euclotypes.ExecutionProfileSelection{
			ProfileID:   "phase_orchestrate_test",
			PhaseRoutes: map[string]string{"analyze": "next"},
		},
		euclotypes.ModeResolution{ModeID: "debug"},
		env,
	)
	if err != nil {
		t.Fatalf("engine Execute: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	if detail == nil || len(detail.CapabilityIDs) != 1 || detail.CapabilityIDs[0] != capOK.id {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestExecuteProfile_ReturnsErrorWhenProfileCapabilityFails(t *testing.T) {
	saved := defaultSnapshotFunc
	defaultSnapshotFunc = func(reg interface{}) euclotypes.CapabilitySnapshot {
		if r, ok := reg.(*capability.Registry); ok {
			return eucloruntime.SnapshotCapabilities(r)
		}
		return euclotypes.CapabilitySnapshot{}
	}
	t.Cleanup(func() { defaultSnapshotFunc = saved })

	capFail := &stubCap{
		id:       "euclo:stub.analyze_fail",
		eligible: true,
		status:   euclotypes.ExecutionStatusFailed,
		summary:  "boom",
	}
	pc := NewProfileController(
		stubRegistry{byProfile: map[string][]CapabilityI{"phase_orchestrate_test": {capFail}}},
		map[string][]gate.PhaseGate{},
		testutil.EnvMinimal(),
		euclotypes.DefaultExecutionProfileRegistry(),
		nil,
	)
	env := testEnvelope()
	res, _, err := pc.ExecuteProfile(context.Background(),
		euclotypes.ExecutionProfileSelection{
			ProfileID:   "phase_orchestrate_test",
			PhaseRoutes: map[string]string{"analyze": "next"},
		},
		euclotypes.ModeResolution{ModeID: "debug"},
		env,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if res == nil || res.Success {
		t.Fatalf("expected failed result, got %+v", res)
	}
}

func TestExecuteInteractive_ReturnsErrorWhenModeMissing(t *testing.T) {
	pc := NewProfileController(nil, nil, testutil.EnvMinimal(), nil, nil)
	reg := interaction.NewModeMachineRegistry()
	_, _, err := pc.ExecuteInteractive(context.Background(), reg,
		euclotypes.ModeResolution{ModeID: "chat"},
		testEnvelope(),
		&interaction.NoopEmitter{},
	)
	if err == nil {
		t.Fatal("expected error for unregistered interactive mode")
	}
}

func TestOrderedPhases_UsesGateChainWhenPresent(t *testing.T) {
	gates := []gate.PhaseGate{
		{From: gate.Phase("alpha"), To: gate.Phase("beta")},
		{From: gate.Phase("beta"), To: gate.Phase("gamma")},
	}
	got := OrderedPhases(nil, gates)
	if len(got) != 3 || got[0] != "alpha" || got[1] != "beta" || got[2] != "gamma" {
		t.Fatalf("unexpected order: %v", got)
	}
}

type recoveryStubRegistry struct {
	byID      map[string]CapabilityI
	byProfile map[string][]CapabilityI
}

func (r recoveryStubRegistry) Lookup(id string) (CapabilityI, bool) {
	if r.byID == nil {
		return nil, false
	}
	c, ok := r.byID[id]
	return c, ok
}

func (r recoveryStubRegistry) ForProfile(profileID string) []CapabilityI {
	if r.byProfile == nil {
		return nil
	}
	return append([]CapabilityI(nil), r.byProfile[profileID]...)
}

func TestAttemptRecovery_CapabilityFallbackRunsEligibleCapability(t *testing.T) {
	saved := defaultSnapshotFunc
	defaultSnapshotFunc = func(reg interface{}) euclotypes.CapabilitySnapshot {
		if r, ok := reg.(*capability.Registry); ok {
			return eucloruntime.SnapshotCapabilities(r)
		}
		return euclotypes.CapabilitySnapshot{}
	}
	t.Cleanup(func() { defaultSnapshotFunc = saved })

	fallback := &stubCap{
		id:       "euclo:stub.fallback_ok",
		eligible: true,
		status:   euclotypes.ExecutionStatusCompleted,
		summary:  "recovered",
		artifacts: []euclotypes.Artifact{{
			ID: "rec", Kind: euclotypes.ArtifactKindAnalyze, Summary: "ok",
			Payload: map[string]any{"from": "fallback"}, ProducerID: "euclo:stub.fallback_ok", Status: "produced",
		}},
	}
	rc := NewRecoveryController(
		recoveryStubRegistry{byID: map[string]CapabilityI{fallback.id: fallback}},
		nil,
		nil,
		testutil.EnvMinimal(),
	)
	failed := euclotypes.ExecutionResult{
		Status: euclotypes.ExecutionStatusFailed,
		Artifacts: []euclotypes.Artifact{{
			ProducerID: "euclo:stub.failed_cap",
			Kind:       euclotypes.ArtifactKindAnalyze,
			ID:         "fail-art", Summary: "boom",
		}},
	}
	hint := euclotypes.RecoveryHint{
		Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
		SuggestedCapability: fallback.id,
	}
	stack := NewRecoveryStack()
	env := testEnvelope()
	got := rc.AttemptRecovery(context.Background(), hint, failed, env, stack)
	if got.Status != euclotypes.ExecutionStatusCompleted {
		t.Fatalf("expected recovered status, got %+v", got)
	}
	if len(stack.Attempts) != 1 || !stack.Attempts[0].Success {
		t.Fatalf("expected one successful recovery attempt, got %+v", stack.Attempts)
	}
}
