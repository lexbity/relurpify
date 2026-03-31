package execution

import (
	"context"
	"database/sql"
	"time"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type RequestRecordView struct {
	ID        string    `json:"id,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Scope     string    `json:"scope,omitempty"`
	Status    string    `json:"status,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type RequestHistoryView struct {
	WorkflowID string              `json:"workflow_id,omitempty"`
	Requests   []RequestRecordView `json:"requests,omitempty"`
	Pending    int                 `json:"pending,omitempty"`
	Running    int                 `json:"running,omitempty"`
	Completed  int                 `json:"completed,omitempty"`
	Failed     int                 `json:"failed,omitempty"`
	Canceled   int                 `json:"canceled,omitempty"`
}

type VersionedPlanView struct {
	ID                     string                   `json:"id,omitempty"`
	WorkflowID             string                   `json:"workflow_id,omitempty"`
	PlanID                 string                   `json:"plan_id,omitempty"`
	Version                int                      `json:"version,omitempty"`
	Status                 string                   `json:"status,omitempty"`
	DerivedFromExploration string                   `json:"derived_from_exploration,omitempty"`
	BasedOnRevision        string                   `json:"based_on_revision,omitempty"`
	SemanticSnapshotRef    string                   `json:"semantic_snapshot_ref,omitempty"`
	PatternRefs            []string                 `json:"pattern_refs,omitempty"`
	AnchorRefs             []string                 `json:"anchor_refs,omitempty"`
	TensionRefs            []string                 `json:"tension_refs,omitempty"`
	Plan                   frameworkplan.LivingPlan `json:"plan"`
}

type ActivePlanView struct {
	WorkflowID   string             `json:"workflow_id,omitempty"`
	ActivePlan   *VersionedPlanView `json:"active_plan,omitempty"`
	ActiveStepID string             `json:"active_step_id,omitempty"`
	Phase        string             `json:"phase,omitempty"`
}

type LearningInteractionView struct {
	ID        string   `json:"id,omitempty"`
	Status    string   `json:"status,omitempty"`
	Blocking  bool     `json:"blocking,omitempty"`
	Prompt    string   `json:"prompt,omitempty"`
	SubjectID string   `json:"subject_id,omitempty"`
	Evidence  []string `json:"evidence,omitempty"`
}

type LearningQueueView struct {
	WorkflowID         string                    `json:"workflow_id,omitempty"`
	PendingLearning    []LearningInteractionView `json:"pending_learning,omitempty"`
	PendingGuidanceIDs []string                  `json:"pending_guidance_ids,omitempty"`
	BlockingLearning   []string                  `json:"blocking_learning,omitempty"`
}

type TensionView struct {
	ID                 string   `json:"id,omitempty"`
	Kind               string   `json:"kind,omitempty"`
	Description        string   `json:"description,omitempty"`
	Severity           string   `json:"severity,omitempty"`
	Status             string   `json:"status,omitempty"`
	PatternIDs         []string `json:"pattern_ids,omitempty"`
	AnchorRefs         []string `json:"anchor_refs,omitempty"`
	SymbolScope        []string `json:"symbol_scope,omitempty"`
	RelatedPlanStepIDs []string `json:"related_plan_step_ids,omitempty"`
	BasedOnRevision    string   `json:"based_on_revision,omitempty"`
}

type TensionSummaryView struct {
	WorkflowID string `json:"workflow_id,omitempty"`
	Total      int    `json:"total,omitempty"`
	Active     int    `json:"active,omitempty"`
	Accepted   int    `json:"accepted,omitempty"`
	Resolved   int    `json:"resolved,omitempty"`
	Unresolved int    `json:"unresolved,omitempty"`
}

type DraftPlanInput struct {
	WorkflowID              string
	DerivedFromExploration  string
	BasedOnRevision         string
	SemanticSnapshotRef     string
	CommentRefs             []string
	TensionRefs             []string
	PatternRefs             []string
	AnchorRefs              []string
	FormationResultRef      string
	FormationProvenanceRefs []string
}

type ArchaeoServiceAccess interface {
	RequestHistory(context.Context, string) (*RequestHistoryView, error)
	ActivePlan(context.Context, string) (*ActivePlanView, error)
	LearningQueue(context.Context, string) (*LearningQueueView, error)
	TensionsByWorkflow(context.Context, string) ([]TensionView, error)
	TensionSummaryByWorkflow(context.Context, string) (*TensionSummaryView, error)
	PlanVersions(context.Context, string) ([]VersionedPlanView, error)
	ActivePlanVersion(context.Context, string) (*VersionedPlanView, error)
	DraftPlanVersion(context.Context, *frameworkplan.LivingPlan, DraftPlanInput) (*VersionedPlanView, error)
	ActivatePlanVersion(context.Context, string, int) (*VersionedPlanView, error)
}

// ServiceBundle carries Euclo-owned runtime dependencies that behaviors may
// need during execution. It is injected at behavior dispatch time so Euclo does
// not need to extend framework-level agent environments with named-agent state.
type ServiceBundle struct {
	Archaeo        ArchaeoServiceAccess
	GraphDB        *graphdb.Engine
	RetrievalDB    *sql.DB
	PlanStore      frameworkplan.PlanStore
	PatternStore   patterns.PatternStore
	CommentStore   patterns.CommentStore
	WorkflowStore  memory.WorkflowStateStore
	GuidanceBroker *guidance.GuidanceBroker
	LearningBroker *archaeolearning.Broker
	DeferralPlan   *guidance.DeferralPlan
}

func (b ServiceBundle) IsZero() bool {
	return b.Archaeo == nil &&
		b.GraphDB == nil &&
		b.RetrievalDB == nil &&
		b.PlanStore == nil &&
		b.PatternStore == nil &&
		b.CommentStore == nil &&
		b.WorkflowStore == nil &&
		b.GuidanceBroker == nil &&
		b.LearningBroker == nil &&
		b.DeferralPlan == nil
}
