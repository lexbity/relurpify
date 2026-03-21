package proof

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/rex/classify"
	"github.com/lexcodex/relurpify/named/rex/route"
)

func TestEvaluateCompletionBlocksMutationWithoutVerification(t *testing.T) {
	decision := route.RouteDecision{Family: route.FamilyArchitect, RequireProof: true}
	class := classify.Classification{MutationCapable: true}
	result := EvaluateCompletion(decision, class, core.NewContext())
	if result.Allowed {
		t.Fatalf("expected blocked completion")
	}
}

func TestBuildActionLogIncludesRoute(t *testing.T) {
	log := BuildActionLog(route.RouteDecision{Family: route.FamilyReAct, Mode: "open", Profile: "managed"}, classify.Classification{Intent: "analysis", RiskLevel: "low", ReadOnly: true}, core.NewContext())
	if len(log) == 0 || log[0].Kind != "route" {
		t.Fatalf("unexpected action log: %+v", log)
	}
}

func TestBuildProofSurfaceMarksWorkflowRetrievalUsage(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.workflow_retrieval", map[string]any{"summary": "prior plan"})
	state.Set("rex.artifact_kinds", []string{"rex.proof_surface", "rex.workflow_retrieval"})
	proof := BuildProofSurface(route.RouteDecision{Family: route.FamilyArchitect, Mode: "planning", Profile: "managed"}, &core.Result{}, state)
	if !proof.WorkflowRetrieval {
		t.Fatalf("expected workflow retrieval usage in proof: %+v", proof)
	}
	if len(proof.ArtifactKinds) != 2 {
		t.Fatalf("unexpected artifact kinds: %+v", proof.ArtifactKinds)
	}
}

func TestEvaluateCompletionAcceptsPassingVerificationForMutation(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "tests passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result := EvaluateCompletion(route.RouteDecision{Family: route.FamilyArchitect, Mode: "mutation", Profile: "managed", RequireProof: true}, classify.Classification{MutationCapable: true}, state)
	if !result.Allowed {
		t.Fatalf("expected allowed completion, got %+v", result)
	}
	if result.Reason != "verification_accepted" {
		t.Fatalf("unexpected reason: %+v", result)
	}
}

func TestBuildProofSurfaceIncludesSuccessGateMetadata(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "tests passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_ = EvaluateCompletion(route.RouteDecision{Family: route.FamilyArchitect, Mode: "mutation", Profile: "managed", RequireProof: true}, classify.Classification{MutationCapable: true}, state)
	proof := BuildProofSurface(route.RouteDecision{Family: route.FamilyArchitect, Mode: "mutation", Profile: "managed"}, &core.Result{}, state)
	if !proof.VerificationEvidence {
		t.Fatalf("expected verification evidence in proof: %+v", proof)
	}
	if proof.SuccessGateReason != "verification_accepted" {
		t.Fatalf("unexpected success gate reason: %+v", proof)
	}
}

func TestBuildActionLogIncludesWorkflowRetrieval(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.workflow_retrieval", map[string]any{"summary": "prior plan"})
	log := BuildActionLog(route.RouteDecision{Family: route.FamilyArchitect, Mode: "planning", Profile: "managed"}, classify.Classification{Intent: "analysis", RiskLevel: "medium", ReadOnly: false}, state)
	found := false
	for _, entry := range log {
		if entry.Kind == "workflow_retrieval" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected workflow retrieval action log entry: %+v", log)
	}
}

func TestBuildActionLogIncludesVerificationAndSuccessGate(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "tests passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_ = EvaluateCompletion(route.RouteDecision{Family: route.FamilyArchitect, Mode: "mutation", Profile: "managed", RequireProof: true}, classify.Classification{MutationCapable: true}, state)
	log := BuildActionLog(route.RouteDecision{Family: route.FamilyArchitect, Mode: "mutation", Profile: "managed"}, classify.Classification{Intent: "mutation", RiskLevel: "high", ReadOnly: false}, state)
	foundVerification := false
	foundGate := false
	for _, entry := range log {
		if entry.Kind == "verification" {
			foundVerification = true
		}
		if entry.Kind == "success_gate" {
			foundGate = true
		}
	}
	if !foundVerification || !foundGate {
		t.Fatalf("expected verification and success_gate entries: %+v", log)
	}
}
