package architect

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents/internal/workflowutil"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
)

func (a *ArchitectAgent) executeLegacyPlan(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	selection, err := a.maybeSelectCandidate(ctx, task, state)
	if err != nil {
		return nil, err
	}
	plannerTask := core.CloneTask(task)
	if selection != "" {
		plannerTask.Instruction = fmt.Sprintf("%s\n\nPreferred approach:\n%s", task.Instruction, selection)
	}
	planResult, err := a.planner.Execute(ctx, plannerTask, state)
	if err != nil {
		return nil, err
	}
	planVal, ok := state.Get("planner.plan")
	if !ok {
		return nil, fmt.Errorf("planner did not populate a plan")
	}
	plan, ok := planVal.(core.Plan)
	if !ok {
		return nil, fmt.Errorf("planner produced invalid plan")
	}
	state.Set("architect.plan", plan)
	state.Set("architect.completed_steps", []string{})
	state.Set("plan.completed_steps", []string{})
	state.Set("architect.last_step_summary", "")
	plannerResultData := plannerOutputFromState(state, planResult, plan, nil)
	if plannerResultData != nil {
		state.Set("architect.plan_result", compactPlannerOutputState(plan, plannerResultData, nil))
	}
	executor := graph.PlanExecutor{
		Options: graph.PlanExecutionOptions{
			MaxRecoveryAttempts: 2,
			BuildStepTask:       a.buildPlanStepTask,
			CompletedStepIDs: func(state *core.Context) []string {
				return core.StringSliceFromContext(state, "architect.completed_steps")
			},
			Diagnose: a.diagnoseStepFailure,
			Recover:  a.recoverStepFailure,
			BeforeStep: func(step core.PlanStep, stepTask *core.Task, state *core.Context) {
				state.Set("architect.current_step", step)
				state.Set("architect.current_step_id", step.ID)
			},
			AfterStep: func(step core.PlanStep, state *core.Context, result *core.Result) {
				completed := append([]string{}, core.StringSliceFromContext(state, "architect.completed_steps")...)
				if !containsString(completed, step.ID) {
					completed = append(completed, step.ID)
				}
				state.Set("architect.completed_steps", completed)
				state.Set("plan.completed_steps", completed)
				if result != nil {
					summary := summarizeStepResult(step, result)
					state.Set("architect.last_step_summary", summary)
				}
			},
		},
	}
	execResult, err := executor.Execute(ctx, a.executor, task, &plan, state)
	if err != nil {
		return nil, err
	}
	verifySummary := fmt.Sprintf("Planned %d steps and executed %d steps.", len(plan.Steps), len(core.StringSliceFromContext(state, "architect.completed_steps")))
	state.Set("architect.summary", verifySummary)
	clearArchitectActiveStepState(state)
	if execResult.Data == nil {
		execResult.Data = map[string]any{}
	}
	execResult.Data["plan"] = plan
	if plannerResultData != nil {
		execResult.Data["planner"] = plannerResultData
	} else if raw, ok := state.Get("architect.plan_result"); ok {
		execResult.Data["planner"] = raw
	}
	execResult.Data["summary"] = verifySummary
	return execResult, nil
}

func (a *ArchitectAgent) buildPlanStepTask(parentTask *core.Task, plan *core.Plan, step core.PlanStep, state *core.Context) *core.Task {
	stepTask := core.CloneTask(parentTask)
	if stepTask == nil {
		stepTask = &core.Task{}
	}
	if stepTask.Context == nil {
		stepTask.Context = map[string]any{}
	}
	stepTask.Context["current_step"] = step
	if plan != nil && strings.TrimSpace(plan.Goal) != "" {
		stepTask.Context["plan_goal"] = plan.Goal
	}
	if previous := strings.TrimSpace(state.GetString("architect.last_step_summary")); previous != "" {
		stepTask.Context["previous_step_result"] = previous
	}
	stepTask.Instruction = fmt.Sprintf("Execute step %s only: %s", step.ID, step.Description)
	if len(step.Files) > 0 {
		stepTask.Instruction += fmt.Sprintf("\nRelevant files: %v", step.Files)
	}
	if step.Expected != "" {
		stepTask.Instruction += fmt.Sprintf("\nExpected outcome: %s", step.Expected)
	}
	if step.Verification != "" {
		stepTask.Instruction += fmt.Sprintf("\nVerification: %s", step.Verification)
	}
	return stepTask
}

