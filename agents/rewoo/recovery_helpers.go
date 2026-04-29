package rewoo

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// RecoveryScenario describes a failure scenario and recovery strategy.
type RecoveryScenario struct {
	ScenarioID      string                 // Unique identifier
	FailureType     string                 // "execution_error", "step_failure", "synthesis_error", "process_crash"
	StepsAttempted  []string               // Step IDs that were executed
	FailedSteps     []string               // Step IDs that failed
	FailureReason   string                 // Human-readable error description
	SuggestedAction string                 // "retry", "replan", "synthesize_from_results", "resume_from_checkpoint"
	CheckpointID    string                 // Checkpoint to resume from (if applicable)
	RiskLevel       string                 // "low", "medium", "high"
	Metadata        map[string]interface{} // Additional context
}

// DiagnosisResult represents the diagnosis of a workflow failure.
type DiagnosisResult struct {
	IsRecoverable bool               // Can the workflow recover?
	Scenarios     []RecoveryScenario // Possible recovery paths
	RecommendedID string             // ID of recommended scenario
	RiskLevel     string             // "low", "medium", "high"
}

// DiagnoseStepFailure analyzes step execution results and recommends recovery strategy.
// Phase 8: Helper for recovery workflows.
func DiagnoseStepFailure(ctx context.Context, env *contextdata.Envelope, results []RewooStepResult, plan *RewooPlan) *DiagnosisResult {
	diagnosis := &DiagnosisResult{
		IsRecoverable: true,
		Scenarios:     make([]RecoveryScenario, 0),
	}

	if len(results) == 0 || plan == nil {
		diagnosis.IsRecoverable = false
		return diagnosis
	}

	// Analyze failure patterns
	failedCount := 0
	failedSteps := make([]string, 0)
	for _, result := range results {
		if !result.Success {
			failedCount++
			failedSteps = append(failedSteps, result.StepID)
		}
	}

	if failedCount == 0 {
		diagnosis.IsRecoverable = false
		return diagnosis
	}

	failureRatio := float64(failedCount) / float64(len(results))

	// Scenario 1: Low failure rate - retry failed steps
	if failureRatio <= 0.25 {
		diagnosis.Scenarios = append(diagnosis.Scenarios, RecoveryScenario{
			ScenarioID:      "retry_failed_steps",
			FailureType:     "step_failure",
			StepsAttempted:  extractStepIDs(results),
			FailedSteps:     failedSteps,
			FailureReason:   fmt.Sprintf("%.0f%% of steps failed", failureRatio*100),
			SuggestedAction: "retry",
			RiskLevel:       "low",
			Metadata: map[string]interface{}{
				"failed_count":  failedCount,
				"total_count":   len(results),
				"failure_ratio": failureRatio,
			},
		})
		diagnosis.RecommendedID = "retry_failed_steps"
		diagnosis.RiskLevel = "low"
	}

	// Scenario 2: Medium failure rate - replan with context
	if failureRatio > 0.25 && failureRatio <= 0.75 {
		// Check for checkpoint to resume from
		checkpointID := ""
		if cpID, ok := env.GetWorkingValue("rewoo.checkpoint_id"); ok {
			if id, ok := cpID.(string); ok {
				checkpointID = id
			}
		}

		diagnosis.Scenarios = append(diagnosis.Scenarios, RecoveryScenario{
			ScenarioID:      "replan_with_context",
			FailureType:     "execution_error",
			StepsAttempted:  extractStepIDs(results),
			FailedSteps:     failedSteps,
			FailureReason:   fmt.Sprintf("%.0f%% of steps failed, context needed", failureRatio*100),
			SuggestedAction: "replan",
			CheckpointID:    checkpointID,
			RiskLevel:       "medium",
			Metadata: map[string]interface{}{
				"failed_count":   failedCount,
				"total_count":    len(results),
				"failure_ratio":  failureRatio,
				"error_patterns": analyzeErrorPatterns(results),
			},
		})
		diagnosis.RecommendedID = "replan_with_context"
		diagnosis.RiskLevel = "medium"
	}

	// Scenario 3: High failure rate - synthesize from partial results
	if failureRatio > 0.75 {
		diagnosis.Scenarios = append(diagnosis.Scenarios, RecoveryScenario{
			ScenarioID:      "synthesize_from_results",
			FailureType:     "execution_error",
			StepsAttempted:  extractStepIDs(results),
			FailedSteps:     failedSteps,
			FailureReason:   fmt.Sprintf("%.0f%% of steps failed, limited recovery possible", failureRatio*100),
			SuggestedAction: "synthesize_from_results",
			RiskLevel:       "high",
			Metadata: map[string]interface{}{
				"failed_count":    failedCount,
				"total_count":     len(results),
				"failure_ratio":   failureRatio,
				"partial_results": countPartialResults(results),
			},
		})
		diagnosis.RecommendedID = "synthesize_from_results"
		diagnosis.RiskLevel = "high"
	}

	return diagnosis
}

