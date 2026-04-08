package orchestrate

import (
	"reflect"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestRecoveryStackAndHelperMappings(t *testing.T) {
	stack := NewRecoveryStack()
	if !stack.CanAttempt() {
		t.Fatal("expected fresh recovery stack to allow attempts")
	}
	stack.Record(RecoveryAttempt{Level: RecoveryLevelCapability})
	stack.Record(RecoveryAttempt{Level: RecoveryLevelCapability})
	stack.Record(RecoveryAttempt{Level: RecoveryLevelCapability})
	if stack.CanAttempt() {
		t.Fatal("expected stack to be exhausted after max depth")
	}

	hint := euclotypes.RecoveryHint{
		Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "fallback-a",
		SuggestedParadigm:   "planning",
	}
	if got := fallbackCapabilitiesFromHint(hint); !reflect.DeepEqual(got, []string{"fallback-a"}) {
		t.Fatalf("unexpected fallback capabilities: %#v", got)
	}
	if got := fallbackCapabilitiesFromHint(euclotypes.RecoveryHint{SuggestedCapability: "a,b, c ; d"}); !reflect.DeepEqual(got, []string{"a,b, c ; d"}) {
		t.Fatalf("unexpected fallback capability suggestion handling: %#v", got)
	}
	if got := fallbackCapabilitiesFromHint(euclotypes.RecoveryHint{
		Context: map[string]any{"fallback_capabilities": []any{"x", "y", 3}},
	}); !reflect.DeepEqual(got, []string{"x", "y"}) {
		t.Fatalf("unexpected parsed fallback capabilities from context: %#v", got)
	}
	if got := firstFallbackCapability([]string{"x", "y"}); got != "x" {
		t.Fatalf("unexpected first fallback capability: %q", got)
	}
	if got := preferredFallbackProfile(euclotypes.RecoveryHint{Context: map[string]any{"preferred_profile": "cap"}}, []string{"one", "two"}); got != "cap" {
		t.Fatalf("unexpected preferred profile: %q", got)
	}
	if got := uniqueRecoveryStrings([]string{" a ", "a", "b", "", "b"}); !reflect.DeepEqual(got, []string{" a ", "a", "b"}) {
		t.Fatalf("unexpected unique recovery strings: %#v", got)
	}
}

func TestRecoveryTraceAndFailureHelpers(t *testing.T) {
	stack := NewRecoveryStack()
	stack.Record(RecoveryAttempt{Level: RecoveryLevelMode, Strategy: euclotypes.RecoveryStrategyModeEscalation, From: "code", To: "debug", Reason: "escalate", Success: true})
	artifact := RecoveryTraceArtifact(stack, "producer-1")
	if artifact.Kind != euclotypes.ArtifactKindRecoveryTrace || artifact.ProducerID != "producer-1" {
		t.Fatalf("unexpected recovery trace artifact: %#v", artifact)
	}
	if got := paradigmFromFailure(euclotypes.ExecutionResult{FailureInfo: &euclotypes.CapabilityFailure{ParadigmUsed: "react"}}); got != "react" {
		t.Fatalf("unexpected paradigm from failure: %q", got)
	}
	if got := producerIDFromFailure(euclotypes.ExecutionResult{Artifacts: []euclotypes.Artifact{{ProducerID: "cap-1"}}}); got != "cap-1" {
		t.Fatalf("unexpected producer from failure: %q", got)
	}
	if got := producerIDFromFailure(euclotypes.ExecutionResult{FailureInfo: &euclotypes.CapabilityFailure{Code: "cap-2"}}); got != "cap-2" {
		t.Fatalf("unexpected fallback producer id: %q", got)
	}
}

func TestControllerHelpersAndResultSynthesis(t *testing.T) {
	task := &core.Task{ID: "task-1", Instruction: "do work"}
	state := core.NewContext()
	artifacts := []euclotypes.Artifact{
		{ID: "a1", Kind: euclotypes.ArtifactKindAnalyze, Summary: "analyze", ProducerID: "cap-a", Payload: map[string]any{"x": 1}},
		{ID: "a2", Kind: euclotypes.ArtifactKindPlan, Summary: "plan", ProducerID: "cap-b", Payload: map[string]any{"y": 2}},
	}
	mergeCapabilityArtifactsToState(state, artifacts)
	if raw, ok := state.Get("euclo.artifacts"); !ok || len(raw.([]euclotypes.Artifact)) != 2 {
		t.Fatalf("expected artifacts merged into state, got %#v", raw)
	}
	if got := phaseExpectedArtifact("plan"); got != euclotypes.ArtifactKindPlan {
		t.Fatalf("unexpected phase artifact: %q", got)
	}
	if got := phaseExpectedArtifact("unknown"); got != "" {
		t.Fatalf("expected unknown phase to map to empty kind, got %q", got)
	}

	pcResult := &ProfileControllerResult{
		CapabilityIDs:  []string{"cap-a"},
		PhasesExecuted: []string{"plan"},
		PhaseRecords: []PhaseArtifactRecord{{
			Phase:             "plan",
			ArtifactsProduced: artifacts[:1],
			ArtifactsConsumed: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake},
		}},
		EarlyStop:        true,
		EarlyStopPhase:   "verify",
		RecoveryAttempts: 2,
	}
	recordProfileControllerObservability(state, pcResult, euclotypes.ModeResolution{ModeID: "debug"}, euclotypes.ExecutionProfileSelection{ProfileID: "profile-a"})
	if _, ok := state.Get("euclo.profile_controller"); !ok {
		t.Fatal("expected controller observability in state")
	}

	records := buildProfileCapabilityPhaseRecords([]string{"plan", "verify"}, []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake}, artifacts)
	if len(records) != 2 || len(records[0].ArtifactsProduced) != 1 {
		t.Fatalf("unexpected phase records: %#v", records)
	}
	stateRecords := profilePhaseRecordsState(records)
	if len(stateRecords) != 2 {
		t.Fatalf("unexpected phase record state: %#v", stateRecords)
	}
	if got := artifactKindsFromState(euclotypes.NewArtifactState(artifacts)); !reflect.DeepEqual(got, []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze, euclotypes.ArtifactKindPlan}) {
		t.Fatalf("unexpected artifact kinds from state: %#v", got)
	}
	if got := artifactKindsFromArtifacts(artifacts); !reflect.DeepEqual(got, []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze, euclotypes.ArtifactKindPlan}) {
		t.Fatalf("unexpected artifact kinds from artifacts: %#v", got)
	}
	if got := artifactKindsToStrings([]euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze, ""}); !reflect.DeepEqual(got, []string{"euclo.analyze"}) {
		t.Fatalf("unexpected artifact kind strings: %#v", got)
	}
	if got := filterArtifactsByKind(artifacts, euclotypes.ArtifactKindPlan); len(got) != 1 || got[0].ID != "a2" {
		t.Fatalf("unexpected filtered artifacts: %#v", got)
	}
	if got := appendUniqueArtifactKinds([]euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan}, euclotypes.ArtifactKindPlan, euclotypes.ArtifactKindAnalyze); !reflect.DeepEqual(got, []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan, euclotypes.ArtifactKindAnalyze}) {
		t.Fatalf("unexpected appended unique artifact kinds: %#v", got)
	}

	if got := taskNodeID(task); got != "task-1" {
		t.Fatalf("unexpected task node id: %q", got)
	}
	if got := taskNodeID(nil); got != "euclo" {
		t.Fatalf("expected nil task node id to default to euclo, got %q", got)
	}

	if got := partialResult(task, pcResult); got == nil || got.NodeID != "task-1" || got.Data["status"] != "partial" {
		t.Fatalf("unexpected partial result: %#v", got)
	}
	failed := failedResult(task, euclotypes.ExecutionResult{Summary: "nope"}, pcResult)
	if failed == nil || failed.Success || failed.Data["status"] != "" {
		t.Fatalf("unexpected failed result: %#v", failed)
	}
	success := successResult(task, euclotypes.ExecutionResult{Summary: "ok"}, pcResult)
	if success == nil || !success.Success || success.Data["status"] != "" {
		t.Fatalf("unexpected success result: %#v", success)
	}
	completed := completedResult(task, pcResult)
	if completed == nil || !completed.Success || completed.Data["status"] != "completed" {
		t.Fatalf("unexpected completed result: %#v", completed)
	}
}

