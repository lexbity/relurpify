package guidance

import (
	"fmt"
	"sync"
	"time"
)

type EngineeringObservation struct {
	ID           string         `json:"id"`
	Source       string         `json:"source"`
	GuidanceKind GuidanceKind   `json:"guidance_kind"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	Questions    []string       `json:"questions,omitempty"`
	Evidence     map[string]any `json:"evidence,omitempty"`
	BlastRadius  int            `json:"blast_radius"`
	CreatedAt    time.Time      `json:"created_at"`
	Resolved     bool           `json:"resolved"`
}

type DeferralPlan struct {
	ID           string                   `json:"id"`
	WorkflowID   string                   `json:"workflow_id"`
	Observations []EngineeringObservation `json:"observations"`
	mu           sync.Mutex
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (dp *DeferralPlan) AddObservation(obs EngineeringObservation) {
	if dp == nil {
		return
	}
	dp.mu.Lock()
	defer dp.mu.Unlock()
	now := time.Now().UTC()
	if dp.CreatedAt.IsZero() {
		dp.CreatedAt = now
	}
	if obs.ID == "" {
		obs.ID = fmt.Sprintf("obs-%d", now.UnixNano())
	}
	if obs.CreatedAt.IsZero() {
		obs.CreatedAt = now
	}
	dp.Observations = append(dp.Observations, cloneObservation(obs))
	dp.UpdatedAt = now
}

func (dp *DeferralPlan) ResolveObservation(id string) {
	if dp == nil || id == "" {
		return
	}
	dp.mu.Lock()
	defer dp.mu.Unlock()
	for i := range dp.Observations {
		if dp.Observations[i].ID != id {
			continue
		}
		dp.Observations[i].Resolved = true
		dp.UpdatedAt = time.Now().UTC()
		return
	}
}

func (dp *DeferralPlan) PendingObservations() []EngineeringObservation {
	if dp == nil {
		return nil
	}
	dp.mu.Lock()
	defer dp.mu.Unlock()
	out := make([]EngineeringObservation, 0, len(dp.Observations))
	for _, obs := range dp.Observations {
		if obs.Resolved {
			continue
		}
		out = append(out, cloneObservation(obs))
	}
	return out
}

func (dp *DeferralPlan) IsEmpty() bool {
	return len(dp.PendingObservations()) == 0
}

type DeferralPolicy struct {
	MaxBlastRadiusForDefer int
	DeferrableKinds        []GuidanceKind
}

func DefaultDeferralPolicy() DeferralPolicy {
	return DeferralPolicy{
		MaxBlastRadiusForDefer: 5,
		DeferrableKinds: []GuidanceKind{
			GuidanceAmbiguity,
			GuidanceConfidence,
			GuidanceApproach,
		},
	}
}

func cloneObservation(obs EngineeringObservation) EngineeringObservation {
	out := obs
	if len(obs.Questions) > 0 {
		out.Questions = append([]string(nil), obs.Questions...)
	}
	if obs.Evidence != nil {
		out.Evidence = make(map[string]any, len(obs.Evidence))
		for k, v := range obs.Evidence {
			out.Evidence[k] = v
		}
	}
	return out
}
