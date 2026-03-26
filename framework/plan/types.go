package plan

import "time"

type PlanStepStatus string

const (
	PlanStepPending     PlanStepStatus = "pending"
	PlanStepInProgress  PlanStepStatus = "in_progress"
	PlanStepCompleted   PlanStepStatus = "completed"
	PlanStepFailed      PlanStepStatus = "failed"
	PlanStepInvalidated PlanStepStatus = "invalidated"
	PlanStepSkipped     PlanStepStatus = "skipped"
)

type EvidenceGate struct {
	RequiredAnchors []string `json:"required_anchors,omitempty"`
	MaxTotalLoss    float64  `json:"max_total_loss,omitempty"`
	RequiredSymbols []string `json:"required_symbols,omitempty"`
}

type InvalidationKind string

const (
	InvalidationSymbolChanged    InvalidationKind = "symbol_changed"
	InvalidationAnchorDrifted    InvalidationKind = "anchor_drifted"
	InvalidationDependencyFailed InvalidationKind = "dependency_failed"
)

type InvalidationRule struct {
	Kind   InvalidationKind `json:"kind"`
	Target string           `json:"target"`
}

type StepAttempt struct {
	AttemptedAt   time.Time `json:"attempted_at"`
	Outcome       string    `json:"outcome"`
	FailureReason string    `json:"failure_reason,omitempty"`
	GitCheckpoint string    `json:"git_checkpoint,omitempty"`
}

type PlanStep struct {
	ID                 string             `json:"id"`
	Description        string             `json:"description"`
	Scope              []string           `json:"scope,omitempty"`
	AnchorDependencies []string           `json:"anchor_dependencies,omitempty"`
	EvidenceGate       *EvidenceGate      `json:"evidence_gate,omitempty"`
	ConfidenceScore    float64            `json:"confidence_score"`
	DependsOn          []string           `json:"depends_on,omitempty"`
	InvalidatedBy      []InvalidationRule `json:"invalidated_by,omitempty"`
	History            []StepAttempt      `json:"history,omitempty"`
	Status             PlanStepStatus     `json:"status"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

type ConvergenceTarget struct {
	PatternIDs []string   `json:"pattern_ids,omitempty"`
	TensionIDs []string   `json:"tension_ids,omitempty"`
	Commentary string     `json:"commentary,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}

type LivingPlan struct {
	ID                string               `json:"id"`
	WorkflowID        string               `json:"workflow_id"`
	Title             string               `json:"title"`
	Steps             map[string]*PlanStep `json:"steps"`
	StepOrder         []string             `json:"step_order"`
	ConvergenceTarget *ConvergenceTarget   `json:"convergence_target,omitempty"`
	Version           int                  `json:"version"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}
