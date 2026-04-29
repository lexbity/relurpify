package proof

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/classify"
	"codeburg.org/lexbit/relurpify/named/rex/route"
)

func TestEvaluateCompletionBlocksMutationWithoutVerification(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	decision := route.RouteDecision{Family: route.FamilyArchitect, RequireProof: true, Mode: "mutation", Profile: "managed"}
	class := classify.Classification{MutationCapable: true}
	result := EvaluateCompletion(decision, class, env)
	if result.Allowed {
		t.Fatalf("expected blocked completion")
	}
}

func TestBuildActionLogIncludesRoute(t *testing.T) {
	log := BuildActionLog(route.RouteDecision{Family: route.FamilyReAct, Mode: "open", Profile: "managed"}, classify.Classification{Intent: "analysis", RiskLevel: "low", ReadOnly: true}, contextdata.NewEnvelope("task-1", ""))
	if len(log) == 0 || log[0].Kind != "route" {
		t.Fatalf("unexpected action log: %+v", log)
	}
}

func TestBuildProofSurfaceMarksWorkflowRetrievalUsage(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	env.SetWorkingValue("pipeline.workflow_retrieval", map[string]any{"summary": "prior plan"}, contextdata.MemoryClassTask)
	env.SetWorkingValue("rex.artifact_kinds", []string{"rex.proof_surface", "rex.workflow_retrieval"}, contextdata.MemoryClassTask)
	proof := BuildProofSurface(route.RouteDecision{Family: route.FamilyArchitect, Mode: "planning", Profile: "managed"}, &core.Result{}, env)
	if !proof.WorkflowRetrieval {
		t.Fatalf("expected workflow retrieval usage in proof: %+v", proof)
	}
	if len(proof.ArtifactKinds) != 2 {
		t.Fatalf("unexpected artifact kinds: %+v", proof.ArtifactKinds)
	}
}

func TestEvaluateCompletionAcceptsPassingVerificationForMutation(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	env.SetWorkingValue("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "tests passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	}, contextdata.MemoryClassTask)
	result := EvaluateCompletion(route.RouteDecision{Family: route.FamilyArchitect, Mode: "mutation", Profile: "managed", RequireProof: true}, classify.Classification{MutationCapable: true}, env)
	if !result.Allowed {
		t.Fatalf("expected allowed completion, got %+v", result)
	}
	if result.Reason != "verification_accepted" {
		t.Fatalf("unexpected reason: %+v", result)
	}
}
