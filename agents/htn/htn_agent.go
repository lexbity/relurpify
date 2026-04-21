package htn

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/agents/htn/runtime"
	"codeburg.org/lexbit/relurpify/agents/internal/workflowutil"
	agentpipeline "codeburg.org/lexbit/relurpify/agents/pipeline"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	frameworkpipeline "codeburg.org/lexbit/relurpify/framework/pipeline"
)

// HTNAgent implements graph.WorkflowExecutor using a Hierarchical Task Network (HTN)
// planning approach. Complex tasks are decomposed into primitive subtasks by
// the method library; a primitive executor (default: any graph.WorkflowExecutor) then
// runs each leaf step.
//
// The agent is small-model-friendly: the LLM never decides how to structure
// work, it only executes focused, narrowly-scoped subtasks.
type HTNAgent struct {
	// Model is the language model used by the primitive executor.
	Model core.LanguageModel
	// Tools is the capability registry passed to the primitive executor.
	Tools *capability.Registry
	// Memory is the memory store shared with the primitive executor.
	Memory memory.MemoryStore
	// Config holds runtime configuration.
	Config *core.Config
	// Methods is the method library. Defaults to NewMethodLibrary() when nil.
	Methods *MethodLibrary
	// PrimitiveExec is the executor used for leaf subtasks.
	// It must be initialised before Execute is called.
	// When nil, HTNAgent falls back to a no-op that marks steps successful.
	PrimitiveExec graph.WorkflowExecutor
	// CheckpointPath is an optional filesystem path for checkpoint storage.
	CheckpointPath string

	initialised bool

	// SemanticContext is the pre-resolved semantic context bundle passed
	// to the agent at construction time. It propagates to PrimitiveExec
	// when that executor is a *react.ReActAgent.
	SemanticContext core.AgentSemanticContext
}

// Initialize satisfies graph.WorkflowExecutor. It wires configuration and ensures the
// method library is populated.
func (a *HTNAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Methods == nil {
		a.Methods = NewMethodLibrary()
	}
	// Validate method library before use.
	for _, method := range a.Methods.All() {
		if err := method.Validate(); err != nil {
			return fmt.Errorf("htn: invalid method library: %w", err)
		}
	}
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	if a.PrimitiveExec != nil {
		if err := a.PrimitiveExec.Initialize(cfg); err != nil {
			return fmt.Errorf("htn: primitive executor initialisation failed: %w", err)
		}
	}
	a.initialised = true
	return nil
}

// Capabilities declares what this agent can do.
func (a *HTNAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityCode,
	}
}

// BuildGraph returns a minimal single-node graph suitable for agenttest and
// visualisation. HTN execution is driven by Execute, not a static graph walk.
func (a *HTNAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("htn_done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	if err := g.SetStart("htn_done"); err != nil {
		return nil, err
	}
	return g, nil
}

