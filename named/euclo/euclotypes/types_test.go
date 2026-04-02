package euclotypes_test

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func TestCollectArtifactsFromState_IncludesWaiverArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.waiver", eucloruntime.ExecutionWaiver{
		WaiverID:  "waiver-1",
		Kind:      eucloruntime.WaiverKindVerification,
		Reason:    "operator approved degraded verification",
		GrantedBy: "operator",
	})

	artifacts := euclotypes.CollectArtifactsFromState(state)
	for _, artifact := range artifacts {
		if artifact.Kind == euclotypes.ArtifactKindWaiver {
			return
		}
	}
	t.Fatalf("expected waiver artifact in %#v", artifacts)
}

func TestCollectArtifactsFromState_IncludesVerificationPlanArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.verification_plan", map[string]any{
		"scope_kind": "explicit",
		"source":     "task_context",
	})

	artifacts := euclotypes.CollectArtifactsFromState(state)
	for _, artifact := range artifacts {
		if artifact.Kind == euclotypes.ArtifactKindVerificationPlan {
			return
		}
	}
	t.Fatalf("expected verification plan artifact in %#v", artifacts)
}

func TestCollectArtifactsFromState_IncludesTDDLifecycleArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.tdd.lifecycle", map[string]any{
		"current_phase": "green",
		"status":        "completed",
	})

	artifacts := euclotypes.CollectArtifactsFromState(state)
	for _, artifact := range artifacts {
		if artifact.Kind == euclotypes.ArtifactKindTDDLifecycle {
			return
		}
	}
	t.Fatalf("expected TDD lifecycle artifact in %#v", artifacts)
}

func TestAssembleFinalReport_IncludesWaiverAndAssuranceClass(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{
			ID:      "verification_plan",
			Kind:    euclotypes.ArtifactKindVerificationPlan,
			Summary: "verification scope selected",
			Payload: map[string]any{
				"scope_kind": "package_tests",
				"source":     "heuristic_go",
			},
		},
		{
			ID:      "tdd_lifecycle",
			Kind:    euclotypes.ArtifactKindTDDLifecycle,
			Summary: "TDD lifecycle complete",
			Payload: map[string]any{
				"current_phase": "complete",
				"status":        "completed",
				"phase_history": []map[string]any{
					{"phase": "red", "status": "completed"},
					{"phase": "green", "status": "completed"},
				},
			},
		},
		{
			ID:      "success_gate",
			Kind:    euclotypes.ArtifactKindSuccessGate,
			Summary: "completion gate evaluated",
			Payload: map[string]any{
				"allowed":         true,
				"reason":          "manual_verification_allowed",
				"assurance_class": "operator_deferred",
			},
		},
		{
			ID:      "waiver",
			Kind:    euclotypes.ArtifactKindWaiver,
			Summary: "operator waiver",
			Payload: map[string]any{
				"waiver_id":  "waiver-1",
				"kind":       "verification",
				"granted_by": "operator",
				"reason":     "continue without executable verification",
			},
		},
	}

	report := euclotypes.AssembleFinalReport(artifacts)
	if report["assurance_class"] != "operator_deferred" {
		t.Fatalf("expected assurance class in report, got %#v", report["assurance_class"])
	}
	if _, ok := report["verification_plan"].(map[string]any); !ok {
		t.Fatalf("expected verification plan payload in report, got %#v", report["verification_plan"])
	}
	if _, ok := report["tdd_lifecycle"].(map[string]any); !ok {
		t.Fatalf("expected TDD lifecycle payload in report, got %#v", report["tdd_lifecycle"])
	}
	if _, ok := report["waiver"].(map[string]any); !ok {
		t.Fatalf("expected waiver payload in report, got %#v", report["waiver"])
	}
}
