package chat

import (
	"context"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

type localReviewRoutine struct{}
type targetedVerificationRoutine struct{}

func NewSupportingRoutines() []euclorelurpic.SupportingRoutine {
	return []euclorelurpic.SupportingRoutine{
		localReviewRoutine{},
		targetedVerificationRoutine{},
	}
}

func (localReviewRoutine) ID() string { return LocalReview }

func (localReviewRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	payload := map[string]any{
		"primary_capability_id": in.Work.PrimaryCapabilityID,
		"focus":                 chatFocusLens(in.Task),
		"scope":                 taskFiles(in.Task),
		"summary":               "local review routine prepared inspection findings",
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

func (targetedVerificationRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	status := "partial"
	checks := []any{map[string]any{"name": "targeted_verification_scope", "status": "partial"}}
	if in.State != nil {
		if raw, ok := in.State.Get("pipeline.verify"); ok && raw != nil {
			if record, ok := raw.(map[string]any); ok {
				if value, ok := record["status"].(string); ok && strings.TrimSpace(value) != "" {
					status = strings.TrimSpace(value)
				}
				if existingChecks, ok := record["checks"].([]any); ok && len(existingChecks) > 0 {
					checks = existingChecks
				}
			}
		}
	}
	payload := map[string]any{
		"status":     status,
		"checks":     checks,
		"repairable": true,
		"summary":    "targeted verification repair routine evaluated local verification posture",
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