// Execute decomposes the task and runs each subtask through the primitive
// executor.
func (a *HTNAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if state == nil {
		state = core.NewContext()
	}
	surfaces := workflowutil.ResolveRuntimeSurfaces(a.Memory)
	closeWorkflowStore := func() {}
	if surfaces.Workflow == nil && strings.TrimSpace(a.CheckpointPath) != "" {
		store, err := db.NewSQLiteWorkflowStateStore(filepath.Clean(a.CheckpointPath))
		if err != nil {
			return nil, fmt.Errorf("htn: open workflow checkpoint store: %w", err)
		}
		surfaces.Workflow = store
		closeWorkflowStore = func() { _ = store.Close() }
	}
	defer closeWorkflowStore()
	var workflowID, runID string
	if surfaces.Workflow != nil {
		var err error
		workflowID, runID, err = workflowutil.EnsureWorkflowRun(ctx, surfaces.Workflow, task, state, "htn")
		if err != nil {
			return nil, fmt.Errorf("htn: workflow init failed: %w", err)
		}
		if err := a.resumeCheckpoint(ctx, surfaces.Workflow, workflowID, runID, task, state); err != nil {
			return nil, fmt.Errorf("htn: resume checkpoint: %w", err)
		}
	}

	// Classify task type if not already set.
	resolvedTask := task
	if task != nil && task.Type == "" {
		resolvedTask = &core.Task{
			ID:          task.ID,
			Type:        ClassifyTask(task),
			Instruction: task.Instruction,
			Context:     task.Context,
			Metadata:    task.Metadata,
		}
	}
	if surfaces.Workflow != nil {
		if retrievalPayload, err := workflowutil.Hydrate(ctx, surfaces.Workflow, workflowID, workflowutil.RetrievalQuery{
			Primary:   resolvedTask.Instruction,
			TaskText:  resolvedTask.Instruction,
			StepFiles: workflowutil.TaskPaths(resolvedTask),
		}, 4, 500); err != nil {
			return nil, fmt.Errorf("htn: retrieval hydrate failed: %w", err)
		} else if len(retrievalPayload) > 0 {
			runtime.PublishWorkflowRetrieval(state, retrievalPayload, true)
			resolvedTask = workflowutil.ApplyTaskRetrieval(resolvedTask, retrievalPayload)
		}
	}
	runtime.PublishTaskState(state, resolvedTask)

	// Find matching method.
	method := a.Methods.Find(resolvedTask)
	if method == nil {
		// No method — delegate directly to primitive executor.
		runtime.PublishResolvedMethodState(state, nil)
		runtime.PublishTerminationState(state, "completed")
		return a.delegateToPrimitive(ctx, resolvedTask, state)
	}
	resolvedMethod := ResolveMethod(*method)
	runtime.PublishResolvedMethodState(state, &resolvedMethod)

	// Decompose into a plan using resolved method (includes operator metadata).
	plan, err := DecomposeResolved(resolvedTask, &resolvedMethod)
	if err != nil {
		return nil, fmt.Errorf("htn: decomposition failed: %w", err)
	}

	// Run preflight to check required capabilities.
	preflightReport, preflightErr := runtime.PlanPreflight(plan, a.Tools)
	runtime.PublishPreflightState(state, preflightReport, preflightErr)
	if preflightErr != nil {
		return nil, fmt.Errorf("htn: %w", preflightErr)
	}

	runtime.PublishPlanState(state, plan)
	executionState := runtime.LoadExecutionState(state)
	executionState.WorkflowID = workflowID
	executionState.RunID = runID
	runtime.PublishExecutionState(state, executionState)

	// Execute via plan_executor.
	stepIndexes := make(map[string]int, len(plan.Steps))
	for idx, step := range plan.Steps {
		stepIndexes[step.ID] = idx
	}
	var checkpointStore *agentpipeline.SQLitePipelineCheckpointStore
	if surfaces.Workflow != nil && workflowID != "" && runID != "" {
		checkpointStore = agentpipeline.NewSQLitePipelineCheckpointStore(surfaces.Workflow, workflowID, runID)
	}
	executor := &graph.PlanExecutor{
		Options: graph.PlanExecutionOptions{
			BuildStepTask: a.buildPlanStepTask,
			MergeBranches: runtime.MergeHTNBranches,
			CompletedStepIDs: func(s *core.Context) []string {
				return runtime.CompletedStepsFromContext(s)
			},
			Recover: func(ctx context.Context, step core.PlanStep, stepTask *core.Task, s *core.Context, err error) (*graph.StepRecovery, error) {
				diagnosis := fmt.Sprintf("retrying step %q after failure: %v", step.ID, err)
				notes := []string{fmt.Sprintf("step %q failed with: %v", step.ID, err)}
				s.Set(runtime.ContextKeyLastRecoveryDiag, diagnosis)
				s.Set(runtime.ContextKeyLastFailureStep, step.ID)
				if err != nil {
					s.Set(runtime.ContextKeyLastFailureError, err.Error())
				}
				s.Set(runtime.ContextKeyLastRecoveryNotes, notes)
				return &graph.StepRecovery{Diagnosis: diagnosis, Notes: notes}, nil
			},
			AfterStep: func(step core.PlanStep, s *core.Context, result *core.Result) {
				a.afterStep(ctx, step, s, result, checkpointStore, stepIndexes, surfaces.Workflow, workflowID, runID, resolvedTask)
			},
		},
	}

	primitiveAgent := runtime.NewPrimitiveDispatcher(a.Tools, a.primitiveAgent())
	if surfaces.Workflow != nil || surfaces.Runtime != nil {
		primitiveAgent = &recordingPrimitiveAgent{
			delegate:   primitiveAgent,
			runtime:    surfaces.Runtime,
			workflow:   surfaces.Workflow,
			workflowID: workflowID,
			runID:      runID,
		}
	}
	startTime := time.Now()
	result, err := executor.Execute(ctx, primitiveAgent, resolvedTask, plan, state)
	executionDuration := time.Since(startTime)
	if err != nil {
		if surfaces.Workflow != nil && workflowID != "" && runID != "" {
			_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusFailed, timePtr(time.Now().UTC()))
		}
		return nil, fmt.Errorf("htn: plan execution failed: %w", err)
	}
	if surfaces.Workflow != nil && workflowID != "" && runID != "" {
		_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusCompleted, timePtr(time.Now().UTC()))
	}
	completed := core.StringSliceFromContext(state, "plan.completed_steps")
	executionState = runtime.LoadExecutionState(state)
	executionState.WorkflowID = workflowID
	executionState.RunID = runID
	executionState.CompletedSteps = append([]string(nil), completed...)
	executionState.CompletedStepCount = len(completed)
	if plan != nil {
		executionState.PlannedStepCount = len(plan.Steps)
	}
	runtime.PublishExecutionState(state, executionState)
	runtime.PublishTerminationState(state, "completed")

	// Phase 9: Persist framework-native artifacts and metrics.
	if surfaces.Workflow != nil && workflowID != "" && runID != "" {
		success := result != nil && result.Success
		_ = a.persistHTNRunSummary(ctx, state, surfaces.Workflow, workflowID, runID, startTime, success, nil)
		_ = a.persistHTNMethodMetadata(ctx, state, surfaces.Workflow, workflowID, runID)
		_ = a.persistHTNExecutionMetrics(ctx, state, surfaces.Workflow, workflowID, runID, time.Second, executionDuration)
	}
	compactHTNCheckpointState(state)
	return result, nil
}

