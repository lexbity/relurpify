package architect

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents/internal/workflowutil"
	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	frameworksearch "github.com/lexcodex/relurpify/framework/search"
)

// ArchitectAgent uses a small-model-friendly workflow:
// 1. Generate an explicit plan once.
// 2. Execute one plan step at a time with a fresh, compact ReAct context.
// 3. Persist workflow state after planning and each completed step.
type ArchitectAgent struct {
	Model             core.LanguageModel
	PlannerTools      *capability.Registry
	ExecutorTools     *capability.Registry
	Memory            memory.MemoryStore
	Config            *core.Config
	IndexManager      *ast.IndexManager
	SearchEngine      *frameworksearch.SearchEngine
	CheckpointPath    string
	WorkflowStatePath string

	planner  *plannerpkg.PlannerAgent
	executor *reactpkg.ReActAgent
	planning *WorkflowPlanningService
}

var errArchitectNeedsReplan = errors.New("architect workflow requires replanning")

const ModeArchitect = "architect"

func (a *ArchitectAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.PlannerTools == nil {
		a.PlannerTools = capability.NewRegistry()
	}
	if a.ExecutorTools == nil {
		a.ExecutorTools = capability.NewRegistry()
	}
	a.planner = &plannerpkg.PlannerAgent{
		Model:  a.Model,
		Tools:  a.PlannerTools,
		Memory: a.Memory,
	}
	if err := a.planner.Initialize(cfg); err != nil {
		return err
	}
	a.executor = &reactpkg.ReActAgent{
		Model:          a.Model,
		Tools:          a.ExecutorTools,
		Memory:         a.Memory,
		Config:         cfg,
		IndexManager:   a.IndexManager,
		SearchEngine:   a.SearchEngine,
		CheckpointPath: a.CheckpointPath,
		Mode:           "code",
	}
	a.planning = &WorkflowPlanningService{
		Model:        a.Model,
		Planner:      a.planner,
		PlannerTools: a.PlannerTools,
		Config:       cfg,
	}
	return a.executor.Initialize(cfg)
}

func (a *ArchitectAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityCode,
		core.CapabilityExplain,
	}
}

