package plan

import "time"

type ChangeKind string

const (
	ChangeKindFeature    ChangeKind = "feature"
	ChangeKindStructural ChangeKind = "structural"
	ChangeKindBoth       ChangeKind = "both"
)

type ReadinessGap struct {
	Description  string   `json:"description"`
	Scope        []string `json:"scope"`
	LinkedStepID string   `json:"linked_step_id,omitempty"`
}

type FoldInManifest struct {
	ManifestID          string         `json:"manifest_id"`
	PlanID              string         `json:"plan_id"`
	FeatureDescription  string         `json:"feature_description"`
	StructuralReadiness []ReadinessGap `json:"structural_readiness,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}