func (a *ArchitectAgent) maybeSelectCandidate(ctx context.Context, task *core.Task, state *core.Context) (string, error) {
	service := a.planning
	if service == nil {
		service = &WorkflowPlanningService{
			Model:        a.Model,
			Planner:      a.planner,
			PlannerTools: a.PlannerTools,
			Config:       a.Config,
		}
	}
	return service.selectCandidate(ctx, task, state)
}

func shouldRunCandidateSelection(task *core.Task) bool {
	if task == nil {
		return false
	}
	text := strings.ToLower(task.Instruction)
	markers := []string{"approach", "architecture", "design", "refactor", "either", "alternatively", "trade-off", "tradeoff"}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func architectPlanningHints(agent *ArchitectAgent) string {
	if agent == nil {
		return ""
	}
	return planningHintsForConfig(agent.Config, agent.PlannerTools)
}

func (a *ArchitectAgent) executeWithWorkflowStore(ctx context.Context, task *core.Task, state *core.Context, store *db.SQLiteWorkflowStateStore) (*core.Result, error) {
	workflowID, resumeRequested, err := a.resolveWorkflowIdentity(ctx, task, store)
	if err != nil {
		return nil, err
	}
	runID := task.ID
	if strings.TrimSpace(runID) == "" {
		runID = fmt.Sprintf("run_%d", time.Now().UnixNano())
	}
	state.Set("architect.workflow_id", workflowID)
	state.Set("architect.run_id", runID)

	workflow, ok, err := store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if !ok {
		if resumeRequested {
			return nil, fmt.Errorf("workflow %s not found for resume", workflowID)
		}
		workflow = &memory.WorkflowRecord{
			WorkflowID:  workflowID,
			TaskID:      workflowID,
			TaskType:    task.Type,
			Instruction: task.Instruction,
			Status:      memory.WorkflowRunStatusPending,
			Metadata: map[string]any{
				"mode": "architect",
			},
		}
		if err := store.CreateWorkflow(ctx, *workflow); err != nil {
			return nil, err
		}
		workflow, _, err = store.GetWorkflow(ctx, workflowID)
		if err != nil {
			return nil, err
		}
	}
	if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:          runID,
		WorkflowID:     workflowID,
		Status:         memory.WorkflowRunStatusRunning,
		AgentName:      "architect",
		AgentMode:      "architect",
		RuntimeVersion: "external-state-store-v1",
		Metadata: map[string]any{
			"task_id": task.ID,
		},
	}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "unique") {
		return nil, err
	}

	planRecord, ok, err := store.GetActivePlan(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// No active plan found — run the planning phase regardless of how the
		// workflow was identified. Providing a workflow_id associates this run
		// with existing retrieval context; it does not require a pre-existing plan.
		planning := a.planning
		if planning == nil {
			planning = &WorkflowPlanningService{
				Model:        a.Model,
				Planner:      a.planner,
				PlannerTools: a.PlannerTools,
				Config:       a.Config,
			}
		}
		planningResult, err := planning.PlanAndPersist(ctx, task, state, store, workflowID, runID)
		if err != nil {
			return nil, err
		}
		planRecord = &planningResult.PlanRecord
	}
	state.Set("architect.plan", planRecord.Plan)
	state.Set("planner.plan", planRecord.Plan)

	if _, err := store.UpdateWorkflowStatus(ctx, workflowID, workflow.Version, memory.WorkflowRunStatusRunning, workflow.CursorStepID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := a.applyReplayDirectives(ctx, task, store, workflowID); err != nil {
		return nil, err
	}

	var lastResult *core.Result
	for {
		steps, err := store.ListSteps(ctx, workflowID)
		if err != nil {
			return nil, err
		}
		completed := completedStepsFromRecords(steps)
		state.Set("architect.completed_steps", completed)
		state.Set("plan.completed_steps", completed)

		if allStepsTerminal(steps) {
			break
		}
		ready, err := store.ListReadySteps(ctx, workflowID)
		if err != nil {
			return nil, err
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("no ready steps available for workflow %s", workflowID)
		}
		step := ready[0]
		if _, err := store.UpdateWorkflowStatus(ctx, workflowID, 0, memory.WorkflowRunStatusRunning, step.StepID); err != nil {
			return nil, err
		}
		res, err := a.executeWorkflowStep(ctx, task, state, store, workflowID, runID, planRecord.Plan, step)
		if err != nil {
			if errors.Is(err, errArchitectNeedsReplan) {
				return nil, err
			}
			_ = store.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusFailed, timePtr(time.Now().UTC()))
			_, _ = store.UpdateWorkflowStatus(ctx, workflowID, 0, memory.WorkflowRunStatusFailed, step.StepID)
			return nil, err
		}
		lastResult = res
	}

	summary := fmt.Sprintf("Planned %d steps and executed %d steps.", len(planRecord.Plan.Steps), len(core.StringSliceFromContext(state, "architect.completed_steps")))
	state.Set("architect.summary", summary)
	state.Set("architect.workflow_id", workflowID)
	state.Set("architect.run_id", runID)
	_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    architectRecordID("workflow_done"),
		WorkflowID: workflowID,
		RunID:      runID,
		EventType:  "workflow_completed",
		Message:    summary,
		CreatedAt:  time.Now().UTC(),
	})
	_ = store.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusCompleted, timePtr(time.Now().UTC()))
	_, _ = store.UpdateWorkflowStatus(ctx, workflowID, 0, memory.WorkflowRunStatusCompleted, "")
	if lastResult == nil {
		lastResult = &core.Result{Success: true, Data: map[string]any{}}
	}
	if lastResult.Data == nil {
		lastResult.Data = map[string]any{}
	}
	lastResult.Data["plan"] = planRecord.Plan
	lastResult.Data["summary"] = summary
	return lastResult, nil
}