func (a *ArchitectAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.planner == nil {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	return a.planner.BuildGraph(task)
}

func (a *ArchitectAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	if state == nil {
		state = core.NewContext()
	}
	if a.planner == nil || a.executor == nil {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}

	state.Set("architect.mode", "plan_execute")
	state.Set("task.id", task.ID)
	state.Set("task.type", string(task.Type))
	state.Set("task.instruction", task.Instruction)

	store, err := a.openWorkflowStateStore()
	if err != nil {
		return nil, err
	}
	if store != nil {
		defer store.Close()
		return a.executeWithWorkflowStore(ctx, task, state, store)
	}
	return a.executeLegacyPlan(ctx, task, state)
}

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

func clearArchitectActiveStepState(state *core.Context) {
	if state == nil {
		return
	}
	state.Set("architect.current_step", map[string]any{})
	state.Set("architect.current_step_id", "")
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
		planRecord, err = a.planIntoWorkflowStore(ctx, task, state, store, workflowID, runID)
		if err != nil {
			return nil, err
		}
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

func (a *ArchitectAgent) planIntoWorkflowStore(ctx context.Context, task *core.Task, state *core.Context, store *db.SQLiteWorkflowStateStore, workflowID, runID string) (*memory.WorkflowPlanRecord, error) {
	service := a.planning
	if service == nil {
		service = &WorkflowPlanningService{
			Model:        a.Model,
			Planner:      a.planner,
			PlannerTools: a.PlannerTools,
			Config:       a.Config,
		}
	}
	result, err := service.PlanAndPersist(ctx, task, state, store, workflowID, runID)
	if err != nil {
		return nil, err
	}
	return &result.PlanRecord, nil
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

func (a *ArchitectAgent) resolveWorkflowIdentity(ctx context.Context, task *core.Task, store *db.SQLiteWorkflowStateStore) (string, bool, error) {
	if task == nil {
		return "", false, fmt.Errorf("task required")
	}
	if task.Context != nil {
		if raw, ok := task.Context["workflow_id"]; ok && strings.TrimSpace(fmt.Sprint(raw)) != "" {
			return strings.TrimSpace(fmt.Sprint(raw)), true, nil
		}
		if enabled, ok := task.Context["resume_latest_workflow"]; ok {
			if flag, ok := enabled.(bool); ok && flag {
				workflows, err := store.ListWorkflows(ctx, 1)
				if err != nil {
					return "", true, err
				}
				if len(workflows) > 0 {
					return workflows[0].WorkflowID, true, nil
				}
			}
		}
	}
	if strings.TrimSpace(task.ID) != "" {
		return strings.TrimSpace(task.ID), false, nil
	}
	return fmt.Sprintf("workflow_%d", time.Now().UnixNano()), false, nil
}

func (a *ArchitectAgent) workflowStateDBPath() string {
	if strings.TrimSpace(a.WorkflowStatePath) != "" {
		return filepath.Clean(a.WorkflowStatePath)
	}
	return ""
}

func (a *ArchitectAgent) openWorkflowStateStore() (*db.SQLiteWorkflowStateStore, error) {
	path := a.workflowStateDBPath()
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return db.NewSQLiteWorkflowStateStore(path)
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

func (a *ArchitectAgent) applyReplayDirectives(ctx context.Context, task *core.Task, store *db.SQLiteWorkflowStateStore, workflowID string) error {
	if task == nil || task.Context == nil {
		return nil
	}
	rerunFrom := optionalContextString(task.Context, "rerun_from_step_id")
	rerunStep := optionalContextString(task.Context, "rerun_step_id")
	rerunInvalidated, _ := task.Context["rerun_invalidated"].(bool)

	switch {
	case rerunFrom != "":
		return a.resetReplayFromStep(ctx, store, workflowID, rerunFrom, true)
	case rerunStep != "":
		return a.resetReplayFromStep(ctx, store, workflowID, rerunStep, true)
	case rerunInvalidated:
		return a.resetInvalidatedSteps(ctx, store, workflowID)
	default:
		return nil
	}
}

func (a *ArchitectAgent) resetReplayFromStep(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, stepID string, includeDependents bool) error {
	if err := store.UpdateStepStatus(ctx, workflowID, stepID, memory.StepStatusPending, "queued for replay"); err != nil {
		return err
	}
	if includeDependents {
		invalidations, err := store.InvalidateDependents(ctx, workflowID, stepID, "rerun requested")
		if err != nil {
			return err
		}
		for _, record := range invalidations {
			if err := store.UpdateStepStatus(ctx, workflowID, record.InvalidatedStepID, memory.StepStatusPending, "queued for replay"); err != nil {
				return err
			}
		}
	}
	_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    architectRecordID("rerun_requested"),
		WorkflowID: workflowID,
		StepID:     stepID,
		EventType:  "rerun_requested",
		Message:    fmt.Sprintf("Replay requested from step %s.", stepID),
		CreatedAt:  time.Now().UTC(),
	})
	return nil
}

func (a *ArchitectAgent) resetInvalidatedSteps(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID string) error {
	steps, err := store.ListSteps(ctx, workflowID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if step.Status != memory.StepStatusInvalidated {
			continue
		}
		if err := store.UpdateStepStatus(ctx, workflowID, step.StepID, memory.StepStatusPending, "queued for replay"); err != nil {
			return err
		}
	}
	return nil
}

func (a *ArchitectAgent) persistStepKnowledge(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, stepRunID, stepID, summary string, status memory.StepStatus, stepTask *core.Task, result *core.Result, errorText string, createdAt time.Time) {
	if status == memory.StepStatusCompleted {
		_ = store.PutKnowledge(ctx, memory.KnowledgeRecord{
			RecordID:   architectRecordID("fact"),
			WorkflowID: workflowID,
			StepRunID:  stepRunID,
			StepID:     stepID,
			Kind:       memory.KnowledgeKindFact,
			Title:      "Completed step result",
			Content:    summary,
			Status:     "accepted",
			CreatedAt:  createdAt,
		})
		if stepTask != nil && stepTask.Context != nil {
			if previous := strings.TrimSpace(fmt.Sprint(stepTask.Context["previous_step_result"])); previous != "" {
				_ = store.PutKnowledge(ctx, memory.KnowledgeRecord{
					RecordID:   architectRecordID("decision"),
					WorkflowID: workflowID,
					StepRunID:  stepRunID,
					StepID:     stepID,
					Kind:       memory.KnowledgeKindDecision,
					Title:      "Step constraint",
					Content:    previous,
					Status:     "accepted",
					Metadata:   map[string]any{"source": "dependency_summary"},
					CreatedAt:  createdAt,
				})
			}
		}
		if result != nil && result.Data != nil {
			if text := strings.TrimSpace(fmt.Sprint(result.Data["text"])); text != "" && text != "<nil>" {
				_ = store.PutKnowledge(ctx, memory.KnowledgeRecord{
					RecordID:   architectRecordID("fact"),
					WorkflowID: workflowID,
					StepRunID:  stepRunID,
					StepID:     stepID,
					Kind:       memory.KnowledgeKindFact,
					Title:      "Execution text",
					Content:    text,
					Status:     "accepted",
					CreatedAt:  createdAt,
				})
			}
		}
		return
	}
	content := summary
	if strings.TrimSpace(errorText) != "" {
		content = errorText
	}
	_ = store.PutKnowledge(ctx, memory.KnowledgeRecord{
		RecordID:   architectRecordID("issue"),
		WorkflowID: workflowID,
		StepRunID:  stepRunID,
		StepID:     stepID,
		Kind:       memory.KnowledgeKindIssue,
		Title:      "Step failure",
		Content:    content,
		Status:     "open",
		CreatedAt:  createdAt,
	})
}

func (a *ArchitectAgent) stepNeedsReplanThreshold() int {
	if a != nil && a.Config != nil && a.Config.MaxIterations > 0 {
		return 3
	}
	return 3
}

func knowledgeContents(records []memory.KnowledgeRecord) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		text := strings.TrimSpace(record.Content)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func knowledgeSummary(records []memory.KnowledgeRecord) string {
	return strings.Join(knowledgeContents(records), "\n")
}

func compactStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func completedStepsFromRecords(steps []memory.WorkflowStepRecord) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Status == memory.StepStatusCompleted {
			out = append(out, step.StepID)
		}
	}
	return out
}

