package debug

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

type rootCauseRoutine struct{}
type hypothesisRefineRoutine struct{}
type localizationRoutine struct{}
type flawSurfaceRoutine struct{}
type verificationRepairRoutine struct{}

func NewSupportingRoutines() []execution.Invocable {
	return []execution.Invocable{
		rootCauseRoutine{},
		hypothesisRefineRoutine{},
		localizationRoutine{},
		flawSurfaceRoutine{},
		verificationRepairRoutine{},
	}
}

func (rootCauseRoutine) ID() string { return RootCause }

func (rootCauseRoutine) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	payload := map[string]any{
		"tension_refs": append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
		"pattern_refs": append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
		"summary":      "root-cause routine derived candidate fault regions from semantic evidence",
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "debug_root_cause_candidates",
		Kind:       euclotypes.ArtifactKindRootCauseCandidates,
		Summary:    "root-cause routine derived candidate fault regions from semantic evidence",
		Payload:    payload,
		ProducerID: RootCause,
		Status:     "produced",
	}}
	return &core.Result{Success: true, Data: map[string]any{"artifacts": artifacts}}, nil
}

func (rootCauseRoutine) IsPrimary() bool { return false }

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

func (hypothesisRefineRoutine) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	payload := map[string]any{
		"pattern_refs": append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
		"prospective":  append([]string(nil), in.Work.SemanticInputs.ProspectiveRefs...),
		"summary":      "hypothesis refinement routine shaped candidate explanations for the failure",
		"confidence":   "medium",
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "debug_hypothesis_refine",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    "hypothesis refinement routine shaped candidate explanations for the failure",
		Payload:    payload,
		ProducerID: HypothesisRefine,
		Status:     "produced",
	}}
	return &core.Result{Success: true, Data: map[string]any{"artifacts": artifacts}}, nil
}

func (hypothesisRefineRoutine) IsPrimary() bool { return false }

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

func (localizationRoutine) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	payload := map[string]any{
		"touched_symbols": append([]string(nil), in.Work.SemanticInputs.RequestProvenanceRefs...),
		"pattern_refs":    append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
		"summary":         "localization routine narrowed the likely implementation scope",
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "debug_localization",
		Kind:       euclotypes.ArtifactKindRootCause,
		Summary:    "localization routine narrowed the likely implementation scope",
		Payload:    payload,
		ProducerID: Localization,
		Status:     "produced",
	}}
	return &core.Result{Success: true, Data: map[string]any{"artifacts": artifacts}}, nil
}

func (localizationRoutine) IsPrimary() bool { return false }

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

func (flawSurfaceRoutine) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	payload := map[string]any{
		"review_source": "euclo:debug.flaw-surface",
		"category":      "flaw_surface",
		"pattern_refs":  append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
		"findings": []map[string]any{
			{
				"severity":         "warning",
				"description":      "flaw-surface routine exposed suspicious design or implementation patterns",
				"rationale":        "debug investigation identified suspicious implementation patterns worth review",
				"category":         "correctness",
				"confidence":       0.6,
				"impacted_files":   []string{},
				"impacted_symbols": append([]string(nil), in.Work.SemanticInputs.RequestProvenanceRefs...),
				"review_source":    "euclo:debug.flaw-surface",
				"traceability": map[string]any{
					"source":       "debug_pattern_context",
					"pattern_refs": append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
				},
			},
		},
		"summary": "flaw-surface routine exposed suspicious design or implementation patterns",
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "debug_flaw_surface",
		Kind:       euclotypes.ArtifactKindReviewFindings,
		Summary:    "flaw-surface routine exposed suspicious design or implementation patterns",
		Payload:    payload,
		ProducerID: FlawSurface,
		Status:     "produced",
	}}
	return &core.Result{Success: true, Data: map[string]any{"artifacts": artifacts}}, nil
}

func (flawSurfaceRoutine) IsPrimary() bool { return false }

