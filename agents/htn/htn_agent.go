package htn

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/agents/htn/runtime"
	pl "codeburg.org/lexbit/relurpify/agents/plan"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// RuntimeSurfaces holds runtime surface references for workflow operations.
type RuntimeSurfaces struct {
	// TODO: Replace with agentlifecycle.Repository
	// per the agentlifecycle workflow-store removal plan
	Workflow interface{}
}

// RetrievalQuery defines parameters for retrieval operations.
type RetrievalQuery struct {
	StepFiles []string
}

// ResolveRuntimeSurfaces resolves runtime surfaces from a memory store.
// TODO: Reimplement without WorkflowStateStore dependency
// per the agentlifecycle workflow-store removal plan
func ResolveRuntimeSurfaces(mem interface{}) RuntimeSurfaces {
	return RuntimeSurfaces{}
}

// Hydrate retrieves workflow retrieval data from the store and returns it as a map.
// This replaces the workflowutil.Hydrate stub with a real implementation.
// TODO: Reimplement without WorkflowStateStore dependency
// per the agentlifecycle workflow-store removal plan
func Hydrate(ctx context.Context, surface interface{}, workflowID string, query RetrievalQuery) (interface{}, error) {
	// Placeholder - retrieval to be reimplemented
	// using agentlifecycle.Repository
	return map[string]any{
		"workflow_id": workflowID,
		"step_files":  query.StepFiles,
	}, nil
}

// TaskPaths extracts file paths from task metadata.
func TaskPaths(task *core.Task) []string {
	if task == nil || task.Metadata == nil {
		return nil
	}
	// Look for file paths in task metadata
	if paths, ok := task.Metadata["files"].([]string); ok {
		return paths
	}
	return nil
}

// ApplyTaskRetrieval applies retrieval payload to task context.
func ApplyTaskRetrieval(task *core.Task, payload interface{}) *core.Task {
	if task == nil || payload == nil {
		return task
	}
	if task.Context == nil {
		task.Context = make(map[string]any)
	}
	task.Context["workflow_retrieval"] = payload
	return task
}

// HTNAgent implements agentgraph.WorkflowExecutor using a Hierarchical Task Network (HTN)
// planning approach. Complex tasks are decomposed into primitive subtasks by
// the method library; a primitive executor (default: any agentgraph.WorkflowExecutor) then
// runs each leaf step.
//
// The agent is small-model-friendly: the LLM never decides how to structure
// work, it only executes focused, narrowly-scoped subtasks.
type HTNAgent struct {
	// Model is the language model used by the primitive executor.
	Model contracts.LanguageModel
	// Tools is the capability registry passed to the primitive executor.
	Tools *capability.Registry
	// Config holds runtime configuration.
	Config *core.Config
	// Methods is the method library. Defaults to NewMethodLibrary() when nil.
	Methods *MethodLibrary
	// PrimitiveExec is the executor used for leaf subtasks.
	// It must be initialised before Execute is called.
	// When nil, HTNAgent falls back to a no-op that marks steps successful.
	PrimitiveExec agentgraph.WorkflowExecutor

	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int

	initialised bool

	// SemanticContext is the pre-resolved semantic context bundle passed
	// to the agent at construction time. It propagates to PrimitiveExec
	// when that executor is a *react.ReActAgent.
	SemanticContext agentspec.AgentSemanticContext
}

// Initialize satisfies agentgraph.WorkflowExecutor. It wires configuration and ensures the
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
func (a *HTNAgent) Capabilities() []string {
	return []string{"htn"}
}