func TestSnapshotAndRecoveryHelpers(t *testing.T) {
	registry := capability.NewRegistry()
	snapshot := snapshotFromEnv(euclotypes.ExecutionEnvelope{Registry: registry})
	if snapshot.HasReadTools || snapshot.HasExecuteTools {
		t.Fatalf("expected empty registry snapshot, got %#v", snapshot)
	}

	phaseRoutes := map[string]string{"beta": "next", "alpha": "start"}
	if got := OrderedPhases(phaseRoutes, nil); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("unexpected ordered phases: %#v", got)
	}

	if !shouldAttemptRecovery(euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusPartial, RecoveryHint: &euclotypes.RecoveryHint{Strategy: euclotypes.RecoveryStrategyModeEscalation}}) {
		t.Fatal("expected partial result with recovery hint to request recovery")
	}
	if shouldAttemptRecovery(euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, RecoveryHint: &euclotypes.RecoveryHint{Strategy: euclotypes.RecoveryStrategyModeEscalation}}) {
		t.Fatal("expected completed result not to request recovery")
	}
}

func TestResolveAndAdapterHelpers(t *testing.T) {
	capA := &stubCap{id: "cap-a", eligible: true, status: euclotypes.ExecutionStatusCompleted}
	capB := &stubCap{id: "cap-b", eligible: false, status: euclotypes.ExecutionStatusCompleted}
	pc := &ProfileController{
		Capabilities: stubRegistry{byProfile: map[string][]CapabilityI{"profile-a": {capA, capB}}},
	}
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindAnalyze}})
	snapshot := euclotypes.CapabilitySnapshot{}
	if got := pc.resolveProfileCapability("profile-a", state, snapshot); got == nil || got.Descriptor().ID != "cap-a" {
		t.Fatalf("unexpected profile capability resolution: %#v", got)
	}
	if got := pc.resolveCapabilityForPhase("analyze", "profile-a", state, snapshot); got == nil || got.Descriptor().ID != "cap-a" {
		t.Fatalf("unexpected phase capability resolution: %#v", got)
	}
	if got := pc.resolveFallbackCapability("analyze", "profile-a", "cap-a", state, snapshot); got != nil {
		t.Fatalf("expected fallback to skip excluded capability, got %#v", got)
	}

	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(capA)
	adapted := AdaptCapabilityRegistry(reg)
	if adapted == nil {
		t.Fatal("expected capability registry adapter")
	}
	if _, ok := adapted.Lookup("cap-a"); !ok {
		t.Fatal("expected adapter lookup to succeed")
	}
}
