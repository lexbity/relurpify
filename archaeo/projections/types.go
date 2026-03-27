package projections

import (
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
)

type ExplorationProjection struct {
	WorkflowID           string                              `json:"workflow_id"`
	ActiveExploration    *archaeodomain.ExplorationSession   `json:"active_exploration,omitempty"`
	ExplorationSnapshots []archaeodomain.ExplorationSnapshot `json:"exploration_snapshots,omitempty"`
	TensionSummary       *archaeodomain.TensionSummary       `json:"tension_summary,omitempty"`
}

type LearningQueueProjection struct {
	WorkflowID         string                        `json:"workflow_id"`
	PendingLearning    []archaeolearning.Interaction `json:"pending_learning,omitempty"`
	PendingGuidanceIDs []string                      `json:"pending_guidance_ids,omitempty"`
	BlockingLearning   []string                      `json:"blocking_learning,omitempty"`
}

type ActivePlanProjection struct {
	WorkflowID        string                             `json:"workflow_id"`
	PhaseState        *archaeodomain.WorkflowPhaseState  `json:"phase_state,omitempty"`
	ActivePlanVersion *archaeodomain.VersionedLivingPlan `json:"active_plan_version,omitempty"`
	ConvergenceState  *archaeodomain.ConvergenceState    `json:"convergence_state,omitempty"`
}

type TimelineProjection struct {
	WorkflowID   string                        `json:"workflow_id"`
	Timeline     []archaeodomain.TimelineEvent `json:"timeline,omitempty"`
	LastEventSeq uint64                        `json:"last_event_seq,omitempty"`
	UpdatedAt    time.Time                     `json:"updated_at"`
}

type MutationHistoryProjection struct {
	WorkflowID      string                        `json:"workflow_id"`
	Mutations       []archaeodomain.MutationEvent `json:"mutations,omitempty"`
	LastMutationAt  *time.Time                    `json:"last_mutation_at,omitempty"`
	BlockingCount   int                           `json:"blocking_count"`
	DispositionByID map[string]string             `json:"disposition_by_id,omitempty"`
}

type RequestHistoryProjection struct {
	WorkflowID string                        `json:"workflow_id"`
	Requests   []archaeodomain.RequestRecord `json:"requests,omitempty"`
	Pending    int                           `json:"pending"`
	Running    int                           `json:"running"`
	Completed  int                           `json:"completed"`
	Failed     int                           `json:"failed"`
	Canceled   int                           `json:"canceled"`
}

type PlanLineageProjection struct {
	WorkflowID       string                              `json:"workflow_id"`
	Versions         []archaeodomain.VersionedLivingPlan `json:"versions,omitempty"`
	ActiveVersion    *archaeodomain.VersionedLivingPlan  `json:"active_version,omitempty"`
	DraftVersions    []archaeodomain.VersionedLivingPlan `json:"draft_versions,omitempty"`
	LatestDraft      *archaeodomain.VersionedLivingPlan  `json:"latest_draft,omitempty"`
	RecomputePending bool                                `json:"recompute_pending"`
}

type ExplorationActivityProjection struct {
	WorkflowID         string                        `json:"workflow_id"`
	ExplorationID      string                        `json:"exploration_id,omitempty"`
	LatestSnapshotID   string                        `json:"latest_snapshot_id,omitempty"`
	ActivityTimeline   []archaeodomain.TimelineEvent `json:"activity_timeline,omitempty"`
	MutationCount      int                           `json:"mutation_count"`
	RequestCount       int                           `json:"request_count"`
	LearningEventCount int                           `json:"learning_event_count"`
}

type LearningOutcomeProvenance struct {
	InteractionID   string                        `json:"interaction_id"`
	SubjectType     string                        `json:"subject_type"`
	SubjectID       string                        `json:"subject_id,omitempty"`
	Status          string                        `json:"status"`
	Blocking        bool                          `json:"blocking,omitempty"`
	BasedOnRevision string                        `json:"based_on_revision,omitempty"`
	CommentRef      string                        `json:"comment_ref,omitempty"`
	Evidence        []archaeolearning.EvidenceRef `json:"evidence,omitempty"`
	MutationIDs     []string                      `json:"mutation_ids,omitempty"`
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

type ProvenanceProjection struct {
	WorkflowID     string                             `json:"workflow_id"`
	Learning       []LearningOutcomeProvenance        `json:"learning,omitempty"`
	Tensions       []TensionProvenance                `json:"tensions,omitempty"`
	PlanVersions   []PlanVersionProvenance            `json:"plan_versions,omitempty"`
	Requests       []archaeodomain.RequestProvenance  `json:"requests,omitempty"`
	Mutations      []archaeodomain.MutationProvenance `json:"mutations,omitempty"`
	LastMutationAt *time.Time                         `json:"last_mutation_at,omitempty"`
	LastRequestAt  *time.Time                         `json:"last_request_at,omitempty"`
}

type CoherenceProjection struct {
	WorkflowID                   string                              `json:"workflow_id"`
	TensionSummary               *archaeodomain.TensionSummary       `json:"tension_summary,omitempty"`
	ActiveTensions               []archaeodomain.Tension             `json:"active_tensions,omitempty"`
	PendingLearning              []archaeolearning.Interaction       `json:"pending_learning,omitempty"`
	ConfidenceAffectingMutations []archaeodomain.MutationEvent       `json:"confidence_affecting_mutations,omitempty"`
	ActivePlanVersion            *archaeodomain.VersionedLivingPlan  `json:"active_plan_version,omitempty"`
	DraftPlanVersions            []archaeodomain.VersionedLivingPlan `json:"draft_plan_versions,omitempty"`
	ConvergenceState             *archaeodomain.ConvergenceState     `json:"convergence_state,omitempty"`
	AcceptedDebt                 int                                 `json:"accepted_debt"`
	BlockingLearningCount        int                                 `json:"blocking_learning_count"`
	BlockingMutationCount        int                                 `json:"blocking_mutation_count"`
}