func (flawSurfaceRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"review_source": "euclo:debug.flaw-surface",
		"category":      "flaw_surface",
		"pattern_refs":  append([]string(nil), in.Work.PatternRefs...),
		"findings": []map[string]any{
			{
				"severity":         "warning",
				"description":      "flaw-surface routine exposed suspicious design or implementation patterns",
				"rationale":        "debug investigation identified suspicious implementation patterns worth review",
				"category":         "correctness",
				"confidence":       0.6,
				"impacted_files":   []string{},
				"impacted_symbols": append([]string(nil), in.Work.RequestProvenanceRefs...),
				"review_source":    "euclo:debug.flaw-surface",
				"traceability": map[string]any{
					"source":       "debug_pattern_context",
					"pattern_refs": append([]string(nil), in.Work.PatternRefs...),
				},
			},
		},
		"summary": "flaw-surface routine exposed suspicious design or implementation patterns",
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

func (verificationRepairRoutine) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	status := "partial"
	recoveryStatus := ""
	attemptCount := 0
	if in.State != nil {
		if record, ok := euclostate.GetPipelineVerify(in.State); ok && len(record) > 0 {
			if existing, ok := record["status"].(string); ok && existing != "" {
				status = existing
			}
		}
		if raw, ok := in.State.Get(euclostate.KeyRecoveryTrace); ok && raw != nil {
			if record, ok := raw.(map[string]any); ok {
				recoveryStatus = strings.TrimSpace(stringValueAny(record["status"]))
				switch typed := record["attempt_count"].(type) {
				case int:
					attemptCount = typed
				case float64:
					attemptCount = int(typed)
				}
			}
		}
	}
	payload := map[string]any{
		"overall_status":  status,
		"repair_path":     "bounded_debug_repair",
		"provenance":      verificationProvenance(in.State),
		"recovery_status": recoveryStatus,
		"attempt_count":   attemptCount,
		"summary":         "verification-repair routine prepared bounded repair guidance for debug work",
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "debug_verification_repair",
		Kind:       euclotypes.ArtifactKindVerificationSummary,
		Summary:    "verification-repair routine prepared bounded repair guidance for debug work",
		Payload:    payload,
		ProducerID: VerificationRepair,
		Status:     "produced",
	}}
	if in.State != nil {
		if raw, ok := in.State.Get(euclostate.KeyRecoveryTrace); ok && raw != nil {
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "debug_verification_recovery_trace",
				Kind:       euclotypes.ArtifactKindRecoveryTrace,
				Summary:    firstNonEmptyDebug(recoveryStatus, "verification repair recovery trace"),
				Payload:    raw,
				ProducerID: VerificationRepair,
				Status:     "produced",
			})
		}
	}
	return &core.Result{Success: true, Data: map[string]any{"artifacts": artifacts}}, nil
}

func (verificationRepairRoutine) IsPrimary() bool { return false }

func (verificationRepairRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	status := "partial"
	recoveryStatus := ""
	attemptCount := 0
	if in.State != nil {
		if record, ok := euclostate.GetPipelineVerify(in.State); ok && len(record) > 0 {
			if existing, ok := record["status"].(string); ok && existing != "" {
				status = existing
			}
		}
		if raw, ok := in.State.Get(euclostate.KeyRecoveryTrace); ok && raw != nil {
			if record, ok := raw.(map[string]any); ok {
				recoveryStatus = strings.TrimSpace(stringValueAny(record["status"]))
				switch typed := record["attempt_count"].(type) {
				case int:
					attemptCount = typed
				case float64:
					attemptCount = int(typed)
				}
			}
		}
	}
	payload := map[string]any{
		"overall_status":  status,
		"repair_path":     "bounded_debug_repair",
		"provenance":      verificationProvenance(in.State),
		"recovery_status": recoveryStatus,
		"attempt_count":   attemptCount,
		"summary":         "verification-repair routine prepared bounded repair guidance for debug work",
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "debug_verification_repair",
		Kind:       euclotypes.ArtifactKindVerificationSummary,
		Summary:    "verification-repair routine prepared bounded repair guidance for debug work",
		Payload:    payload,
		ProducerID: VerificationRepair,
		Status:     "produced",
	}}
	if in.State != nil {
		if raw, ok := in.State.Get(euclostate.KeyRecoveryTrace); ok && raw != nil {
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "debug_verification_recovery_trace",
				Kind:       euclotypes.ArtifactKindRecoveryTrace,
				Summary:    firstNonEmptyDebug(recoveryStatus, "verification repair recovery trace"),
				Payload:    raw,
				ProducerID: VerificationRepair,
				Status:     "produced",
			})
		}
	}
	return artifacts, nil
}

func verificationProvenance(state *core.Context) string {
	if state == nil {
		return "absent"
	}
	if record, ok := euclostate.GetPipelineVerify(state); ok && len(record) > 0 {
		if value, ok := record["provenance"].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		return "executed"
	}
	return "absent"
}

func stringValueAny(v any) string {
	if value, ok := v.(string); ok {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