// BuildGraph returns a minimal single-node graph suitable for agenttest and
// visualisation. HTN execution is driven by Execute, not a static graph walk.
func (a *HTNAgent) BuildGraph(task *core.Task) (*agentgraph.Graph, error) {
	g := agentgraph.NewGraph()
	done := agentgraph.NewTerminalNode("htn_done")
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
func (a *HTNAgent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if env == nil {
		env = contextdata.NewEnvelope("htn", "session")
	}
	workflowID := ""
	runID := ""

	surfaces := ResolveRuntimeSurfaces(nil)

	// Classify task type if not already set.
	resolvedTask := task
	if task != nil && task.Type == "" {
		resolvedTask = &core.Task{
			ID:          task.ID,
			Type:        string(ClassifyTask(task)),
			Instruction: task.Instruction,
			Context:     task.Context,
			Metadata:    task.Metadata,
		}
	}
	if surfaces.Workflow != nil {
		if retrievalPayload, err := Hydrate(ctx, surfaces.Workflow, workflowID, RetrievalQuery{
			StepFiles: TaskPaths(resolvedTask),
		}); err != nil {
			return nil, fmt.Errorf("htn: retrieval hydrate failed: %w", err)
		} else if retrievalPayload != nil {
			// Agent-specific runtime state publishing
			// runtime.PublishWorkflowRetrieval(env, retrievalPayload, true)
			resolvedTask = ApplyTaskRetrieval(resolvedTask, retrievalPayload)
		}
	}
	// Agent-specific task state publishing
	// runtime.PublishTaskState(env, resolvedTask)

	// Execute streaming trigger before method decomposition
	if err := a.executeStreamingTrigger(ctx, resolvedTask, env); err != nil {
		return nil, fmt.Errorf("htn: streaming trigger failed: %w", err)
	}

	// Find matching method.
	method := a.Methods.Find(resolvedTask)
	if method == nil {
		// No method — delegate directly to primitive executor.
		// Agent-specific method state publishing
		// runtime.PublishResolvedMethodState(env, nil)
		// runtime.PublishTerminationState(env, "completed")
		return a.delegateToPrimitive(ctx, resolvedTask, env)
	}
	resolvedMethod := ResolveMethod(*method)
	// Agent-specific method state publishing
	// runtime.PublishResolvedMethodState(env, &resolvedMethod)

	// Decompose into a plan using resolved method (includes operator metadata).
	compiledPlan, err := DecomposeResolved(resolvedTask, &resolvedMethod)
	if err != nil {
		return nil, fmt.Errorf("htn: decomposition failed: %w", err)
	}

	// Run preflight to check required capabilities.
	preflightReport, preflightErr := runtime.PlanPreflight(compiledPlan, a.Tools)
	// Agent-specific preflight state publishing
	// runtime.PublishPreflightState(env, preflightReport, preflightErr)
	if preflightErr != nil {
		return nil, fmt.Errorf("htn: %w", preflightErr)
	}
	_ = preflightReport

	// Agent-specific plan state publishing
	// runtime.PublishPlanState(env, compiledPlan)
	// Agent-specific execution state loading
	// executionState := runtime.LoadExecutionState(env)
	executionState := runtime.ExecutionState{}
	executionState.WorkflowID = workflowID
	executionState.RunID = runID
	// Agent-specific execution state publishing
	// runtime.PublishExecutionState(env, executionState)

	// Execute via plan_executor.
	stepIndexes := make(map[string]int, len(compiledPlan.Steps))
	for idx, step := range compiledPlan.Steps {
		stepIndexes[step.ID] = idx
	}
	var checkpointStore any
	executor := &pl.PlanExecutor{
		Options: pl.PlanExecutionOptions{
			BuildStepTask: a.buildPlanStepTask,
			MergeBranches: runtime.MergeHTNBranches,
			CompletedStepIDs: func(s *contextdata.Envelope) []string {
				return runtime.CompletedStepsFromEnvelope(s)
			},
			Recover: func(ctx context.Context, step pl.PlanStep, stepTask *core.Task, s *contextdata.Envelope, err error) (*pl.StepRecovery, error) {
				diagnosis := fmt.Sprintf("retrying step %q after failure: %v", step.ID, err)
				notes := []string{fmt.Sprintf("step %q failed with: %v", step.ID, err)}
				s.SetWorkingValue(runtime.ContextKeyLastRecoveryDiag, diagnosis, contextdata.MemoryClassTask)
				s.SetWorkingValue(runtime.ContextKeyLastFailureStep, step.ID, contextdata.MemoryClassTask)
				if err != nil {
					s.SetWorkingValue(runtime.ContextKeyLastFailureError, err.Error(), contextdata.MemoryClassTask)
				}
				s.SetWorkingValue(runtime.ContextKeyLastRecoveryNotes, notes, contextdata.MemoryClassTask)
				return &pl.StepRecovery{Diagnosis: diagnosis, Notes: notes}, nil
			},
			AfterStep: func(step pl.PlanStep, s *contextdata.Envelope, result *pl.Result) {
				a.afterStep(ctx, step, s, result, checkpointStore, stepIndexes, surfaces.Workflow, workflowID, runID, resolvedTask)
			},
		},
	}

	primitiveAgent := runtime.NewPrimitiveDispatcher(a.Tools, a.primitiveAgent())
	if surfaces.Workflow != nil {
		primitiveAgent = &recordingPrimitiveAgent{
			delegate:   primitiveAgent,
			workflow:   surfaces.Workflow,
			workflowID: workflowID,
			runID:      runID,
		}
	}
	startTime := time.Now()
	result, err := executor.Execute(ctx, primitiveAgent, resolvedTask, compiledPlan, env)
	_ = time.Since(startTime) // executionDuration - used when persistence is re-enabled
	if err != nil {
		// Agent-specific workflow status update
		// if surfaces.Workflow != nil && workflowID != "" && runID != "" {
		// 	_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusFailed, timePtr(time.Now().UTC()))
		// }
		return nil, fmt.Errorf("htn: plan execution failed: %w", err)
	}
	// Agent-specific workflow status update
	// if surfaces.Workflow != nil && workflowID != "" && runID != "" {
	// 	_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusCompleted, timePtr(time.Now().UTC()))
	// }
	// StringSliceFromContext - now available in envelope
	completed := env.StringSliceFromContext("plan.completed_steps")
	if completed == nil {
		completed = []string{}
	}
	// Agent-specific execution state loading
	// executionState = runtime.LoadExecutionState(env)
	executionState.WorkflowID = workflowID
	executionState.RunID = runID
	executionState.CompletedSteps = append([]string(nil), completed...)
	executionState.CompletedStepCount = len(completed)
	if compiledPlan != nil {
		executionState.PlannedStepCount = len(compiledPlan.Steps)
	}
	// Agent-specific execution state publishing
	// runtime.PublishExecutionState(env, executionState)
	// Agent-specific termination state publishing
	// runtime.PublishTerminationState(env, "completed")

	// Workflow store persistence disabled - memory package being rebuilt
	// if surfaces.Workflow != nil && workflowID != "" && runID != "" {
	// 	success := result != nil && result.Success
	// 	_ = a.persistHTNRunSummary(ctx, env, surfaces.Workflow, workflowID, runID, startTime, success, nil)
	// 	_ = a.persistHTNMethodMetadata(ctx, env, surfaces.Workflow, workflowID, runID)
	// 	_ = a.persistHTNExecutionMetrics(ctx, env, surfaces.Workflow, workflowID, runID, time.Second, executionDuration)
	// }
	// Agent-specific checkpoint state compaction
	// compactHTNCheckpointState(env)
	return result, nil
}

func (a *HTNAgent) buildPlanStepTask(parentTask *core.Task, compiledPlan *pl.Plan, step pl.PlanStep, env *contextdata.Envelope) *core.Task {
	stepTask := &core.Task{
		ID:          parentTask.ID,
		Type:        parentTask.Type,
		Instruction: parentTask.Instruction,
		Context:     map[string]any{},
		Metadata:    parentTask.Metadata,
	}
	// NEW: Pass parent state to step task for shared context access
	// This prevents React from re-discovering workspace for each step
	if env != nil {
		stepTask.Context["parent_state"] = env
	}
	stepTask.Context["current_step"] = step
	if compiledPlan != nil && strings.TrimSpace(compiledPlan.Goal) != "" {
		stepTask.Context["plan_goal"] = compiledPlan.Goal
	}
	// Bind step metadata onto the step task context
	stepTask.Context["step_id"] = step.ID
	stepTask.Context["step_description"] = step.Description
	stepTask.Context["step_files"] = step.Files
	stepTask.Context["step_expected"] = step.Expected
	stepTask.Context["step_verification"] = step.Verification
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
	step pl.PlanStep,
	env *contextdata.Envelope,
	result *pl.Result,
	checkpointStore any,
	stepIndexes map[string]int,
	wfStore interface{},
	workflowID, runID string,
	task *core.Task,
) {
	completed := runtime.CompletedStepsFromEnvelope(env)
	if !containsStepID(completed, step.ID) {
		completed = append(completed, step.ID)
	}
	env.SetWorkingValue("plan.completed_steps", completed, contextdata.MemoryClassTask)
	// Agent-specific execution state loading
	// execution := runtime.LoadExecutionState(env)
	// execution.CompletedSteps = append([]string(nil), completed...)
	// runtime.PublishExecutionState(env, execution)
	// Checkpoint saving disabled - pipeline store is a stub
	// if checkpointStore != nil {
	// 	_ = checkpointStore.Save(&frameworkpipeline.Checkpoint{
	// 		CheckpointID: fmt.Sprintf("htn_%s_%d", step.ID, time.Now().UnixNano()),
	// 		TaskID:       taskID(task),
	// 		StageName:    step.ID,
	// 		StageIndex:   stepIndexes[step.ID],
	// 		CreatedAt:    time.Now().UTC(),
	// 		Context:      env.Clone(),
	// 		Result: frameworkpipeline.StageResult{
	// 			StageName:     step.ID,
	// 			DecodedOutput: resultData(result),
	// 			ValidationOK:  result != nil && result.Success,
	// 			ErrorText:     resultErrorText(result),
	// 			Transition: frameworkpipeline.StageTransition{
	// 				Kind: frameworkpipeline.TransitionNext,
	// 			},
	// 		},
	// 	})
	// }
	// Workflow store persistence disabled - memory package being rebuilt
	// if wfStore != nil && workflowID != "" && runID != "" {
	// 	operatorName := step.Tool
	// 	if step.Tool == "" {
	// 		operatorName = step.ID
	// 	}
	// 	success := result != nil && result.Success
	// 	var outputKeys []string
	// 	if result != nil && result.Data != nil {
	// 		for k := range result.Data {
	// 			outputKeys = append(outputKeys, k)
	// 		}
	// 	}
	// 	stepRunID := fmt.Sprintf("%s_%d", step.ID, time.Now().UnixNano())
	// 	_ = a.persistOperatorOutcome(ctx, wfStore, workflowID, runID, stepRunID, operatorName, step.ID, 0, success, outputKeys, nil)
	// }
}

// delegateToPrimitive passes the task through the capability dispatcher.
func (a *HTNAgent) delegateToPrimitive(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	return runtime.DispatchTask(ctx, a.Tools, a.primitiveAgent(), task, env)
}

// primitiveAgent returns the configured primitive executor or a no-op fallback.
func (a *HTNAgent) primitiveAgent() agentgraph.WorkflowExecutor {
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
func (n *noopAgent) Capabilities() []string          { return nil }
func (n *noopAgent) BuildGraph(_ *core.Task) (*agentgraph.Graph, error) {
	g := agentgraph.NewGraph()
	done := agentgraph.NewTerminalNode("noop_done")
	_ = g.AddNode(done)
	_ = g.SetStart("noop_done")
	return g, nil
}
func (n *noopAgent) Execute(_ context.Context, _ *core.Task, _ *contextdata.Envelope) (*core.Result, error) {
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

// resumeCheckpoint is temporarily disabled - memory/db package being rebuilt
func (a *HTNAgent) resumeCheckpoint(ctx context.Context, store any, workflowID, runID string, task *core.Task, env *contextdata.Envelope) error {
	_ = ctx
	_ = store
	_ = workflowID
	_ = runID
	_ = task
	_ = env
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

// streamMode returns the streaming mode, defaulting to blocking.
func (a *HTNAgent) streamMode() contextstream.Mode {
	if a.StreamMode != "" {
		return a.StreamMode
	}
	return contextstream.ModeBlocking
}

// streamQuery returns the query for streaming, defaulting to task instruction.
func (a *HTNAgent) streamQuery(task *core.Task) string {
	if a.StreamQuery != "" {
		return a.StreamQuery
	}
	if task != nil {
		return task.Instruction
	}
	return ""
}

// streamMaxTokens returns the max tokens for streaming, defaulting to 256.
func (a *HTNAgent) streamMaxTokens() int {
	if a.StreamMaxTokens > 0 {
		return a.StreamMaxTokens
	}
	return 256
}

// streamTriggerNode creates a streaming trigger node for the HTN agent.
func (a *HTNAgent) streamTriggerNode(task *core.Task) agentgraph.Node {
	query := a.streamQuery(task)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	node := agentgraph.NewContextStreamNode("htn_stream", retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	node.BudgetShortfallPolicy = "emit_partial"
	node.Metadata = map[string]any{
		"agent": "htn",
		"stage": "pre_decomposition",
	}
	return node
}

// executeStreamingTrigger runs the streaming trigger before method decomposition.
func (a *HTNAgent) executeStreamingTrigger(ctx context.Context, task *core.Task, env *contextdata.Envelope) error {
	node := a.streamTriggerNode(task)
	if node == nil {
		return nil
	}
	// Execute the stream node directly
	_, err := node.Execute(ctx, env)
	return err
}