// RecoverStepFailure executes recovery based on diagnosis.
// Phase 8: Helper for implementing recovery workflows.
func RecoverStepFailure(ctx context.Context, env *contextdata.Envelope, diagnosis *DiagnosisResult, store *RewooCheckpointStore) error {
	if !diagnosis.IsRecoverable || diagnosis.RecommendedID == "" {
		return fmt.Errorf("recovery: scenario not recoverable")
	}

	// Find recommended scenario
	var recommendedScenario *RecoveryScenario
	for i := range diagnosis.Scenarios {
		if diagnosis.Scenarios[i].ScenarioID == diagnosis.RecommendedID {
			recommendedScenario = &diagnosis.Scenarios[i]
			break
		}
	}

	if recommendedScenario == nil {
		return fmt.Errorf("recovery: recommended scenario not found")
	}

	// Execute recovery strategy
	switch recommendedScenario.SuggestedAction {
	case "retry":
		// Retry: mark failed steps for retry
		env.SetWorkingValue("rewoo.retry_steps", recommendedScenario.FailedSteps, contextdata.MemoryClassTask)
		env.SetWorkingValue("rewoo.recovery_action", "retry", contextdata.MemoryClassTask)
		return nil

	case "replan":
		// Replan: signal replan is needed with context
		env.SetWorkingValue("rewoo.replan_required", true, contextdata.MemoryClassTask)
		env.SetWorkingValue("rewoo.failed_steps", recommendedScenario.FailedSteps, contextdata.MemoryClassTask)
		env.SetWorkingValue("rewoo.recovery_action", "replan", contextdata.MemoryClassTask)
		return nil

	case "synthesize_from_results":
		// Synthesize: continue with partial results
		env.SetWorkingValue("rewoo.synthesis_partial", true, contextdata.MemoryClassTask)
		env.SetWorkingValue("rewoo.recovery_action", "synthesize_from_results", contextdata.MemoryClassTask)
		return nil

	default:
		return fmt.Errorf("recovery: unknown action %s", recommendedScenario.SuggestedAction)
	}
}

// Helper functions

func extractStepIDs(results []RewooStepResult) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.StepID)
	}
	return ids
}

func analyzeErrorPatterns(results []RewooStepResult) map[string]int {
	patterns := make(map[string]int)
	for _, result := range results {
		if !result.Success && result.Error != "" {
			// Simple categorization
			if result.Error == "timeout" {
				patterns["timeout"]++
			} else if result.Error == "tool_not_found" {
				patterns["tool_not_found"]++
			} else if result.Error == "permission_denied" {
				patterns["permission_denied"]++
			} else {
				patterns["other"]++
			}
		}
	}
	return patterns
}

func countPartialResults(results []RewooStepResult) int {
	count := 0
	for _, result := range results {
		// Partial result: has output but not fully successful
		if !result.Success && result.Output != nil {
			count++
		}
	}
	return count
}
