package proof

import (
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/rex/classify"
	"github.com/lexcodex/relurpify/named/rex/route"
)

type VerificationPolicy struct {
	PolicyID              string   `json:"policy_id"`
	ModeID                string   `json:"mode_id"`
	ProfileID             string   `json:"profile_id"`
	RequiresVerification  bool     `json:"requires_verification"`
	AcceptedStatuses      []string `json:"accepted_statuses,omitempty"`
	RequiresExecutedCheck bool     `json:"requires_executed_check"`
	ManualOutcomeAllowed  bool     `json:"manual_outcome_allowed"`
}

type VerificationCheckRecord struct {
	Name    string `json:"name,omitempty"`
	Command string `json:"command,omitempty"`
	Status  string `json:"status,omitempty"`
	Details string `json:"details,omitempty"`
}

type VerificationEvidenceRecord struct {
	Status          string                    `json:"status"`
	Summary         string                    `json:"summary,omitempty"`
	Checks          []VerificationCheckRecord `json:"checks,omitempty"`
	Source          string                    `json:"source,omitempty"`
	EvidencePresent bool                      `json:"evidence_present"`
}

type SuccessGateResult struct {
	Allowed bool     `json:"allowed"`
	Reason  string   `json:"reason,omitempty"`
	Details []string `json:"details,omitempty"`
}

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
		if raw, ok := state.Get("rex.context_expansion"); ok && raw != nil {
			log = append(log, ActionLogEntry{Kind: "retrieval", Message: "expanded rex context", Timestamp: now, Metadata: map[string]any{"payload": raw}})
		}
		if raw, ok := state.Get("pipeline.workflow_retrieval"); ok && raw != nil {
			log = append(log, ActionLogEntry{Kind: "workflow_retrieval", Message: "loaded rex workflow retrieval context", Timestamp: now, Metadata: map[string]any{"payload": raw}})
		}
		if raw, ok := state.Get("rex.verification"); ok && raw != nil {
			log = append(log, ActionLogEntry{Kind: "verification", Message: "normalized rex verification evidence", Timestamp: now, Metadata: map[string]any{"payload": raw}})
		}
		if raw, ok := state.Get("rex.success_gate"); ok && raw != nil {
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
		if gate, ok := state.Get("rex.success_gate"); ok && gate != nil {
			if typed, ok := gate.(SuccessGateResult); ok {
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
func VerificationEvidence(state *core.Context) VerificationEvidenceRecord {
	if state == nil {
		return VerificationEvidenceRecord{Status: "not_verified", Source: "absent"}
	}
	if raw, ok := state.Get("pipeline.verify"); ok && raw != nil {
		evidence := verificationEvidenceFromRaw(raw)
		if evidence.Status != "" {
			return evidence
		}
	}
	if summary := strings.TrimSpace(state.GetString("react.verification_latched_summary")); summary != "" {
		return VerificationEvidenceRecord{
			Status:          "pass",
			Summary:         summary,
			Source:          "react.verification_latched_summary",
			EvidencePresent: true,
		}
	}
	return VerificationEvidenceRecord{Status: "not_verified", Source: "absent"}
}

// EvaluateCompletion applies route-aware verification policy and maps the result to rex completion semantics.
func EvaluateCompletion(decision route.RouteDecision, class classify.Classification, state *core.Context) CompletionDecision {
	evidence := VerificationEvidence(state)
	if !decision.RequireProof {
		gate := SuccessGateResult{Allowed: true, Reason: "proof_not_required"}
		if state != nil {
			state.Set("rex.verification_policy", ResolveVerificationPolicy(decision, class))
			state.Set("rex.verification", evidence)
			state.Set("rex.success_gate", gate)
			state.Set("rex.verification_status", evidence.Status)
		}
		return CompletionDecision{Allowed: true, Reason: gate.Reason}
	}
	policy := ResolveVerificationPolicy(decision, class)
	if class.ReadOnly {
		if !evidence.EvidencePresent {
			gate := SuccessGateResult{Allowed: true, Reason: "inspection-only"}
			if state != nil {
				state.Set("rex.verification_policy", policy)
				state.Set("rex.verification", evidence)
				state.Set("rex.success_gate", gate)
				state.Set("rex.verification_status", evidence.Status)
			}
			return CompletionDecision{Allowed: true, Reason: gate.Reason}
		}
	}
	gate := EvaluateSuccessGate(policy, evidence)
	if state != nil {
		state.Set("rex.verification_policy", policy)
		state.Set("rex.verification", evidence)
		state.Set("rex.success_gate", gate)
		state.Set("rex.verification_status", evidence.Status)
	}
	return CompletionDecision{
		Allowed: gate.Allowed,
		Reason:  gate.Reason,
		Details: append([]string{}, gate.Details...),
	}
}

// ResolveVerificationPolicy maps rex route and classification into the existing verification policy model.
func ResolveVerificationPolicy(decision route.RouteDecision, class classify.Classification) VerificationPolicy {
	policy := VerificationPolicy{
		PolicyID:              fmt.Sprintf("%s/%s", decision.Mode, proofProfileID(decision)),
		ModeID:                decision.Mode,
		ProfileID:             proofProfileID(decision),
		RequiresVerification:  decision.RequireProof && !class.ReadOnly,
		AcceptedStatuses:      []string{"pass"},
		RequiresExecutedCheck: decision.RequireProof && !class.ReadOnly,
	}
	switch decision.Mode {
	case "planning", "open":
		policy.RequiresVerification = false
		policy.RequiresExecutedCheck = false
	case "structured", "mutation":
		policy.ManualOutcomeAllowed = false
	}
	return policy
}

func verificationStatus(state *core.Context) string {
	if state == nil {
		return ""
	}
	if raw, ok := state.Get("rex.verification"); ok && raw != nil {
		if payload, ok := raw.(map[string]any); ok {
			if status, ok := payload["status"].(string); ok {
				return strings.TrimSpace(status)
			}
		}
		if typed, ok := raw.(VerificationEvidenceRecord); ok {
			return strings.TrimSpace(typed.Status)
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

func EvaluateSuccessGate(policy VerificationPolicy, evidence VerificationEvidenceRecord) SuccessGateResult {
	if !policy.RequiresVerification {
		return SuccessGateResult{Allowed: true, Reason: "verification_not_required"}
	}
	if !evidence.EvidencePresent {
		return SuccessGateResult{Allowed: false, Reason: "verification_missing", Details: []string{"required verification evidence was not produced"}}
	}
	status := strings.ToLower(strings.TrimSpace(evidence.Status))
	if status == "" {
		status = "not_verified"
	}
	accepted := false
	for _, allowed := range policy.AcceptedStatuses {
		if status == strings.ToLower(strings.TrimSpace(allowed)) {
			accepted = true
			break
		}
	}
	if !accepted {
		if status == "needs_manual_verification" && policy.ManualOutcomeAllowed {
			return SuccessGateResult{Allowed: true, Reason: "manual_verification_allowed"}
		}
		return SuccessGateResult{Allowed: false, Reason: "verification_status_rejected", Details: []string{"status=" + status}}
	}
	if policy.RequiresExecutedCheck {
		executed := false
		for _, check := range evidence.Checks {
			if strings.EqualFold(strings.TrimSpace(check.Status), "pass") {
				executed = true
				break
			}
		}
		if !executed {
			return SuccessGateResult{Allowed: false, Reason: "verification_check_missing", Details: []string{"verification passed without any passing check record"}}
		}
	}
	return SuccessGateResult{Allowed: true, Reason: "verification_accepted"}
}

func verificationEvidenceFromRaw(raw any) VerificationEvidenceRecord {
	switch typed := raw.(type) {
	case map[string]any:
		return verificationEvidenceFromMap(typed)
	default:
		text := strings.TrimSpace(fmt.Sprint(raw))
		if text == "" || text == "<nil>" {
			return VerificationEvidenceRecord{Status: "not_verified", Source: "pipeline.verify"}
		}
		return VerificationEvidenceRecord{Status: "pass", Summary: text, Source: "pipeline.verify", EvidencePresent: true}
	}
}

func verificationEvidenceFromMap(payload map[string]any) VerificationEvidenceRecord {
	evidence := VerificationEvidenceRecord{
		Status:          strings.TrimSpace(fmt.Sprint(payload["status"])),
		Summary:         strings.TrimSpace(fmt.Sprint(payload["summary"])),
		Source:          "pipeline.verify",
		EvidencePresent: true,
	}
	if evidence.Status == "<nil>" {
		evidence.Status = ""
	}
	if evidence.Summary == "<nil>" {
		evidence.Summary = ""
	}
	switch typed := payload["checks"].(type) {
	case []VerificationCheckRecord:
		evidence.Checks = append([]VerificationCheckRecord{}, typed...)
	case []any:
		checks := make([]VerificationCheckRecord, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			checks = append(checks, VerificationCheckRecord{
				Name:    strings.TrimSpace(fmt.Sprint(entry["name"])),
				Command: strings.TrimSpace(fmt.Sprint(entry["command"])),
				Status:  strings.TrimSpace(fmt.Sprint(entry["status"])),
				Details: strings.TrimSpace(fmt.Sprint(entry["details"])),
			})
		}
		evidence.Checks = checks
	}
	if evidence.Status == "" {
		evidence.Status = "not_verified"
		evidence.EvidencePresent = false
	}
	return evidence
}
