package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type migrationExecuteCapability struct {
	env agentenv.AgentEnvironment
}

func NewMigrationExecuteCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &migrationExecuteCapability{env: env}
}

func (c *migrationExecuteCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:migration.execute",
		Name:          "Migration Execute",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "migration", "planning"},
		Annotations:   map[string]any{"supported_profiles": []string{"plan_stage_execute"}},
	}
}

func (c *migrationExecuteCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindMigrationPlan,
			euclotypes.ArtifactKindEditIntent,
			euclotypes.ArtifactKindVerification,
		},
	}
}

func (c *migrationExecuteCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasWriteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "write tools required for migration execution"}
	}
	if !snapshot.HasVerificationTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "verification tools required for migration execution"}
	}
	if !looksLikeMigrationRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "migration.execute requires explicit migration intent"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "explicit migration request"}
}

func (c *migrationExecuteCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	task := env.Task
	if task == nil {
		task = &core.Task{ID: "migration-execute", Instruction: "Execute migration", Type: core.TaskTypeCodeModification}
	}
	if task.Type == "" {
		task.Type = core.TaskTypeCodeModification
	}
	planPayload := existingMigrationPlan(env.State)
	if len(planPayload) == 0 {
		planPayload = buildMigrationPlanPayload(task)
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "migration_plan",
		Kind:       euclotypes.ArtifactKindMigrationPlan,
		Summary:    summarizePayload(planPayload),
		Payload:    planPayload,
		ProducerID: "euclo:migration.execute",
		Status:     "produced",
	}}
	steps := asRecordSlice(planPayload["steps"])
	completedSteps := 0
	for idx, step := range steps {
		stepID := stringValue(step["id"])
		stateClone := env.State.Clone()
		stateClone.Set("migration.step", step)
		runner := &frameworkpipeline.Runner{Options: frameworkpipeline.RunnerOptions{
			Model:     env.Environment.Model,
			ModelName: migrationModelName(env),
		}}
		stepTask := &core.Task{
			ID:          fmt.Sprintf("%s-migration-step-%d", task.ID, idx+1),
			Instruction: fmt.Sprintf("Execute migration step %s: %s", stepID, stringValue(step["description"])),
			Type:        core.TaskTypeCodeModification,
			Context:     task.Context,
		}
		results, err := runner.Execute(ctx, stepTask, stateClone, newMigrationStages(step))
		postCheck, _ := stateClone.Get("migration.postcheck_result")
		execOutput, _ := stateClone.Get("migration.step_changes")
		if execRecord, ok := execOutput.(map[string]any); ok {
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         stepID + "_edit_intent",
				Kind:       euclotypes.ArtifactKindEditIntent,
				Summary:    firstNonEmpty(stringValue(execRecord["summary"]), stringValue(step["description"])),
				Payload:    execRecord,
				ProducerID: "euclo:migration.execute",
				Status:     "produced",
			})
		}
		if err != nil {
			step["status"] = "failed"
			step["stage_results"] = results
			snapshot := captureMigrationRollbackSnapshot(task, step)
			rollback := restoreMigrationRollbackSnapshot(env.State, snapshot)
			finalVerification := map[string]any{
				"overall_status":  "fail",
				"failed_step":     stepID,
				"rollback":        rollback,
				"stage_results":   results,
				"completed_steps": completedSteps,
				"total_steps":     len(steps),
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "migration_verification",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    fmt.Sprintf("migration halted at %s and rollback attempted", stepID),
				Payload:    finalVerification,
				ProducerID: "euclo:migration.execute",
				Status:     "produced",
			})
			planPayload["completed_steps"] = completedSteps
			planPayload["steps"] = steps
			artifacts[0].Payload = planPayload
			artifacts[0].Summary = summarizePayload(planPayload)
			mergeStateArtifactsToContext(env.State, artifacts)
			_ = persistMigrationArtifacts(ctx, env, artifacts)
			return euclotypes.ExecutionResult{
				Status:    euclotypes.ExecutionStatusPartial,
				Summary:   fmt.Sprintf("migration failed at step %s", stepID),
				Artifacts: artifacts,
				FailureInfo: &euclotypes.CapabilityFailure{
					Code:         "migration_step_failed",
					Message:      err.Error(),
					Recoverable:  true,
					FailedPhase:  "execute",
					ParadigmUsed: "pipeline",
				},
				RecoveryHint: &euclotypes.RecoveryHint{Strategy: euclotypes.RecoveryStrategyProfileEscalation, Context: map[string]any{"failed_step": stepID, "rollback_result": rollback}},
			}
		}
		step["status"] = "completed"
		step["stage_results"] = results
		step["postcheck_result"] = postCheck
		completedSteps++
	}
	finalVerification := map[string]any{
		"overall_status":  "pass",
		"completed_steps": completedSteps,
		"total_steps":     len(steps),
		"summary":         fmt.Sprintf("migration completed %d/%d steps", completedSteps, len(steps)),
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "migration_verification",
		Kind:       euclotypes.ArtifactKindVerification,
		Summary:    "migration verification completed",
		Payload:    finalVerification,
		ProducerID: "euclo:migration.execute",
		Status:     "produced",
	})
	planPayload["completed_steps"] = completedSteps
	planPayload["steps"] = steps
	artifacts[0].Payload = planPayload
	artifacts[0].Summary = summarizePayload(planPayload)
	mergeStateArtifactsToContext(env.State, artifacts)
	_ = persistMigrationArtifacts(ctx, env, artifacts)
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "migration executed successfully", Artifacts: artifacts}
}

func looksLikeMigrationRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	if text == "" {
		return false
	}
	for _, token := range []string{"migration", "migrate", "upgrade dependency", "upgrade api", "schema change", "dependency update", "rollout", "move to v", "upgrade to v"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func existingMigrationPlan(state *core.Context) map[string]any {
	if state == nil {
		return nil
	}
	if raw, ok := state.Get("euclo.migration_plan"); ok {
		if record, ok := raw.(map[string]any); ok {
			return record
		}
	}
	return nil
}

func buildMigrationPlanPayload(task *core.Task) map[string]any {
	migrationType := inferMigrationType(capTaskInstruction(task))
	files := fileNamesFromTask(task)
	steps := []map[string]any{
		{
			"id":             "assess_current_state",
			"description":    fmt.Sprintf("Assess current %s state and prepare the migration boundary", migrationType),
			"preconditions":  []string{"workspace is readable", "migration target is identified"},
			"postconditions": []string{"migration scope is confirmed", "affected files are enumerated"},
			"rollback_path":  "no-op; assessment only",
			"blast_radius":   "low",
			"status":         "pending",
			"files":          files,
		},
		{
			"id":                   "apply_migration_changes",
			"description":          fmt.Sprintf("Apply the %s changes in dependency/config/code order", migrationType),
			"preconditions":        []string{"migration scope is confirmed"},
			"postconditions":       []string{"code and configuration reflect the new target state"},
			"rollback_path":        "restore pre-migration file snapshot",
			"blast_radius":         "medium",
			"status":               "pending",
			"files":                files,
			"force_postcheck_fail": taskContextString(task, "migration_fail_postcheck_step") == "apply_migration_changes",
			"force_precheck_fail":  taskContextString(task, "migration_fail_precheck_step") == "apply_migration_changes",
		},
		{
			"id":                   "verify_and_finalize",
			"description":          fmt.Sprintf("Verify the %s migration and finalize follow-up notes", migrationType),
			"preconditions":        []string{"migration changes are applied"},
			"postconditions":       []string{"verification checks pass", "rollback path remains documented"},
			"rollback_path":        "restore pre-migration file snapshot and dependency versions",
			"blast_radius":         "low",
			"status":               "pending",
			"files":                files,
			"force_postcheck_fail": taskContextString(task, "migration_fail_postcheck_step") == "verify_and_finalize",
			"force_precheck_fail":  taskContextString(task, "migration_fail_precheck_step") == "verify_and_finalize",
		},
	}
	return map[string]any{"migration_type": migrationType, "steps": steps, "rollback_strategy": "per_step", "completed_steps": 0, "total_steps": len(steps)}
}

func inferMigrationType(instruction string) string {
	lower := strings.ToLower(instruction)
	switch {
	case strings.Contains(lower, "schema"), strings.Contains(lower, "database"):
		return "schema_change"
	case strings.Contains(lower, "dependency"), strings.Contains(lower, "version"), strings.Contains(lower, "upgrade"):
		return "dependency_update"
	case strings.Contains(lower, "api"):
		return "api_upgrade"
	default:
		return "migration"
	}
}

type taskContextFile struct {
	Path    string
	Content string
}

func taskContextFiles(task *core.Task) []taskContextFile {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context["context_file_contents"]
	if !ok {
		return nil
	}
	var files []taskContextFile
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			files = append(files, taskContextFile{Path: stringValue(record["path"]), Content: stringValue(record["content"])})
		}
	case []map[string]any:
		for _, record := range typed {
			files = append(files, taskContextFile{Path: stringValue(record["path"]), Content: stringValue(record["content"])})
		}
	}
	return files
}

func fileNamesFromTask(task *core.Task) []string {
	var files []string
	for _, file := range taskContextFiles(task) {
		if file.Path != "" {
			files = append(files, file.Path)
		}
	}
	return uniqueStrings(files)
}

func migrationWorkflowStatePath(task *core.Task) string {
	if custom := taskContextString(task, "workflow_state_path"); custom != "" {
		return custom
	}
	taskID := "migration"
	if task != nil && strings.TrimSpace(task.ID) != "" {
		taskID = strings.TrimSpace(task.ID)
	}
	return filepath.Join(os.TempDir(), "relurpify-migration-workflows", taskID+".db")
}

func migrationModelName(env euclotypes.ExecutionEnvelope) string {
	if env.Environment.Config == nil {
		return ""
	}
	return env.Environment.Config.Model
}

func asRecordSlice(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if record, ok := item.(map[string]any); ok {
				out = append(out, record)
			}
		}
		return out
	default:
		return nil
	}
}

func persistMigrationArtifacts(ctx context.Context, env euclotypes.ExecutionEnvelope, artifacts []euclotypes.Artifact) error {
	return euclotypes.PersistWorkflowArtifacts(ctx, env.WorkflowStore, env.WorkflowID, env.RunID, artifacts)
}
