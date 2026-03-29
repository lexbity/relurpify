package debug

import (
	"context"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

type rootCauseRoutine struct{}
type hypothesisRefineRoutine struct{}
type localizationRoutine struct{}
type flawSurfaceRoutine struct{}
type verificationRepairRoutine struct{}

func NewSupportingRoutines() []euclorelurpic.SupportingRoutine {
	return []euclorelurpic.SupportingRoutine{
		rootCauseRoutine{},
		hypothesisRefineRoutine{},
		localizationRoutine{},
		flawSurfaceRoutine{},
		verificationRepairRoutine{},
	}
}

func (rootCauseRoutine) ID() string { return RootCause }

func (rootCauseRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"tension_refs": append([]string(nil), in.Work.TensionRefs...),
		"pattern_refs": append([]string(nil), in.Work.PatternRefs...),
		"summary":      "root-cause routine derived candidate fault regions from semantic evidence",
	}
	return []euclotypes.Artifact{{
		ID:         "debug_root_cause_candidates",
		Kind:       euclotypes.ArtifactKindRootCauseCandidates,
		Summary:    "root-cause routine derived candidate fault regions from semantic evidence",
		Payload:    payload,
		ProducerID: RootCause,
		Status:     "produced",
	}}, nil
}

func (hypothesisRefineRoutine) ID() string { return HypothesisRefine }

func (hypothesisRefineRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"pattern_refs": append([]string(nil), in.Work.PatternRefs...),
		"prospective":  append([]string(nil), in.Work.ProspectiveRefs...),
		"summary":      "hypothesis refinement routine shaped candidate explanations for the failure",
		"confidence":   "medium",
	}
	return []euclotypes.Artifact{{
		ID:         "debug_hypothesis_refine",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    "hypothesis refinement routine shaped candidate explanations for the failure",
		Payload:    payload,
		ProducerID: HypothesisRefine,
		Status:     "produced",
	}}, nil
}

func (localizationRoutine) ID() string { return Localization }

func (localizationRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"touched_symbols": append([]string(nil), in.Work.RequestProvenanceRefs...),
		"pattern_refs":    append([]string(nil), in.Work.PatternRefs...),
		"summary":         "localization routine narrowed the likely implementation scope",
	}
	return []euclotypes.Artifact{{
		ID:         "debug_localization",
		Kind:       euclotypes.ArtifactKindRootCause,
		Summary:    "localization routine narrowed the likely implementation scope",
		Payload:    payload,
		ProducerID: Localization,
		Status:     "produced",
	}}, nil
}

func (flawSurfaceRoutine) ID() string { return FlawSurface }

func (flawSurfaceRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"category":     "flaw_surface",
		"pattern_refs": append([]string(nil), in.Work.PatternRefs...),
		"summary":      "flaw-surface routine exposed suspicious design or implementation patterns",
	}
	return []euclotypes.Artifact{{
		ID:         "debug_flaw_surface",
		Kind:       euclotypes.ArtifactKindReviewFindings,
		Summary:    "flaw-surface routine exposed suspicious design or implementation patterns",
		Payload:    payload,
		ProducerID: FlawSurface,
		Status:     "produced",
	}}, nil
}

func (verificationRepairRoutine) ID() string { return VerificationRepair }

func (verificationRepairRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	status := "partial"
	if in.State != nil {
		if raw, ok := in.State.Get("pipeline.verify"); ok && raw != nil {
			if record, ok := raw.(map[string]any); ok {
				if existing, ok := record["status"].(string); ok && existing != "" {
					status = existing
				}
			}
		}
	}
	payload := map[string]any{
		"status":      status,
		"repair_path": "bounded_debug_repair",
		"summary":     "verification-repair routine prepared bounded repair guidance for debug work",
	}
	return []euclotypes.Artifact{{
		ID:         "debug_verification_repair",
		Kind:       euclotypes.ArtifactKindVerificationSummary,
		Summary:    "verification-repair routine prepared bounded repair guidance for debug work",
		Payload:    payload,
		ProducerID: VerificationRepair,
		Status:     "produced",
	}}, nil
}
