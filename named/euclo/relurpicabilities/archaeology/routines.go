package archaeology

import (
	"context"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

type patternSurfaceRoutine struct{}
type prospectiveAssessRoutine struct{}
type convergenceGuardRoutine struct{}
type coherenceAssessRoutine struct{}
type scopeExpandRoutine struct{}

func NewSupportingRoutines() []euclorelurpic.SupportingRoutine {
	return []euclorelurpic.SupportingRoutine{
		patternSurfaceRoutine{},
		prospectiveAssessRoutine{},
		convergenceGuardRoutine{},
		coherenceAssessRoutine{},
		scopeExpandRoutine{},
	}
}

func (patternSurfaceRoutine) ID() string { return PatternSurface }

func (patternSurfaceRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"pattern_refs": append([]string(nil), in.Work.PatternRefs...),
		"summary":      "pattern-surface routine grounded archaeology exploration in surfaced codebase patterns",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_pattern_surface",
		Kind:       euclotypes.ArtifactKindExplore,
		Summary:    "pattern-surface routine grounded archaeology exploration in surfaced codebase patterns",
		Payload:    payload,
		ProducerID: PatternSurface,
		Status:     "produced",
	}}, nil
}

func (prospectiveAssessRoutine) ID() string { return ProspectiveAssess }

func (prospectiveAssessRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"prospective_refs": append([]string(nil), in.Work.ProspectiveRefs...),
		"pattern_refs":     append([]string(nil), in.Work.PatternRefs...),
		"operation":        ProspectiveAssess,
		"summary":          "prospective-assess routine shaped candidate engineering directions",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_prospective_assess",
		Kind:       euclotypes.ArtifactKindPlanCandidates,
		Summary:    "prospective-assess routine shaped candidate engineering directions",
		Payload:    payload,
		ProducerID: ProspectiveAssess,
		Status:     "produced",
	}}, nil
}

func (convergenceGuardRoutine) ID() string { return ConvergenceGuard }

func (convergenceGuardRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"convergence_refs": append([]string(nil), in.Work.ConvergenceRefs...),
		"operation":        ConvergenceGuard,
		"summary":          "convergence-guard routine checked candidate plans for unresolved divergence",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_convergence_guard",
		Kind:       euclotypes.ArtifactKindPlanCandidates,
		Summary:    "convergence-guard routine checked candidate plans for unresolved divergence",
		Payload:    payload,
		ProducerID: ConvergenceGuard,
		Status:     "produced",
	}}, nil
}

func (coherenceAssessRoutine) ID() string { return CoherenceAssess }

func (coherenceAssessRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"pattern_refs": append([]string(nil), in.Work.PatternRefs...),
		"tension_refs": append([]string(nil), in.Work.TensionRefs...),
		"operation":    CoherenceAssess,
		"summary":      "coherence-assess routine checked whether discovered structures fit together coherently",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_coherence_assess",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    "coherence-assess routine checked whether discovered structures fit together coherently",
		Payload:    payload,
		ProducerID: CoherenceAssess,
		Status:     "produced",
	}}, nil
}

func (scopeExpandRoutine) ID() string { return ScopeExpansionAssess }

func (scopeExpandRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"pattern_refs": append([]string(nil), in.Work.PatternRefs...),
		"operation":    ScopeExpansionAssess,
		"summary":      "scope-expansion routine identified adjacent system areas implicated by the current exploration",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_scope_expansion",
		Kind:       euclotypes.ArtifactKindContextExpansion,
		Summary:    "scope-expansion routine identified adjacent system areas implicated by the current exploration",
		Payload:    payload,
		ProducerID: ScopeExpansionAssess,
		Status:     "produced",
	}}, nil
}