func (a *ArchitectAgent) executeWorkflowStep(ctx context.Context, task *core.Task, state *core.Context, store *db.SQLiteWorkflowStateStore, workflowID, runID string, plan core.Plan, step memory.WorkflowStepRecord) (*core.Result, error) {
	stepSlice, ok, err := store.LoadStepSlice(ctx, workflowID, step.StepID, 20)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("step slice %s not found", step.StepID)
	}
	stepState := core.NewContext()
	stepState.Set("task.id", task.ID)
	stepState.Set("task.type", string(task.Type))
	stepState.Set("task.instruction", task.Instruction)
	stepState.Set("architect.workflow_id", workflowID)
	stepState.Set("architect.run_id", runID)
	stepState.Set("architect.plan", plan)
	stepState.Set("planner.plan", plan)
	stepState.Set("architect.completed_steps", core.StringSliceFromContext(state, "architect.completed_steps"))
	stepState.Set("plan.completed_steps", core.StringSliceFromContext(state, "architect.completed_steps"))
	if retrievalPayload, err := workflowutil.Hydrate(ctx, store, workflowID, workflowutil.RetrievalQuery{
		Primary:      step.Step.Description,
		TaskText:     task.Instruction,
		StepID:       step.StepID,
		StepFiles:    append([]string{}, step.Step.Files...),
		Expected:     step.Step.Expected,
		Verification: step.Step.Verification,
		PreviousNotes: compactStrings(
			dependencySummary(stepSlice.DependencyRuns),
			knowledgeSummary(stepSlice.Facts),
			knowledgeSummary(stepSlice.Decisions),
			knowledgeSummary(stepSlice.Issues),
		),
	}, 4, 500); err != nil {
		return nil, err
	} else if len(retrievalPayload) > 0 {
		workflowutil.ApplyState(stepState, "architect.workflow_retrieval", retrievalPayload)
		workflowutil.ApplyState(state, "architect.workflow_retrieval", retrievalPayload)
	}

	previousSummary := dependencySummary(stepSlice.DependencyRuns)
	if previousSummary != "" {
		stepState.Set("architect.last_step_summary", previousSummary)
	}
	state.Set("architect.current_step_id", step.StepID)
	state.Set("architect.current_step", step.Step)
	state.Set("architect.last_step_summary", previousSummary)
	if err := store.UpdateStepStatus(ctx, workflowID, step.StepID, memory.StepStatusRunning, previousSummary); err != nil {
		return nil, err
	}
	_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    architectRecordID("step_started"),
		WorkflowID: workflowID,
		RunID:      runID,
		StepID:     step.StepID,
		EventType:  "step_started",
		Message:    step.Step.Description,
		CreatedAt:  time.Now().UTC(),
	})
	startedAt := time.Now().UTC()
	attempt := nextStepAttempt(ctx, store, workflowID, step.StepID)
	stepTask := buildWorkflowStepTask(task, plan, stepSlice, previousSummary)
	if raw, ok := stepState.Get("architect.workflow_retrieval"); ok {
		if payload, ok := raw.(map[string]any); ok {
			stepTask = workflowutil.ApplyTask(stepTask, payload)
		}
	}
	result, execErr := a.executor.Execute(ctx, stepTask, stepState)
	finishedAt := time.Now().UTC()
	status := memory.StepStatusCompleted
	summary := summarizeStepResult(step.Step, result)
	errorText := ""
	if execErr != nil || result == nil || !result.Success {
		status = memory.StepStatusFailed
		if execErr != nil {
			errorText = execErr.Error()
			summary = execErr.Error()
		} else {
			errorText = "step failed"
			summary = "step failed"
		}
	}
	if err := store.CreateStepRun(ctx, memory.StepRunRecord{
		StepRunID:      architectRecordID("step_run"),
		WorkflowID:     workflowID,
		RunID:          runID,
		StepID:         step.StepID,
		Attempt:        attempt,
		Status:         status,
		Summary:        summary,
		ResultData:     resultData(result, errorText),
		VerificationOK: status == memory.StepStatusCompleted,
		ErrorText:      errorText,
		StartedAt:      startedAt,
		FinishedAt:     &finishedAt,
	}); err != nil {
		return nil, err
	}
	stepRunID := latestStepRunID(ctx, store, workflowID, step.StepID)
	if err := store.UpdateStepStatus(ctx, workflowID, step.StepID, status, summary); err != nil {
		return nil, err
	}
	_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    architectRecordID("step_finished"),
		WorkflowID: workflowID,
		RunID:      runID,
		StepID:     step.StepID,
		EventType:  string(status),
		Message:    summary,
		CreatedAt:  finishedAt,
	})
	artifactPayload := mustJSONForArchitect(resultData(result, errorText))
	stepArtifact := memory.StepArtifactRecord{
		ArtifactID:        architectRecordID("step_artifact"),
		WorkflowID:        workflowID,
		StepRunID:         stepRunID,
		Kind:              "step_result",
		ContentType:       "application/json",
		StorageKind:       memory.ArtifactStorageInline,
		SummaryText:       summary,
		InlineRawText:     artifactPayload,
		RawSizeBytes:      int64(len(artifactPayload)),
		CompressionMethod: "none",
		CreatedAt:         finishedAt,
	}
	_ = store.UpsertArtifact(ctx, stepArtifact)
	state.Set("architect.last_step_result_ref", workflowutil.StepArtifactReference(stepArtifact))
	state.Set("architect.last_step_result_summary", summary)
	a.persistStepSecurityEvents(ctx, store, workflowID, runID, step.StepID, stepState, result, finishedAt)
	a.persistStepKnowledge(ctx, store, workflowID, stepRunID, step.StepID, summary, status, stepTask, result, errorText, finishedAt)
	if status != memory.StepStatusCompleted {
		if nextStepAttempt(ctx, store, workflowID, step.StepID)-1 >= a.stepNeedsReplanThreshold() {
			if err := store.UpdateStepStatus(ctx, workflowID, step.StepID, memory.StepStatusNeedsReplan, summary); err == nil {
				_, _ = store.UpdateWorkflowStatus(ctx, workflowID, 0, memory.WorkflowRunStatusNeedsReplan, step.StepID)
				_ = store.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusNeedsReplan, timePtr(finishedAt))
				_ = store.PutKnowledge(ctx, memory.KnowledgeRecord{
					RecordID:   architectRecordID("issue"),
					WorkflowID: workflowID,
					StepRunID:  stepRunID,
					StepID:     step.StepID,
					Kind:       memory.KnowledgeKindIssue,
					Title:      "Workflow requires replanning",
					Content:    summary,
					Status:     "open",
					Metadata:   map[string]any{"requires_replan": true},
					CreatedAt:  finishedAt,
				})
				_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
					EventID:    architectRecordID("needs_replan"),
					WorkflowID: workflowID,
					RunID:      runID,
					StepID:     step.StepID,
					EventType:  "needs_replan",
					Message:    fmt.Sprintf("Step %s requires replanning after repeated failures.", step.StepID),
					CreatedAt:  finishedAt,
				})
				return nil, fmt.Errorf("%w: step %s requires replanning after repeated failures", errArchitectNeedsReplan, step.StepID)
			}
		}
		return nil, fmt.Errorf("step %s failed: %s", step.StepID, summary)
	}
	completed := append([]string{}, core.StringSliceFromContext(state, "architect.completed_steps")...)
	if !containsString(completed, step.StepID) {
		completed = append(completed, step.StepID)
	}
	state.Set("architect.completed_steps", completed)
	state.Set("plan.completed_steps", completed)
	state.Set("architect.last_step_summary", summary)
	if result != nil {
		if final, ok := stepState.Get("react.final_output"); ok {
			state.Set("react.final_output", final)
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["final_output"] = final
		}
	}
	return result, nil
}