func (a *HTNAgent) buildPlanStepTask(parentTask *core.Task, plan *core.Plan, step core.PlanStep, state *core.Context) *core.Task {
	stepTask := core.CloneTask(parentTask)
	if stepTask == nil {
		stepTask = &core.Task{}
	}
	if stepTask.Context == nil {
		stepTask.Context = map[string]any{}
	}
	// NEW: Pass parent state to step task for shared context access
	// This prevents React from re-discovering workspace for each step
	if state != nil {
		stepTask.Context["parent_state"] = state
	}
	stepTask.Context["current_step"] = step
	if plan != nil && strings.TrimSpace(plan.Goal) != "" {
		stepTask.Context["plan_goal"] = plan.Goal
	}
	// Bind operator metadata from step params onto the step task.
	if step.Params != nil {
		if taskType, ok := step.Params["operator_task_type"].(string); ok && taskType != "" {
			stepTask.Type = core.TaskType(taskType)
		}
		stepTask.Context["operator_task_type"] = step.Params["operator_task_type"]
		stepTask.Context["operator_executor"] = step.Params["operator_executor"]
		stepTask.Context["operator_name"] = step.Params["operator_name"]
		if caps, ok := step.Params["required_capabilities"]; ok {
			stepTask.Context["required_capabilities"] = caps
		}
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

// afterStep is called by the PlanExecutor after each step completes. It syncs
// completed-step tracking, saves a pipeline checkpoint, and persists the
// operator outcome to the workflow store.
func (a *HTNAgent) afterStep(
	ctx context.Context,
	step core.PlanStep,
	state *core.Context,
	result *core.Result,
	checkpointStore *agentpipeline.SQLitePipelineCheckpointStore,
	stepIndexes map[string]int,
	wfStore memory.WorkflowStateStore,
	workflowID, runID string,
	task *core.Task,
) {
	completed := runtime.CompletedStepsFromContext(state)
	if !containsStepID(completed, step.ID) {
		completed = append(completed, step.ID)
	}
	state.Set("plan.completed_steps", completed)
	execution := runtime.LoadExecutionState(state)
	execution.CompletedSteps = append([]string(nil), completed...)
	runtime.PublishExecutionState(state, execution)
	if checkpointStore != nil {
		_ = checkpointStore.Save(&frameworkpipeline.Checkpoint{
			CheckpointID: fmt.Sprintf("htn_%s_%d", step.ID, time.Now().UnixNano()),
			TaskID:       taskID(task),
			StageName:    step.ID,
			StageIndex:   stepIndexes[step.ID],
			CreatedAt:    time.Now().UTC(),
			Context:      state.Clone(),
			Result: frameworkpipeline.StageResult{
				StageName:     step.ID,
				DecodedOutput: resultData(result),
				ValidationOK:  result != nil && result.Success,
				ErrorText:     resultErrorText(result),
				Transition: frameworkpipeline.StageTransition{
					Kind: frameworkpipeline.TransitionNext,
				},
			},
		})
	}
	if wfStore != nil && workflowID != "" && runID != "" {
		operatorName := step.Tool
		if step.Tool == "" {
			operatorName = step.ID
		}
		success := result != nil && result.Success
		var outputKeys []string
		if result != nil && result.Data != nil {
			for k := range result.Data {
				outputKeys = append(outputKeys, k)
			}
		}
		stepRunID := fmt.Sprintf("%s_%d", step.ID, time.Now().UnixNano())
		_ = a.persistOperatorOutcome(ctx, wfStore, workflowID, runID, stepRunID, operatorName, step.ID, 0, success, outputKeys, nil)
	}
}

// delegateToPrimitive passes the task through the capability dispatcher.
func (a *HTNAgent) delegateToPrimitive(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	return runtime.DispatchTask(ctx, a.Tools, a.primitiveAgent(), task, state)
}

// primitiveAgent returns the configured primitive executor or a no-op fallback.
func (a *HTNAgent) primitiveAgent() graph.WorkflowExecutor {
	if a.PrimitiveExec != nil {
		return a.PrimitiveExec
	}
	return &noopAgent{}
}

func containsStepID(values []string, stepID string) bool {
	for _, value := range values {
		if value == stepID {
			return true
		}
	}
	return false
}

// noopAgent is a stand-in primitive executor that immediately succeeds. It is
// used in tests that want to exercise HTN decomposition without a real LLM.
type noopAgent struct{}

func (n *noopAgent) Initialize(_ *core.Config) error { return nil }
func (n *noopAgent) Capabilities() []core.Capability { return nil }
func (n *noopAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("noop_done")
	_ = g.AddNode(done)
	_ = g.SetStart("noop_done")
	return g, nil
}
func (n *noopAgent) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	return &core.Result{Success: true, Data: map[string]any{}}, nil
}

func taskID(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ID)
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func (a *HTNAgent) resumeCheckpoint(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, runID string, task *core.Task, state *core.Context) error {
	if store == nil || task == nil || strings.TrimSpace(task.ID) == "" {
		return nil
	}
	ids, err := store.ListPipelineCheckpoints(ctx, task.ID)
	if err != nil || len(ids) == 0 {
		return err
	}
	checkpoint, err := agentpipeline.NewSQLitePipelineCheckpointStore(store, workflowID, runID).Load(task.ID, ids[0])
	if err != nil {
		return err
	}
	if checkpoint == nil || checkpoint.Context == nil {
		return nil
	}
	state.Merge(checkpoint.Context)
	state.Set("htn.resume_checkpoint_id", checkpoint.CheckpointID)
	// Validate the merged checkpoint state to catch stale/invalid completed steps.
	if _, loaded, err := runtime.LoadStateFromContext(state); loaded && err != nil {
		return fmt.Errorf("invalid checkpoint state: %w", err)
	}
	return nil
}

func resultData(result *core.Result) any {
	if result == nil || len(result.Data) == 0 {
		return nil
	}
	return result.Data
}

func resultErrorText(result *core.Result) string {
	if result == nil || result.Success {
		return ""
	}
	return "step failed"
}
