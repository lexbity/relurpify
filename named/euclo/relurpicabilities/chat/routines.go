package chat

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

type directEditExecutionRoutine struct{}
type localReviewRoutine struct{}
type targetedVerificationRoutine struct{}

func NewSupportingRoutines() []execution.Invocable {
	return []execution.Invocable{
		localReviewRoutine{},
		targetedVerificationRoutine{},
		directEditExecutionRoutine{},
	}
}

func (directEditExecutionRoutine) ID() string { return DirectEditExecution }

func (directEditExecutionRoutine) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	editTarget := ""
	if in.State != nil {
		if raw, ok := in.State.Get("chat.edit_target"); ok && raw != nil {
			if s, ok := raw.(string); ok {
				editTarget = strings.TrimSpace(s)
			}
		}
	}
	payload := map[string]any{
		"primary_capability_id": in.Work.PrimaryRelurpicCapabilityID,
		"routine_source":        DirectEditExecution,
		"edit_target":           editTarget,
		"status":                "prepared",
		"summary":               "direct edit execution routine prepared patch context for chat.implement",
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "chat_direct_edit_execution",
		Kind:       euclotypes.ArtifactKindExecutionStatus,
		Summary:    "direct edit execution routine prepared patch context for chat.implement",
		Payload:    payload,
		ProducerID: DirectEditExecution,
		Status:     "produced",
	}}
	return &core.Result{Success: true, Data: map[string]any{"artifacts": artifacts}}, nil
}

func (directEditExecutionRoutine) IsPrimary() bool { return false }

func (localReviewRoutine) ID() string { return LocalReview }

func (localReviewRoutine) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	payload := map[string]any{
		"primary_capability_id": in.Work.PrimaryRelurpicCapabilityID,
		"review_source":         LocalReview,
		"focus":                 chatFocusLens(in.Task),
		"scope": map[string]any{
			"files":      taskFiles(in.Task),
			"focus_lens": chatFocusLens(in.Task),
		},
		"findings": []map[string]any{},
		"summary":  "local review routine prepared inspection findings",
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "chat_local_review",
		Kind:       euclotypes.ArtifactKindReviewFindings,
		Summary:    "local review routine prepared inspection findings",
		Payload:    payload,
		ProducerID: LocalReview,
		Status:     "produced",
	}}
	return &core.Result{Success: true, Data: map[string]any{"artifacts": artifacts}}, nil
}

func (localReviewRoutine) IsPrimary() bool { return false }

func (localReviewRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"primary_capability_id": in.Work.PrimaryCapabilityID,
		"review_source":         LocalReview,
		"focus":                 chatFocusLens(in.Task),
		"scope": map[string]any{
			"files":      taskFiles(in.Task),
			"focus_lens": chatFocusLens(in.Task),
		},
		"findings": []map[string]any{},
		"summary":  "local review routine prepared inspection findings",
	}
	return []euclotypes.Artifact{{
		ID:         "chat_local_review",
		Kind:       euclotypes.ArtifactKindReviewFindings,
		Summary:    "local review routine prepared inspection findings",
		Payload:    payload,
		ProducerID: LocalReview,
		Status:     "produced",
	}}, nil
}

func (targetedVerificationRoutine) ID() string { return TargetedVerification }

func (targetedVerificationRoutine) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	status := "partial"
	checks := []any{map[string]any{"name": "targeted_verification_scope", "status": "partial"}}
	if in.State != nil {
		if record, ok := euclostate.GetPipelineVerify(in.State); ok && len(record) > 0 {
			if value, ok := record["status"].(string); ok && strings.TrimSpace(value) != "" {
				status = strings.TrimSpace(value)
			}
			if existingChecks, ok := record["checks"].([]any); ok && len(existingChecks) > 0 {
				checks = existingChecks
			}
		}
	}
	payload := map[string]any{
		"overall_status": status,
		"checks":         checks,
		"repairable":     true,
		"provenance":     verificationProvenance(in.State),
		"summary":        "targeted verification repair routine evaluated local verification posture",
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "chat_targeted_verification",
		Kind:       euclotypes.ArtifactKindVerificationSummary,
		Summary:    "targeted verification repair routine evaluated local verification posture",
		Payload:    payload,
		ProducerID: TargetedVerification,
		Status:     "produced",
	}}
	return &core.Result{Success: true, Data: map[string]any{"artifacts": artifacts}}, nil
}

func (targetedVerificationRoutine) IsPrimary() bool { return false }

func (targetedVerificationRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	status := "partial"
	checks := []any{map[string]any{"name": "targeted_verification_scope", "status": "partial"}}
	if in.State != nil {
		if record, ok := euclostate.GetPipelineVerify(in.State); ok && len(record) > 0 {
			if value, ok := record["status"].(string); ok && strings.TrimSpace(value) != "" {
				status = strings.TrimSpace(value)
			}
			if existingChecks, ok := record["checks"].([]any); ok && len(existingChecks) > 0 {
				checks = existingChecks
			}
		}
	}
	payload := map[string]any{
		"overall_status": status,
		"checks":         checks,
		"repairable":     true,
		"provenance":     verificationProvenance(in.State),
		"summary":        "targeted verification repair routine evaluated local verification posture",
	}
	return []euclotypes.Artifact{{
		ID:         "chat_targeted_verification",
		Kind:       euclotypes.ArtifactKindVerificationSummary,
		Summary:    "targeted verification repair routine evaluated local verification posture",
		Payload:    payload,
		ProducerID: TargetedVerification,
		Status:     "produced",
	}}, nil
}

func verificationProvenance(state *core.Context) string {
	if state == nil {
		return "absent"
	}
	if raw, ok := state.Get("pipeline.verify"); ok && raw != nil {
		if record, ok := raw.(map[string]any); ok {
			if value, ok := record["provenance"].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		return "executed"
	}
	return "absent"
}

func chatFocusLens(task *core.Task) string {
	if task == nil {
		return "general"
	}
	lower := strings.ToLower(task.Instruction)
	for _, lens := range []string{"security", "performance", "compatibility", "correctness", "style"} {
		if strings.Contains(lower, lens) {
			return lens
		}
	}
	return "general"
}

func taskFiles(task *core.Task) []string {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context["context_file_contents"]
	if !ok {
		return nil
	}
	var files []string
	switch typed := raw.(type) {
	case []map[string]any:
		for _, item := range typed {
			if path, ok := item["path"].(string); ok && strings.TrimSpace(path) != "" {
				files = append(files, strings.TrimSpace(path))
			}
		}
	case []any:
		for _, item := range typed {
			if record, ok := item.(map[string]any); ok {
				if path, ok := record["path"].(string); ok && strings.TrimSpace(path) != "" {
					files = append(files, strings.TrimSpace(path))
				}
			}
		}
	}
	return files
}
