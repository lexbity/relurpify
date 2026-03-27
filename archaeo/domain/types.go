package domain

import (
	"time"

	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type EucloPhase string

const (
	PhaseArchaeology       EucloPhase = "archaeology"
	PhaseIntentElicitation EucloPhase = "intent_elicitation"
	PhasePlanFormation     EucloPhase = "plan_formation"
	PhaseExecution         EucloPhase = "execution"
	PhaseVerification      EucloPhase = "verification"
	PhaseSurfacing         EucloPhase = "surfacing"
	PhaseBlocked           EucloPhase = "blocked"
	PhaseDeferred          EucloPhase = "deferred"
	PhaseCompleted         EucloPhase = "completed"
)

type WorkflowPhaseState struct {
	WorkflowID          string     `json:"workflow_id"`
	CurrentPhase        EucloPhase `json:"current_phase"`
	EnteredAt           time.Time  `json:"entered_at"`
	LastTransitionAt    time.Time  `json:"last_transition_at"`
	ActiveExplorationID string     `json:"active_exploration_id,omitempty"`
	ActivePlanID        string     `json:"active_plan_id,omitempty"`
	ActivePlanVersion   *int       `json:"active_plan_version,omitempty"`
	BlockedReason       string     `json:"blocked_reason,omitempty"`
	PendingGuidance     []string   `json:"pending_guidance,omitempty"`
	PendingLearning     []string   `json:"pending_learning,omitempty"`
	RecomputeRequired   bool       `json:"recompute_required,omitempty"`
	BasedOnRevision     string     `json:"based_on_revision,omitempty"`
}

type PhaseTransition struct {
	To                  EucloPhase
	At                  time.Time
	Reason              string
	ActiveExplorationID string
	ActivePlanID        string
	ActivePlanVersion   *int
	BlockedReason       string
	PendingGuidance     []string
	PendingLearning     []string
	RecomputeRequired   *bool
	BasedOnRevision     string
}

type ExplorationStatus string

const (
	ExplorationStatusActive     ExplorationStatus = "active"
	ExplorationStatusStale      ExplorationStatus = "stale"
	ExplorationStatusArchived   ExplorationStatus = "archived"
	ExplorationStatusSuperseded ExplorationStatus = "superseded"
)

type ExplorationSession struct {
	ID                string            `json:"id"`
	WorkspaceID       string            `json:"workspace_id"`
	Status            ExplorationStatus `json:"status"`
	LatestSnapshotID  string            `json:"latest_snapshot_id,omitempty"`
	SnapshotIDs       []string          `json:"snapshot_ids,omitempty"`
	BasedOnRevision   string            `json:"based_on_revision,omitempty"`
	RecomputeRequired bool              `json:"recompute_required,omitempty"`
	StaleReason       string            `json:"stale_reason,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type ExplorationSnapshot struct {
	ID                   string    `json:"id"`
	ExplorationID        string    `json:"exploration_id"`
	WorkspaceID          string    `json:"workspace_id"`
	WorkflowID           string    `json:"workflow_id"`
	SnapshotKey          string    `json:"snapshot_key"`
	BasedOnRevision      string    `json:"based_on_revision,omitempty"`
	SemanticSnapshotRef  string    `json:"semantic_snapshot_ref,omitempty"`
	RecomputeRequired    bool      `json:"recompute_required,omitempty"`
	StaleReason          string    `json:"stale_reason,omitempty"`
	CandidatePatternRefs []string  `json:"candidate_pattern_refs,omitempty"`
	CandidateAnchorRefs  []string  `json:"candidate_anchor_refs,omitempty"`
	TensionIDs           []string  `json:"tension_ids,omitempty"`
	OpenLearningIDs      []string  `json:"open_learning_ids,omitempty"`
	Summary              string    `json:"summary,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type LivingPlanVersionStatus string

const (
	LivingPlanVersionDraft      LivingPlanVersionStatus = "draft"
	LivingPlanVersionActive     LivingPlanVersionStatus = "active"
	LivingPlanVersionSuperseded LivingPlanVersionStatus = "superseded"
	LivingPlanVersionArchived   LivingPlanVersionStatus = "archived"
)

type VersionedLivingPlan struct {
	ID                      string                   `json:"id"`
	WorkflowID              string                   `json:"workflow_id"`
	Version                 int                      `json:"version"`
	ParentVersion           *int                     `json:"parent_version,omitempty"`
	DerivedFromExploration  string                   `json:"derived_from_exploration,omitempty"`
	BasedOnRevision         string                   `json:"based_on_revision,omitempty"`
	SemanticSnapshotRef     string                   `json:"semantic_snapshot_ref,omitempty"`
	Status                  LivingPlanVersionStatus  `json:"status"`
	RecomputeRequired       bool                     `json:"recompute_required,omitempty"`
	StaleReason             string                   `json:"stale_reason,omitempty"`
	ComputedAt              time.Time                `json:"computed_at"`
	ActivatedAt             *time.Time               `json:"activated_at,omitempty"`
	SupersededAt            *time.Time               `json:"superseded_at,omitempty"`
	Plan                    frameworkplan.LivingPlan `json:"plan"`
	CommentRefs             []string                 `json:"comment_refs,omitempty"`
	TensionRefs             []string                 `json:"tension_refs,omitempty"`
	PatternRefs             []string                 `json:"pattern_refs,omitempty"`
	AnchorRefs              []string                 `json:"anchor_refs,omitempty"`
	FormationResultRef      string                   `json:"formation_result_ref,omitempty"`
	FormationProvenanceRefs []string                 `json:"formation_provenance_refs,omitempty"`
	CreatedAt               time.Time                `json:"created_at"`
	UpdatedAt               time.Time                `json:"updated_at"`
}

type ExecutionHandoff struct {
	ID                  string    `json:"id"`
	WorkflowID          string    `json:"workflow_id"`
	ExplorationID       string    `json:"exploration_id,omitempty"`
	PlanID              string    `json:"plan_id"`
	PlanVersion         int       `json:"plan_version"`
	StepID              string    `json:"step_id,omitempty"`
	BasedOnRevision     string    `json:"based_on_revision,omitempty"`
	SemanticSnapshotRef string    `json:"semantic_snapshot_ref,omitempty"`
	HandoffAccepted     bool      `json:"handoff_accepted"`
	HandoffRef          string    `json:"handoff_ref"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type TensionStatus string

const (
	TensionInferred   TensionStatus = "inferred"
	TensionConfirmed  TensionStatus = "confirmed"
	TensionAccepted   TensionStatus = "accepted"
	TensionUnresolved TensionStatus = "unresolved"
	TensionResolved   TensionStatus = "resolved"
)

type Tension struct {
	ID                 string        `json:"id"`
	WorkflowID         string        `json:"workflow_id"`
	ExplorationID      string        `json:"exploration_id,omitempty"`
	SnapshotID         string        `json:"snapshot_id,omitempty"`
	SourceRef          string        `json:"source_ref,omitempty"`
	PatternIDs         []string      `json:"pattern_ids,omitempty"`
	AnchorRefs         []string      `json:"anchor_refs,omitempty"`
	SymbolScope        []string      `json:"symbol_scope,omitempty"`
	Kind               string        `json:"kind"`
	Description        string        `json:"description"`
	Severity           string        `json:"severity,omitempty"`
	Status             TensionStatus `json:"status"`
	BlastRadiusNodeIDs []string      `json:"blast_radius_node_ids,omitempty"`
	RelatedPlanStepIDs []string      `json:"related_plan_step_ids,omitempty"`
	CommentRefs        []string      `json:"comment_refs,omitempty"`
	BasedOnRevision    string        `json:"based_on_revision,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

type TensionSummary struct {
	WorkflowID      string         `json:"workflow_id,omitempty"`
	ExplorationID   string         `json:"exploration_id,omitempty"`
	Total           int            `json:"total"`
	Active          int            `json:"active"`
	Accepted        int            `json:"accepted"`
	Resolved        int            `json:"resolved"`
	Unresolved      int            `json:"unresolved"`
	BySeverity      map[string]int `json:"by_severity,omitempty"`
	ByKind          map[string]int `json:"by_kind,omitempty"`
	BlockingCount   int            `json:"blocking_count"`
	AcceptedDebt    int            `json:"accepted_debt"`
	LatestUpdatedAt *time.Time     `json:"latest_updated_at,omitempty"`
}

type ConvergenceStatus string

const (
	ConvergenceStatusUnknown  ConvergenceStatus = "unknown"
	ConvergenceStatusVerified ConvergenceStatus = "verified"
	ConvergenceStatusFailed   ConvergenceStatus = "failed"
)

type ConvergenceState struct {
	WorkflowID           string            `json:"workflow_id"`
	PlanID               string            `json:"plan_id,omitempty"`
	PlanVersion          *int              `json:"plan_version,omitempty"`
	Status               ConvergenceStatus `json:"status"`
	Description          string            `json:"description,omitempty"`
	UnresolvedTensionIDs []string          `json:"unresolved_tension_ids,omitempty"`
	BasedOnRevision      string            `json:"based_on_revision,omitempty"`
	SemanticSnapshotRef  string            `json:"semantic_snapshot_ref,omitempty"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

type MutationCategory string

const (
	MutationObservation      MutationCategory = "observation"
	MutationConfidenceChange MutationCategory = "confidence_change"
	MutationStepInvalidation MutationCategory = "step_invalidation"
	MutationPlanStaleness    MutationCategory = "plan_staleness"
	MutationBlockingSemantic MutationCategory = "blocking_semantic"
)

type BlastRadiusScope string

const (
	BlastRadiusLocal     BlastRadiusScope = "local"
	BlastRadiusStep      BlastRadiusScope = "step"
	BlastRadiusPlan      BlastRadiusScope = "plan"
	BlastRadiusWorkflow  BlastRadiusScope = "workflow"
	BlastRadiusWorkspace BlastRadiusScope = "workspace"
)

type BlastRadius struct {
	Scope              BlastRadiusScope `json:"scope"`
	AffectedStepIDs    []string         `json:"affected_step_ids,omitempty"`
	AffectedSymbolIDs  []string         `json:"affected_symbol_ids,omitempty"`
	AffectedPatternIDs []string         `json:"affected_pattern_ids,omitempty"`
	AffectedAnchorRefs []string         `json:"affected_anchor_refs,omitempty"`
	AffectedNodeIDs    []string         `json:"affected_node_ids,omitempty"`
	EstimatedCount     int              `json:"estimated_count,omitempty"`
}

type MutationImpact string

const (
	ImpactInformational         MutationImpact = "informational"
	ImpactAdvisory              MutationImpact = "advisory"
	ImpactCaution               MutationImpact = "caution"
	ImpactLocalBlocking         MutationImpact = "local_blocking"
	ImpactHandoffInvalidating   MutationImpact = "handoff_invalidating"
	ImpactPlanRecomputeRequired MutationImpact = "plan_recompute_required"
)

type ExecutionDisposition string

const (
	DispositionContinue            ExecutionDisposition = "continue"
	DispositionContinueOnStalePlan ExecutionDisposition = "continue_on_stale_plan"
	DispositionPauseForLearning    ExecutionDisposition = "pause_for_learning"
	DispositionPauseForGuidance    ExecutionDisposition = "pause_for_guidance"
	DispositionInvalidateStep      ExecutionDisposition = "invalidate_step"
	DispositionBlockExecution      ExecutionDisposition = "block_execution"
	DispositionRequireReplan       ExecutionDisposition = "require_replan"
)

type MutationCheckpoint string

const (
	MutationCheckpointPreExecution    MutationCheckpoint = "pre_execution"
	MutationCheckpointPreDispatch     MutationCheckpoint = "pre_dispatch"
	MutationCheckpointPostExecution   MutationCheckpoint = "post_execution"
	MutationCheckpointPreVerification MutationCheckpoint = "pre_verification"
	MutationCheckpointPreFinalization MutationCheckpoint = "pre_finalization"
)

type MutationCheckpointSummary struct {
	Checkpoint      MutationCheckpoint   `json:"checkpoint"`
	WorkflowID      string               `json:"workflow_id,omitempty"`
	HandoffRef      string               `json:"handoff_ref,omitempty"`
	ActiveStepID    string               `json:"active_step_id,omitempty"`
	Disposition     ExecutionDisposition `json:"disposition"`
	HighestImpact   MutationImpact       `json:"highest_impact"`
	Blocking        bool                 `json:"blocking,omitempty"`
	RequireReplan   bool                 `json:"require_replan,omitempty"`
	ContinueOnStale bool                 `json:"continue_on_stale,omitempty"`
	MutationIDs     []string             `json:"mutation_ids,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
}

type MutationEvent struct {
	ID                  string               `json:"id"`
	WorkflowID          string               `json:"workflow_id"`
	ExplorationID       string               `json:"exploration_id,omitempty"`
	PlanID              string               `json:"plan_id,omitempty"`
	PlanVersion         *int                 `json:"plan_version,omitempty"`
	StepID              string               `json:"step_id,omitempty"`
	Category            MutationCategory     `json:"category"`
	SourceKind          string               `json:"source_kind,omitempty"`
	SourceRef           string               `json:"source_ref,omitempty"`
	Description         string               `json:"description,omitempty"`
	BlastRadius         BlastRadius          `json:"blast_radius"`
	Impact              MutationImpact       `json:"impact"`
	Disposition         ExecutionDisposition `json:"disposition"`
	Blocking            bool                 `json:"blocking,omitempty"`
	BasedOnRevision     string               `json:"based_on_revision,omitempty"`
	SemanticSnapshotRef string               `json:"semantic_snapshot_ref,omitempty"`
	Metadata            map[string]any       `json:"metadata,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
}

type TimelineEvent struct {
	Seq        uint64         `json:"seq"`
	EventID    string         `json:"event_id"`
	WorkflowID string         `json:"workflow_id"`
	RunID      string         `json:"run_id,omitempty"`
	StepID     string         `json:"step_id,omitempty"`
	EventType  string         `json:"event_type"`
	Message    string         `json:"message,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type RequestKind string

const (
	RequestExplorationRefresh  RequestKind = "exploration_refresh"
	RequestPatternSurfacing    RequestKind = "pattern_surfacing"
	RequestTensionAnalysis     RequestKind = "tension_analysis"
	RequestProspectiveAnalysis RequestKind = "prospective_analysis"
	RequestConvergenceReview   RequestKind = "convergence_review"
	RequestPlanReformation     RequestKind = "plan_reformation"
)

type RequestStatus string

const (
	RequestStatusPending     RequestStatus = "pending"
	RequestStatusDispatched  RequestStatus = "dispatched"
	RequestStatusRunning     RequestStatus = "running"
	RequestStatusCompleted   RequestStatus = "completed"
	RequestStatusFailed      RequestStatus = "failed"
	RequestStatusCanceled    RequestStatus = "canceled"
	RequestStatusInvalidated RequestStatus = "invalidated"
	RequestStatusSuperseded  RequestStatus = "superseded"
)

type RequestValidity string

const (
	RequestValidityValid       RequestValidity = "valid"
	RequestValidityPartial     RequestValidity = "partial"
	RequestValidityInvalidated RequestValidity = "invalidated"
	RequestValiditySuperseded  RequestValidity = "superseded"
)

type RequestResult struct {
	Kind     string         `json:"kind"`
	RefID    string         `json:"ref_id,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type RequestInvalidation struct {
	Validity          RequestValidity `json:"validity"`
	Reason            string          `json:"reason,omitempty"`
	SupersededBy      string          `json:"superseded_by,omitempty"`
	ConflictingRefIDs []string        `json:"conflicting_ref_ids,omitempty"`
	At                time.Time       `json:"at"`
}

type RequestFulfillment struct {
	Kind           string          `json:"kind"`
	RefID          string          `json:"ref_id,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	Metadata       map[string]any  `json:"metadata,omitempty"`
	Validity       RequestValidity `json:"validity"`
	Applied        bool            `json:"applied"`
	AppliedAt      *time.Time      `json:"applied_at,omitempty"`
	RejectedReason string          `json:"rejected_reason,omitempty"`
}

type RequestRecord struct {
	ID                  string              `json:"id"`
	WorkflowID          string              `json:"workflow_id"`
	ExplorationID       string              `json:"exploration_id,omitempty"`
	SnapshotID          string              `json:"snapshot_id,omitempty"`
	PlanID              string              `json:"plan_id,omitempty"`
	PlanVersion         *int                `json:"plan_version,omitempty"`
	Kind                RequestKind         `json:"kind"`
	Status              RequestStatus       `json:"status"`
	Title               string              `json:"title"`
	Description         string              `json:"description,omitempty"`
	RequestedBy         string              `json:"requested_by,omitempty"`
	CorrelationID       string              `json:"correlation_id,omitempty"`
	IdempotencyKey      string              `json:"idempotency_key,omitempty"`
	SubjectRefs         []string            `json:"subject_refs,omitempty"`
	Input               map[string]any      `json:"input,omitempty"`
	DispatchMetadata    map[string]any      `json:"dispatch_metadata,omitempty"`
	Result              *RequestResult      `json:"result,omitempty"`
	Fulfillment         *RequestFulfillment `json:"fulfillment,omitempty"`
	FulfillmentRef      string              `json:"fulfillment_ref,omitempty"`
	ErrorText           string              `json:"error_text,omitempty"`
	RetryCount          int                 `json:"retry_count,omitempty"`
	Attempt             int                 `json:"attempt,omitempty"`
	ClaimedBy           string              `json:"claimed_by,omitempty"`
	ClaimedAt           *time.Time          `json:"claimed_at,omitempty"`
	LeaseExpiresAt      *time.Time          `json:"lease_expires_at,omitempty"`
	SupersedesRequestID string              `json:"supersedes_request_id,omitempty"`
	InvalidatedAt       *time.Time          `json:"invalidated_at,omitempty"`
	InvalidationReason  string              `json:"invalidation_reason,omitempty"`
	BasedOnRevision     string              `json:"based_on_revision,omitempty"`
	RequestedAt         time.Time           `json:"requested_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	StartedAt           *time.Time          `json:"started_at,omitempty"`
	CompletedAt         *time.Time          `json:"completed_at,omitempty"`
}

type RequestProvenance struct {
	RequestID           string          `json:"request_id"`
	Kind                RequestKind     `json:"kind"`
	Status              RequestStatus   `json:"status"`
	CorrelationID       string          `json:"correlation_id,omitempty"`
	IdempotencyKey      string          `json:"idempotency_key,omitempty"`
	RequestedBy         string          `json:"requested_by,omitempty"`
	ExplorationID       string          `json:"exploration_id,omitempty"`
	SnapshotID          string          `json:"snapshot_id,omitempty"`
	PlanID              string          `json:"plan_id,omitempty"`
	PlanVersion         *int            `json:"plan_version,omitempty"`
	BasedOnRevision     string          `json:"based_on_revision,omitempty"`
	SubjectRefs         []string        `json:"subject_refs,omitempty"`
	FulfillmentRef      string          `json:"fulfillment_ref,omitempty"`
	FulfillmentValidity RequestValidity `json:"fulfillment_validity,omitempty"`
	SupersedesRequestID string          `json:"supersedes_request_id,omitempty"`
	InvalidationReason  string          `json:"invalidation_reason,omitempty"`
	RequestedAt         time.Time       `json:"requested_at"`
	CompletedAt         *time.Time      `json:"completed_at,omitempty"`
}

type MutationProvenance struct {
	MutationID          string               `json:"mutation_id"`
	Category            MutationCategory     `json:"category"`
	Impact              MutationImpact       `json:"impact"`
	Disposition         ExecutionDisposition `json:"disposition"`
	Blocking            bool                 `json:"blocking,omitempty"`
	SourceKind          string               `json:"source_kind,omitempty"`
	SourceRef           string               `json:"source_ref,omitempty"`
	BasedOnRevision     string               `json:"based_on_revision,omitempty"`
	SemanticSnapshotRef string               `json:"semantic_snapshot_ref,omitempty"`
	Description         string               `json:"description,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
}

type FormationCandidate struct {
	ID                 string   `json:"id"`
	Summary            string   `json:"summary,omitempty"`
	StepIDs            []string `json:"step_ids,omitempty"`
	PatternRefs        []string `json:"pattern_refs,omitempty"`
	AnchorRefs         []string `json:"anchor_refs,omitempty"`
	TensionRefs        []string `json:"tension_refs,omitempty"`
	PendingLearningIDs []string `json:"pending_learning_ids,omitempty"`
	ProvenanceRefs     []string `json:"provenance_refs,omitempty"`
}

type FormationResult struct {
	ID                   string               `json:"id"`
	WorkflowID           string               `json:"workflow_id"`
	ExplorationID        string               `json:"exploration_id,omitempty"`
	SnapshotID           string               `json:"snapshot_id,omitempty"`
	PlanID               string               `json:"plan_id,omitempty"`
	PlanVersion          *int                 `json:"plan_version,omitempty"`
	InputSummary         map[string]any       `json:"input_summary,omitempty"`
	ChosenCandidate      FormationCandidate   `json:"chosen_candidate"`
	Alternatives         []FormationCandidate `json:"alternatives,omitempty"`
	Rationale            string               `json:"rationale,omitempty"`
	UnresolvedTensionIDs []string             `json:"unresolved_tension_ids,omitempty"`
	DeferredUncertainty  []string             `json:"deferred_uncertainty,omitempty"`
	ProvenanceRefs       []string             `json:"provenance_refs,omitempty"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
}

type ProvenanceRecord struct {
	WorkflowID     string                      `json:"workflow_id"`
	Learning       []LearningOutcomeProvenance `json:"learning,omitempty"`
	Tensions       []TensionProvenance         `json:"tensions,omitempty"`
	PlanVersions   []PlanVersionProvenance     `json:"plan_versions,omitempty"`
	Requests       []RequestProvenance         `json:"requests,omitempty"`
	Mutations      []MutationProvenance        `json:"mutations,omitempty"`
	LastMutationAt *time.Time                  `json:"last_mutation_at,omitempty"`
	LastRequestAt  *time.Time                  `json:"last_request_at,omitempty"`
}

type LearningOutcomeProvenance struct {
	InteractionID   string   `json:"interaction_id"`
	SubjectType     string   `json:"subject_type"`
	SubjectID       string   `json:"subject_id,omitempty"`
	Status          string   `json:"status"`
	Blocking        bool     `json:"blocking,omitempty"`
	BasedOnRevision string   `json:"based_on_revision,omitempty"`
	CommentRef      string   `json:"comment_ref,omitempty"`
	EvidenceRefs    []string `json:"evidence_refs,omitempty"`
	MutationIDs     []string `json:"mutation_ids,omitempty"`
}

type TensionProvenance struct {
	TensionID          string   `json:"tension_id"`
	Status             string   `json:"status"`
	Description        string   `json:"description,omitempty"`
	CommentRefs        []string `json:"comment_refs,omitempty"`
	PatternIDs         []string `json:"pattern_ids,omitempty"`
	AnchorRefs         []string `json:"anchor_refs,omitempty"`
	RelatedPlanStepIDs []string `json:"related_plan_step_ids,omitempty"`
	BasedOnRevision    string   `json:"based_on_revision,omitempty"`
	MutationIDs        []string `json:"mutation_ids,omitempty"`
}

type PlanVersionProvenance struct {
	PlanID                  string   `json:"plan_id"`
	Version                 int      `json:"version"`
	ParentVersion           *int     `json:"parent_version,omitempty"`
	DerivedFromExploration  string   `json:"derived_from_exploration,omitempty"`
	BasedOnRevision         string   `json:"based_on_revision,omitempty"`
	SemanticSnapshotRef     string   `json:"semantic_snapshot_ref,omitempty"`
	CommentRefs             []string `json:"comment_refs,omitempty"`
	PatternRefs             []string `json:"pattern_refs,omitempty"`
	AnchorRefs              []string `json:"anchor_refs,omitempty"`
	TensionRefs             []string `json:"tension_refs,omitempty"`
	MutationIDs             []string `json:"mutation_ids,omitempty"`
	FormationResultRef      string   `json:"formation_result_ref,omitempty"`
	FormationProvenanceRefs []string `json:"formation_provenance_refs,omitempty"`
}
