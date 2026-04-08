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

func TestProofHelpersCoverPolicyAndEvidenceBranches(t *testing.T) {
	state := core.NewContext()
	state.Set("rex.verification_status", "pass")
	if got := verificationStatus(state); got != "pass" {
		t.Fatalf("verificationStatus = %q", got)
	}
	if got := proofProfileID(route.RouteDecision{Family: route.FamilyPlanner, Profile: "x"}); got != "read_only_review" {
		t.Fatalf("proofProfileID planner = %q", got)
	}
	if got := proofProfileID(route.RouteDecision{Family: route.FamilyPipeline, Profile: "x"}); got != "structured_verification" {
		t.Fatalf("proofProfileID pipeline = %q", got)
	}
	if got := proofProfileID(route.RouteDecision{Family: route.FamilyArchitect, Profile: "x"}); got != "edit_verify_repair" {
		t.Fatalf("proofProfileID architect = %q", got)
	}
	if got := proofProfileID(route.RouteDecision{Family: route.FamilyReAct, Profile: "managed"}); got != "rex/managed" {
		t.Fatalf("proofProfileID default = %q", got)
	}

	evidence := VerificationEvidenceRecord{Status: "pass", EvidencePresent: true}
	policy := VerificationPolicy{RequiresVerification: true, AcceptedStatuses: []string{"pass"}, RequiresExecutedCheck: true}
	if result := EvaluateSuccessGate(policy, evidence); result.Reason != "verification_check_missing" {
		t.Fatalf("unexpected success gate: %+v", result)
	}
	evidence.Checks = []VerificationCheckRecord{{Status: "pass"}}
	if result := EvaluateSuccessGate(policy, evidence); !result.Allowed || result.Reason != "verification_accepted" {
		t.Fatalf("unexpected success gate accepted: %+v", result)
	}
	if result := EvaluateSuccessGate(VerificationPolicy{RequiresVerification: false}, VerificationEvidenceRecord{}); !result.Allowed || result.Reason != "verification_not_required" {
		t.Fatalf("unexpected non-required gate: %+v", result)
	}
	if result := EvaluateSuccessGate(VerificationPolicy{RequiresVerification: true, AcceptedStatuses: []string{"pass"}, ManualOutcomeAllowed: true}, VerificationEvidenceRecord{Status: "needs_manual_verification", EvidencePresent: true, Checks: []VerificationCheckRecord{{Status: "pass"}}}); result.Reason != "manual_verification_allowed" {
		t.Fatalf("unexpected manual verification gate: %+v", result)
	}
}

func TestVerificationEvidenceAndResolvePolicyBranches(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{"status": "pass", "summary": "ok", "checks": []any{map[string]any{"status": "pass"}}})
	evidence := VerificationEvidence(state)
	if !evidence.EvidencePresent || evidence.Status != "pass" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
	if got := verificationEvidenceFromRaw("passed by log"); got.Status != "pass" {
		t.Fatalf("unexpected raw evidence: %+v", got)
	}
	if got := verificationEvidenceFromRaw(map[string]any{"status": "pass", "checks": []any{map[string]any{"status": "pass"}}}); got.Status != "pass" || len(got.Checks) != 1 {
		t.Fatalf("unexpected map evidence: %+v", got)
	}
	if got := verificationEvidenceFromRaw(nil); got.Status != "not_verified" {
		t.Fatalf("unexpected nil evidence: %+v", got)
	}

	policy := ResolveVerificationPolicy(route.RouteDecision{Family: route.FamilyArchitect, Mode: "mutation", Profile: "managed", RequireProof: true}, classify.Classification{ReadOnly: false})
	if !policy.RequiresVerification || !policy.RequiresExecutedCheck || policy.ManualOutcomeAllowed {
		t.Fatalf("unexpected policy: %+v", policy)
	}
	policy = ResolveVerificationPolicy(route.RouteDecision{Family: route.FamilyPlanner, Mode: "planning", Profile: "managed", RequireProof: true}, classify.Classification{ReadOnly: true})
	if policy.RequiresVerification || policy.RequiresExecutedCheck {
		t.Fatalf("planning/read-only policy should relax checks: %+v", policy)
	}
	if policy := ResolveVerificationPolicy(route.RouteDecision{Family: route.FamilyReAct, Mode: "open", Profile: "managed"}, classify.Classification{}); policy.RequiresVerification {
		t.Fatalf("open mode should not require verification: %+v", policy)
	}
}

func TestBuildProofSurfaceUsesCompletionAndVerificationState(t *testing.T) {
	state := core.NewContext()
	state.Set("rex.recovery_attempts", 2)
	state.Set("rex.artifact_kinds", []string{"a", "b"})
	state.Set("pipeline.verify", map[string]any{
		"status":  "needs_manual_verification",
		"summary": "manual review needed",
		"checks":  []any{map[string]any{"status": "pass"}},
	})
	state.Set("rex.success_gate", SuccessGateResult{Allowed: true, Reason: "manual_verification_allowed"})
	_ = EvaluateCompletion(route.RouteDecision{Family: route.FamilyPipeline, Mode: "structured", Profile: "managed", RequireProof: true}, classify.Classification{ReadOnly: false}, state)
	proof := BuildProofSurface(route.RouteDecision{Family: route.FamilyPipeline, Mode: "structured", Profile: "managed"}, &core.Result{}, state)
	if !proof.VerificationEvidence || proof.CompletionAllowed || proof.RecoveryCount != 2 {
		t.Fatalf("unexpected proof surface: %+v", proof)
	}
	if proof.SuccessGateReason != "verification_status_rejected" || len(proof.ArtifactKinds) != 2 {
		t.Fatalf("unexpected proof surface metadata: %+v", proof)
	}
}

func TestVerificationStatusAndEvidenceFallbacks(t *testing.T) {
	if got := verificationStatus(nil); got != "" {
		t.Fatalf("verificationStatus(nil) = %q", got)
	}
	state := core.NewContext()
	state.Set("rex.verification", map[string]any{"status": "pass"})
	if got := verificationStatus(state); got != "pass" {
		t.Fatalf("verificationStatus(map) = %q", got)
	}
	state = core.NewContext()
	state.Set("rex.verification", VerificationEvidenceRecord{Status: "pass", EvidencePresent: true})
	if got := verificationStatus(state); got != "pass" {
		t.Fatalf("verificationStatus(struct) = %q", got)
	}
	if got := VerificationEvidence(nil); got.Status != "not_verified" || got.Source != "absent" {
		t.Fatalf("VerificationEvidence(nil) = %+v", got)
	}
	state = core.NewContext()
	if got := VerificationEvidence(state); got.Status != "not_verified" || got.Source != "absent" {
		t.Fatalf("VerificationEvidence(empty) = %+v", got)
	}
}