func buildWorkflowStepTask(task *core.Task, plan core.Plan, slice *memory.WorkflowStepSlice, previousSummary string) *core.Task {
	step := slice.Step.Step
	stepTask := core.CloneTask(task)
	if stepTask == nil {
		stepTask = &core.Task{}
	}
	if stepTask.Context == nil {
		stepTask.Context = map[string]any{}
	}
	stepTask.Context["current_step"] = step
	stepTask.Context["plan_goal"] = plan.Goal
	if previousSummary != "" {
		stepTask.Context["previous_step_result"] = previousSummary
	}
	stepTask.Context["external_state_slice"] = renderExternalStateSlice(slice)
	stepTask.Instruction = fmt.Sprintf("Execute step %s only: %s", step.ID, step.Description)
	if len(step.Files) > 0 {
		stepTask.Instruction += fmt.Sprintf("\nRelevant files: %v", step.Files)
	}
	if step.Expected != "" {
		stepTask.Instruction += fmt.Sprintf("\nExpected outcome: %s", step.Expected)
	}
	if step.Verification != "" {
		stepTask.Instruction += fmt.Sprintf("\nVerification: %s", step.Verification)
	}
	return stepTask
}

func renderExternalStateSlice(slice *memory.WorkflowStepSlice) string {
	if slice == nil {
		return ""
	}
	type depSummary struct {
		StepID   string `json:"step_id"`
		Summary  string `json:"summary"`
		Attempts int    `json:"attempts"`
	}
	type artifactSummary struct {
		Kind    string `json:"kind"`
		Summary string `json:"summary"`
	}
	payload := map[string]any{
		"workflow_id": slice.Workflow.WorkflowID,
		"current_step": map[string]any{
			"id":          slice.Step.Step.ID,
			"description": slice.Step.Step.Description,
			"files":       slice.Step.Step.Files,
		},
	}
	deps := make([]depSummary, 0, len(slice.DependencyRuns))
	for _, run := range slice.DependencyRuns {
		deps = append(deps, depSummary{StepID: run.StepID, Summary: run.Summary, Attempts: run.Attempt})
	}
	if len(deps) > 0 {
		payload["dependency_runs"] = deps
	}
	if len(slice.Artifacts) > 0 {
		artifacts := make([]artifactSummary, 0, len(slice.Artifacts))
		for _, artifact := range slice.Artifacts {
			artifacts = append(artifacts, artifactSummary{Kind: artifact.Kind, Summary: artifact.SummaryText})
		}
		payload["artifacts"] = artifacts
	}
	if len(slice.Facts) > 0 {
		payload["facts"] = knowledgeContents(slice.Facts)
	}
	if len(slice.Issues) > 0 {
		payload["issues"] = knowledgeContents(slice.Issues)
	}
	if len(slice.Decisions) > 0 {
		payload["decisions"] = knowledgeContents(slice.Decisions)
	}
	return mustJSONForArchitect(payload)
}