func allStepsTerminal(steps []memory.WorkflowStepRecord) bool {
	if len(steps) == 0 {
		return true
	}
	for _, step := range steps {
		if step.Status == memory.StepStatusPending || step.Status == memory.StepStatusRunning {
			return false
		}
	}
	return true
}

func dependencySummary(runs []memory.StepRunRecord) string {
	if len(runs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(runs))
	for _, run := range runs {
		if strings.TrimSpace(run.Summary) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", run.StepID, run.Summary))
	}
	return strings.Join(parts, "\n")
}

func nextStepAttempt(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, stepID string) int {
	runs, err := store.ListStepRuns(ctx, workflowID, stepID)
	if err != nil || len(runs) == 0 {
		return 1
	}
	return runs[len(runs)-1].Attempt + 1
}

func latestStepRunID(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, stepID string) string {
	runs, err := store.ListStepRuns(ctx, workflowID, stepID)
	if err != nil || len(runs) == 0 {
		return ""
	}
	return runs[len(runs)-1].StepRunID
}

func resultData(result *core.Result, errorText string) map[string]any {
	if result == nil {
		return map[string]any{"error": errorText}
	}
	if result.Data == nil {
		if errorText == "" {
			return map[string]any{}
		}
		return map[string]any{"error": errorText}
	}
	out := make(map[string]any, len(result.Data)+1)
	for k, v := range result.Data {
		out[k] = v
	}
	if errorText != "" {
		out["error"] = errorText
	}
	return out
}

func mustJSONForArchitect(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func architectRecordID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func (a *ArchitectAgent) diagnoseStepFailure(ctx context.Context, step core.PlanStep, err error) (string, error) {
	if err == nil {
		return "", nil
	}
	if a == nil || a.Model == nil {
		return diagnoseStepFailure(ctx, step, err)
	}
	prompt := fmt.Sprintf(`Summarize a recovery action for a failed implementation step.
Return one short paragraph with a concrete next action.
Step: %s
Files: %v
Error: %v`, step.Description, step.Files, err)
	resp, genErr := a.Model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       a.Config.Model,
		Temperature: 0,
		MaxTokens:   160,
	})
	if genErr != nil {
		return diagnoseStepFailure(ctx, step, err)
	}
	return strings.TrimSpace(resp.Text), nil
}

