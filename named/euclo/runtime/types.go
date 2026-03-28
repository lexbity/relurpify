package runtime

import (
	"time"

	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// TaskEnvelope is the typed Euclo intake shape used to normalize coding
// requests before routing deeper into the runtime.
type TaskEnvelope struct {
	TaskID                string                        `json:"task_id,omitempty"`
	Instruction           string                        `json:"instruction,omitempty"`
	Workspace             string                        `json:"workspace,omitempty"`
	ModeHint              string                        `json:"mode_hint,omitempty"`
	ResumedMode           string                        `json:"resumed_mode,omitempty"`
	ExplicitVerification  string                        `json:"explicit_verification,omitempty"`
	EditPermitted         bool                          `json:"edit_permitted"`
	CapabilitySnapshot    euclotypes.CapabilitySnapshot `json:"capability_snapshot"`
	PreviousArtifactKinds []string                      `json:"previous_artifact_kinds,omitempty"`
	ResolvedMode          string                        `json:"resolved_mode,omitempty"`
	ExecutionProfile      string                        `json:"execution_profile,omitempty"`
}

type TaskClassification struct {
	IntentFamilies                 []string `json:"intent_families,omitempty"`
	RecommendedMode                string   `json:"recommended_mode,omitempty"`
	MixedIntent                    bool     `json:"mixed_intent"`
	EditPermitted                  bool     `json:"edit_permitted"`
	RequiresEvidenceBeforeMutation bool     `json:"requires_evidence_before_mutation"`
	RequiresDeterministicStages    bool     `json:"requires_deterministic_stages"`
	Scope                          string   `json:"scope,omitempty"`
	RiskLevel                      string   `json:"risk_level,omitempty"`
	ReasonCodes                    []string `json:"reason_codes,omitempty"`
}

// Re-export commonly used types from euclotypes for convenience
type (
	ModeResolution            = euclotypes.ModeResolution
	ExecutionProfileSelection = euclotypes.ExecutionProfileSelection
	ModeRegistry              = euclotypes.ModeRegistry
	ExecutionProfileRegistry  = euclotypes.ExecutionProfileRegistry
	Artifact                  = euclotypes.Artifact
)

type RetrievalPolicy struct {
	ModeID            string `json:"mode_id"`
	ProfileID         string `json:"profile_id"`
	LocalPathsFirst   bool   `json:"local_paths_first"`
	WidenToWorkflow   bool   `json:"widen_to_workflow"`
	WidenWhenNoLocal  bool   `json:"widen_when_no_local"`
	WorkflowLimit     int    `json:"workflow_limit"`
	WorkflowMaxTokens int    `json:"workflow_max_tokens"`
	ExpansionStrategy string `json:"expansion_strategy"`
}

type ContextExpansion struct {
	LocalPaths        []string       `json:"local_paths,omitempty"`
	WorkflowRetrieval map[string]any `json:"workflow_retrieval,omitempty"`
	WidenedToWorkflow bool           `json:"widened_to_workflow"`
	ExpansionStrategy string         `json:"expansion_strategy,omitempty"`
	Summary           string         `json:"summary,omitempty"`
}

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

type VerificationEvidence struct {
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

type EditIntent struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Content string `json:"content,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type EditOperationRecord struct {
	Path           string         `json:"path"`
	Action         string         `json:"action"`
	Summary        string         `json:"summary,omitempty"`
	Requested      bool           `json:"requested"`
	ApprovalStatus string         `json:"approval_status,omitempty"`
	Status         string         `json:"status"`
	Tool           string         `json:"tool,omitempty"`
	Error          string         `json:"error,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
}

type EditExecutionRecord struct {
	Requested []EditOperationRecord `json:"requested,omitempty"`
	Approved  []EditOperationRecord `json:"approved,omitempty"`
	Executed  []EditOperationRecord `json:"executed,omitempty"`
	Rejected  []EditOperationRecord `json:"rejected,omitempty"`
	Summary   string                `json:"summary,omitempty"`
}

type PhaseCapabilityRoute struct {
	Phase  string `json:"phase"`
	Family string `json:"family"`
	Agent  string `json:"agent"`
}

type CapabilityFamilyRouting struct {
	ModeID            string                 `json:"mode_id"`
	ProfileID         string                 `json:"profile_id"`
	PrimaryFamilyID   string                 `json:"primary_family_id"`
	FallbackFamilyIDs []string               `json:"fallback_family_ids,omitempty"`
	Routes            []PhaseCapabilityRoute `json:"routes,omitempty"`
}

type ActionLogEntry struct {
	Kind      string         `json:"kind"`
	Message   string         `json:"message"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ProofSurface struct {
	ModeID                string   `json:"mode_id,omitempty"`
	ProfileID             string   `json:"profile_id,omitempty"`
	PrimaryFamilyID       string   `json:"primary_family_id,omitempty"`
	VerificationStatus    string   `json:"verification_status,omitempty"`
	SuccessGateReason     string   `json:"success_gate_reason,omitempty"`
	ArtifactKinds         []string `json:"artifact_kinds,omitempty"`
	WorkflowRetrievalUsed bool     `json:"workflow_retrieval_used"`
	CapabilityIDs         []string `json:"capability_ids,omitempty"`
	GateEvalsCount        int      `json:"gate_evals_count,omitempty"`
	PhasesExecuted        []string `json:"phases_executed,omitempty"`
	RecoveryAttempts      int      `json:"recovery_attempts,omitempty"`
}

// RuntimeSurfaces abstracts access to workflow and runtime stores
type RuntimeSurfaces struct {
	Workflow *db.SQLiteWorkflowStateStore
	Runtime  memory.RuntimeMemoryStore
}
