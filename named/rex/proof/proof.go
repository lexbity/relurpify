package proof

import (
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/lexcodex/relurpify/named/rex/classify"
	"github.com/lexcodex/relurpify/named/rex/route"
)

// ActionLogEntry is rex's durable action-log view.
type ActionLogEntry struct {
	Kind      string         `json:"kind"`
	Message   string         `json:"message"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ProofSurface summarizes rex execution for audit and completion.
type ProofSurface struct {
	RouteFamily             string   `json:"route_family"`
	Mode                    string   `json:"mode"`
	Profile                 string   `json:"profile"`
	VerificationStatus      string   `json:"verification_status"`
	VerificationSource      string   `json:"verification_source,omitempty"`
	VerificationEvidence    bool     `json:"verification_evidence"`
	SuccessGateReason       string   `json:"success_gate_reason,omitempty"`
	RecoveryCount           int      `json:"recovery_count"`
	ArtifactKinds           []string `json:"artifact_kinds,omitempty"`
	WorkflowRetrieval       bool     `json:"workflow_retrieval"`
	CompletionAllowed       bool     `json:"completion_allowed"`
}

// CompletionDecision records final gate evaluation.
type CompletionDecision struct {
	Allowed bool     `json:"allowed"`
	Reason  string   `json:"reason,omitempty"`
	Details []string `json:"details,omitempty"`
}

// BuildActionLog builds a small deterministic action log for rex.
func BuildActionLog(decision route.RouteDecision, class classify.Classification, state *core.Context) []ActionLogEntry {
	now := time.Now().UTC()
	log := []ActionLogEntry{
		{Kind: "route", Message: "resolved rex route", Timestamp: now, Metadata: map[string]any{"family": decision.Family, "mode": decision.Mode, "profile": decision.Profile}},
		{Kind: "classification", Message: "classified rex task", Timestamp: now, Metadata: map[string]any{"intent": class.Intent, "risk": class.RiskLevel, "read_only": class.ReadOnly}},
	}
	if state != nil {
		if workflowID := strings.TrimSpace(state.GetString("rex.workflow_id")); workflowID != "" {
			log = append(log, ActionLogEntry{Kind: "identity", Message: "resolved rex workflow identity", Timestamp: now, Metadata: map[string]any{"workflow_id": workflowID}})
		}
		if raw, ok := state.Get("euclo.context_expansion"); ok && raw != nil {
			log = append(log, ActionLogEntry{Kind: "retrieval", Message: "expanded rex context", Timestamp: now, Metadata: map[string]any{"payload": raw}})
		}
		if raw, ok := state.Get("pipeline.workflow_retrieval"); ok && raw != nil {
			log = append(log, ActionLogEntry{Kind: "workflow_retrieval", Message: "loaded rex workflow retrieval context", Timestamp: now, Metadata: map[string]any{"payload": raw}})
		}
		if raw, ok := state.Get("euclo.verification"); ok && raw != nil {
			log = append(log, ActionLogEntry{Kind: "verification", Message: "normalized rex verification evidence", Timestamp: now, Metadata: map[string]any{"payload": raw}})
		}
		if raw, ok := state.Get("euclo.success_gate"); ok && raw != nil {
			log = append(log, ActionLogEntry{Kind: "success_gate", Message: "evaluated rex completion gate", Timestamp: now, Metadata: map[string]any{"payload": raw}})
		}
	}
	return log
}

// BuildProofSurface builds the proof surface from route and result state.
func BuildProofSurface(decision route.RouteDecision, result *core.Result, state *core.Context) ProofSurface {
	proof := ProofSurface{
		RouteFamily:      decision.Family,
		Mode:             decision.Mode,
		Profile:          decision.Profile,
		VerificationStatus: verificationStatus(state),
		CompletionAllowed: result == nil || result.Error == nil,
	}
	if state != nil {
		if evidence := VerificationEvidence(state); evidence.EvidencePresent {
			proof.VerificationEvidence = true
			proof.VerificationSource = evidence.Source
		}
		if gate, ok := state.Get("euclo.success_gate"); ok && gate != nil {
			if typed, ok := gate.(eucloruntime.SuccessGateResult); ok {
				proof.SuccessGateReason = typed.Reason
				proof.CompletionAllowed = typed.Allowed && proof.CompletionAllowed
			}
		}
		if attempts, ok := state.Get("rex.recovery_attempts"); ok {
			if count, ok := attempts.(int); ok {
				proof.RecoveryCount = count
			}
		}
		if raw, ok := state.Get("rex.artifact_kinds"); ok {
			switch typed := raw.(type) {
			case []string:
				proof.ArtifactKinds = append([]string{}, typed...)
			}
		}
		if raw, ok := state.Get("pipeline.workflow_retrieval"); ok && raw != nil {
			proof.WorkflowRetrieval = true
		}
	}
	return proof
}

// VerificationEvidence normalizes raw verification state from delegate execution.
func VerificationEvidence(state *core.Context) eucloruntime.VerificationEvidence {
	return eucloruntime.NormalizeVerificationEvidence(state)
}

// EvaluateCompletion applies route-aware verification policy and maps the result to rex completion semantics.
func EvaluateCompletion(decision route.RouteDecision, class classify.Classification, state *core.Context) CompletionDecision {
	evidence := VerificationEvidence(state)
	if !decision.RequireProof {
		gate := eucloruntime.SuccessGateResult{Allowed: true, Reason: "proof_not_required"}
		if state != nil {
			state.Set("euclo.verification_policy", ResolveVerificationPolicy(decision, class))
			state.Set("euclo.verification", evidence)
			state.Set("euclo.success_gate", gate)
			state.Set("rex.verification_status", evidence.Status)
		}
		return CompletionDecision{Allowed: true, Reason: gate.Reason}
	}
	policy := ResolveVerificationPolicy(decision, class)
	if class.ReadOnly {
		if !evidence.EvidencePresent {
			gate := eucloruntime.SuccessGateResult{Allowed: true, Reason: "inspection-only"}
			if state != nil {
				state.Set("euclo.verification_policy", policy)
				state.Set("euclo.verification", evidence)
				state.Set("euclo.success_gate", gate)
				state.Set("rex.verification_status", evidence.Status)
			}
			return CompletionDecision{Allowed: true, Reason: gate.Reason}
		}
	}
	gate := eucloruntime.EvaluateSuccessGate(policy, evidence, nil)
	if state != nil {
		state.Set("euclo.verification_policy", policy)
		state.Set("euclo.verification", evidence)
		state.Set("euclo.success_gate", gate)
		state.Set("rex.verification_status", evidence.Status)
	}
	return CompletionDecision{
		Allowed: gate.Allowed,
		Reason:  gate.Reason,
		Details: append([]string{}, gate.Details...),
	}
}

// ResolveVerificationPolicy maps rex route and classification into the existing verification policy model.
func ResolveVerificationPolicy(decision route.RouteDecision, class classify.Classification) eucloruntime.VerificationPolicy {
	mode := euclotypes.ModeResolution{ModeID: decision.Mode}
	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:            proofProfileID(decision),
		VerificationRequired: decision.RequireProof && !class.ReadOnly,
	}
	return eucloruntime.ResolveVerificationPolicy(mode, profile)
}

func verificationStatus(state *core.Context) string {
	if state == nil {
		return ""
	}
	if raw, ok := state.Get("euclo.verification"); ok && raw != nil {
		if typed, ok := raw.(interface{ GetStatus() string }); ok {
			return typed.GetStatus()
		}
		if payload, ok := raw.(map[string]any); ok {
			if status, ok := payload["status"].(string); ok {
				return strings.TrimSpace(status)
			}
		}
	}
	return strings.TrimSpace(state.GetString("rex.verification_status"))
}

func proofProfileID(decision route.RouteDecision) string {
	switch decision.Family {
	case route.FamilyArchitect:
		return "edit_verify_repair"
	case route.FamilyPipeline:
		return "structured_verification"
	case route.FamilyPlanner:
		return "read_only_review"
	default:
		return fmt.Sprintf("rex/%s", decision.Profile)
	}
}