func (a *ArchitectAgent) recoverStepFailure(ctx context.Context, step core.PlanStep, stepTask *core.Task, state *core.Context, err error) (*graph.StepRecovery, error) {
	if err == nil {
		return nil, nil
	}
	recoveryNotes, recoveryContext := a.runRecoveryMiniLoop(ctx, step, state, err)
	recovery := &graph.StepRecovery{
		Diagnosis: fmt.Sprintf("Step %s failed and needs a narrower retry.", step.ID),
		Notes:     append(buildRecoveryNotes(step, state, err), recoveryNotes...),
		Context:   recoveryContext,
	}
	if a != nil && a.Model != nil {
		prompt := fmt.Sprintf(`Produce a compact recovery plan for one failed coding step.
Return JSON as {"diagnosis":"...","notes":["..."]}.
Step: %s
Files: %v
Previous step result: %s
Last error: %v`, step.Description, step.Files, state.GetString("architect.last_step_summary"), err)
		resp, genErr := a.Model.Generate(ctx, prompt, &core.LLMOptions{
			Model:       a.Config.Model,
			Temperature: 0,
			MaxTokens:   220,
		})
		if genErr == nil {
			var parsed struct {
				Diagnosis string   `json:"diagnosis"`
				Notes     []string `json:"notes"`
			}
			if jsonErr := json.Unmarshal([]byte(reactpkg.ExtractJSON(resp.Text)), &parsed); jsonErr == nil {
				if strings.TrimSpace(parsed.Diagnosis) != "" {
					recovery.Diagnosis = strings.TrimSpace(parsed.Diagnosis)
				}
				if len(parsed.Notes) > 0 {
					recovery.Notes = append([]string{}, parsed.Notes...)
				}
			}
		}
	}
	if recovery.Context == nil {
		recovery.Context = map[string]any{}
	}
	if len(step.Files) > 0 {
		recovery.Context["recovery_files"] = append([]string{}, step.Files...)
	}
	if stepTask != nil && stepTask.Context != nil {
		if current, ok := stepTask.Context["current_step"]; ok {
			recovery.Context["recovery_step"] = current
		}
	}
	if state != nil {
		state.Set("architect.last_recovery_notes", append([]string{}, recovery.Notes...))
		state.Set("architect.last_recovery_diagnosis", recovery.Diagnosis)
	}
	return recovery, nil
}

func (a *ArchitectAgent) runRecoveryMiniLoop(ctx context.Context, step core.PlanStep, state *core.Context, err error) ([]string, map[string]any) {
	tools := a.recoveryRegistry()
	if tools == nil {
		return nil, nil
	}
	notes := make([]string, 0, 4)
	evidence := make(map[string]any)

	for _, path := range limitStrings(uniqueStrings(step.Files), 2) {
		if result, execErr := a.executeRecoveryTool(ctx, tools, state, "file_read", map[string]any{"path": path}); execErr == nil && result != nil && result.Success {
			content := strings.TrimSpace(fmt.Sprint(result.Data["content"]))
			snippet := firstRecoverySnippet(content)
			if snippet != "" {
				notes = append(notes, fmt.Sprintf("Inspected %s: %s", path, snippet))
				appendRecoveryEvidence(evidence, "file_reads", map[string]any{"path": path, "snippet": snippet})
			}
		}
	}

	if pattern := recoverySearchPattern(err); pattern != "" {
		args := map[string]any{"pattern": pattern}
		if dir := recoverySearchDirectory(step.Files); dir != "" {
			args["directory"] = dir
		}
		if result, execErr := a.executeRecoveryTool(ctx, tools, state, "search_grep", args); execErr == nil && result != nil && result.Success {
			matches := countRecoveryMatches(result.Data["matches"])
			if matches > 0 {
				notes = append(notes, fmt.Sprintf("Found %d matching lines for %q during recovery.", matches, pattern))
				appendRecoveryEvidence(evidence, "grep", map[string]any{"pattern": pattern, "matches": matches})
			}
		} else if result, execErr := a.executeRecoveryTool(ctx, tools, state, "file_search", args); execErr == nil && result != nil && result.Success {
			matches := countRecoveryMatches(result.Data["matches"])
			if matches > 0 {
				notes = append(notes, fmt.Sprintf("Found %d matching lines for %q during recovery.", matches, pattern))
				appendRecoveryEvidence(evidence, "grep", map[string]any{"pattern": pattern, "matches": matches})
			}
		}
	}

	for _, symbol := range limitStrings(uniqueStrings(extractRecoverySymbols(step)), 1) {
		if result, execErr := a.executeRecoveryTool(ctx, tools, state, "query_ast", map[string]any{"action": "get_signature", "symbol": symbol}); execErr == nil && result != nil && result.Success {
			signature := strings.TrimSpace(fmt.Sprint(result.Data["signature"]))
			if signature != "" {
				notes = append(notes, fmt.Sprintf("AST signature for %s: %s", symbol, truncateRecovery(signature, 120)))
				appendRecoveryEvidence(evidence, "ast", map[string]any{"symbol": symbol, "signature": signature})
			}
		}
	}

	if len(notes) == 0 && len(evidence) == 0 {
		return nil, nil
	}
	return uniqueStrings(notes), evidence
}

func (a *ArchitectAgent) recoveryRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	if a.ExecutorTools != nil {
		return a.ExecutorTools
	}
	return a.PlannerTools
}

func (a *ArchitectAgent) executeRecoveryTool(ctx context.Context, registry *capability.Registry, state *core.Context, name string, args map[string]any) (*core.ToolResult, error) {
	if registry == nil {
		return nil, nil
	}
	if !registry.HasCapability(name) {
		return nil, nil
	}
	if !registry.CapabilityAvailable(ctx, state, name) {
		return nil, nil
	}
	return registry.InvokeCapability(ctx, state, name, args)
}

func summarizeStepResult(step core.PlanStep, result *core.Result) string {
	if result == nil {
		return fmt.Sprintf("Step %s completed.", step.ID)
	}
	if result.Error != nil {
		return fmt.Sprintf("Step %s failed: %v", step.ID, result.Error)
	}
	return fmt.Sprintf("Step %s completed: %s", step.ID, step.Description)
}

func buildRecoveryNotes(step core.PlanStep, state *core.Context, err error) []string {
	notes := []string{
		fmt.Sprintf("Re-check the failing step goal: %s", step.Description),
		fmt.Sprintf("Validate the reported error before editing again: %v", err),
	}
	if len(step.Files) > 0 {
		notes = append(notes, fmt.Sprintf("Inspect the step files first: %s", strings.Join(step.Files, ", ")))
	}
	if state != nil {
		if previous := strings.TrimSpace(state.GetString("architect.last_step_summary")); previous != "" {
			notes = append(notes, "Use the previous step summary as a constraint: "+previous)
		}
	}
	return notes
}

func extractRecoverySymbols(step core.PlanStep) []string {
	return contextmgr.ExtractSymbolReferences(step.Description + " " + strings.Join(step.Files, " "))
}

func recoverySearchPattern(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	pattern := strings.TrimSpace(lines[0])
	if idx := strings.Index(pattern, ":"); idx > 0 {
		rest := strings.TrimSpace(pattern[idx+1:])
		if rest != "" {
			pattern = rest
		}
	}
	if len(pattern) > 80 {
		pattern = pattern[:80]
	}
	return strings.TrimSpace(pattern)
}

func recoverySearchDirectory(files []string) string {
	if len(files) == 0 {
		return "."
	}
	dir := filepath.Dir(files[0])
	if dir == "." || dir == "" {
		return "."
	}
	return dir
}

func countRecoveryMatches(raw any) int {
	switch matches := raw.(type) {
	case []map[string]interface{}:
		return len(matches)
	case []any:
		return len(matches)
	default:
		return 0
	}
}

func (a *ArchitectAgent) persistStepSecurityEvents(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, runID, stepID string, stepState *core.Context, result *core.Result, createdAt time.Time) {
	if store == nil {
		return
	}
	var rawEnvelope any
	var ok bool
	if stepState != nil {
		rawEnvelope, ok = stepState.Get("react.last_tool_result_envelope")
	}
	if (!ok || rawEnvelope == nil) && stepState != nil {
		if rawLast, found := stepState.Get("react.last_result"); found && rawLast != nil {
			if lastResult, typed := rawLast.(*core.Result); typed && lastResult != nil && lastResult.Metadata != nil {
				rawEnvelope, ok = lastResult.Metadata["capability_result"]
			}
		}
	}
	if (!ok || rawEnvelope == nil) && result != nil && result.Metadata != nil {
		rawEnvelope, ok = result.Metadata["capability_result"]
	}
	if !ok || rawEnvelope == nil {
		if stepState == nil {
			return
		}
		rawObs, found := stepState.Get("react.tool_observations")
		if !found || rawObs == nil {
			return
		}
		observations, typed := rawObs.([]reactpkg.ToolObservation)
		if !typed || len(observations) == 0 {
			return
		}
		last := observations[len(observations)-1]
		if strings.TrimSpace(last.Tool) == "" {
			return
		}
		_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
			EventID:    architectRecordID("security_event"),
			WorkflowID: workflowID,
			RunID:      runID,
			StepID:     stepID,
			EventType:  "security.capability_invoked",
			Message:    fmt.Sprintf("Capability %s invoked during workflow step execution.", last.Tool),
			Metadata: map[string]any{
				"capability_id": "tool:" + last.Tool,
				"capability":    last.Tool,
				"phase":         last.Phase,
				"success":       last.Success,
			},
			CreatedAt: createdAt,
		})
		return
	}
	envelope, ok := rawEnvelope.(*core.CapabilityResultEnvelope)
	if !ok || envelope == nil {
		return
	}
	metadata := map[string]any{
		"capability_id": envelope.Descriptor.ID,
		"capability":    envelope.Descriptor.Name,
		"kind":          string(envelope.Descriptor.Kind),
		"trust_class":   string(envelope.Descriptor.TrustClass),
		"insertion":     string(envelope.Insertion.Action),
	}
	if envelope.Policy != nil {
		metadata["policy_snapshot_id"] = envelope.Policy.ID
	}
	if envelope.Descriptor.Source.ProviderID != "" {
		metadata["provider_id"] = envelope.Descriptor.Source.ProviderID
	}
	if envelope.Descriptor.Source.SessionID != "" {
		metadata["session_id"] = envelope.Descriptor.Source.SessionID
	}
	if envelope.Approval != nil && envelope.Approval.TargetResource != "" {
		metadata["target_resource"] = envelope.Approval.TargetResource
	}
	_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    architectRecordID("security_event"),
		WorkflowID: workflowID,
		RunID:      runID,
		StepID:     stepID,
		EventType:  "security.insertion_decision",
		Message:    fmt.Sprintf("Capability %s insertion resolved as %s.", envelope.Descriptor.Name, envelope.Insertion.Action),
		Metadata:   metadata,
		CreatedAt:  createdAt,
	})
}

func appendRecoveryEvidence(evidence map[string]any, key string, value any) {
	if evidence == nil || key == "" || value == nil {
		return
	}
	current, ok := evidence[key]
	if !ok {
		evidence[key] = []any{value}
		return
	}
	switch values := current.(type) {
	case []any:
		evidence[key] = append(values, value)
	default:
		evidence[key] = []any{values, value}
	}
}

func firstRecoverySnippet(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return truncateRecovery(line, 120)
	}
	return ""
}

func truncateRecovery(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max]) + "..."
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func limitStrings(values []string, max int) []string {
	if max <= 0 || len(values) <= max {
		return append([]string{}, values...)
	}
	return append([]string{}, values[:max]...)
}

func diagnoseStepFailure(_ context.Context, step core.PlanStep, err error) (string, error) {
	if err == nil {
		return "", nil
	}
	return fmt.Sprintf("Retry step %s with a narrower change set and validate the failing file first. Last error: %v", step.ID, err), nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func optionalContextString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func coerceArchitectPlan(raw any) (core.Plan, bool) {
	switch typed := raw.(type) {
	case core.Plan:
		return typed, true
	case *core.Plan:
		if typed == nil {
			return core.Plan{}, false
		}
		return *typed, true
	default:
		encoded, err := json.Marshal(raw)
		if err != nil {
			return core.Plan{}, false
		}
		var decoded core.Plan
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return core.Plan{}, false
		}
		if decoded.Dependencies == nil {
			decoded.Dependencies = map[string][]string{}
		}
		return decoded, true
	}
}
