package htn

import (
	"context"
	"fmt"
	"strings"

	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/htn/runtime"
	pl "codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	execctx "codeburg.org/lexbit/relurpify/execution/context"
	"codeburg.org/lexbit/relurpify/model"
)

// RuntimeSurfaces holds runtime surface references for workflow operations.
type RuntimeSurfaces struct {
	Workflow any
}

// RetrievalQuery defines parameters for retrieval operations.
type RetrievalQuery struct {
	StepFiles []string
}

// ResolveRuntimeSurfaces resolves runtime surfaces from a memory store.
func ResolveRuntimeSurfaces(mem any) RuntimeSurfaces {
	return RuntimeSurfaces{}
}

// Hydrate retrieves workflow retrieval data from the store and returns it as a map.
func Hydrate(ctx context.Context, surface any, workflowID string, query RetrievalQuery) (any, error) {
	return map[string]any{
		"workflow_id": workflowID,
		"step_files":  query.StepFiles,
	}, nil
}

// TaskPaths extracts file paths from task metadata.
func TaskPaths(task *execution.Task) []string {
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
func ApplyTaskRetrieval(task *execution.Task, payload any) *execution.Task {
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
	Model model.LanguageModel
	// Tools is the capability registry passed to the primitive executor.
	Tools *capability.CapabilityRegistry
	// Config holds runtime configuration.
	Config *execution.Config
	// Methods is the method library. Defaults to NewMethodLibrary() when nil.
	Methods *runtime.MethodLibrary
	// PrimitiveExec is the executor used for leaf subtasks.
	// It must be initialised before Execute is called.
	// When nil, HTNAgent falls back to a no-op that marks steps successful.
	PrimitiveExec agentgraph.WorkflowExecutor

	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int
	// StreamTrigger is the compiler trigger wired into the run context before
	// graph construction. A missing trigger skips context streaming entirely
	// (disabled feature, not a failure).
	StreamTrigger *contextstream.Trigger

	// authoredMethod is a recipe-authored decomposition (Wave 3 D4). When set,
	// Execute bypasses ClassifyTask and the method library entirely and
	// decomposes to exactly the authored tasks, in declaration order, with zero
	// LLM decomposition calls. Nil preserves the library lookup path (FR-9).
	authoredMethod *runtime.Method

	initialised bool

	// SemanticContext is the pre-resolved semantic context bundle passed
	// to the agent at construction time. It propagates to PrimitiveExec
	// when that executor is a *react.ReActAgent.
	SemanticContext execctx.AgentSemanticContext
}

// AuthoredTask is one recipe-authored HTN task (D4): its text is the sub-goal,
// and an optional capability pins the tool the task dispatches to. A task
// without a capability runs the primitive executor with the text as the goal.
type AuthoredTask struct {
	Text       string
	Capability string
}

// authoredTaskType is the synthetic task type of an authored method. Authored
// methods bypass the method library, so the type is provenance only; it exists
// because the runtime contract requires a non-empty method and subtask type.
const authoredTaskType execution.TaskType = "htn.authored"

// buildAuthoredMethod lowers recipe-authored tasks into a runtime method. The
// returned method is validated against the same runtime contract as a library
// method, so a spec-invalid authored task is an option-construction error.
func buildAuthoredMethod(name string, tasks []AuthoredTask) (*runtime.Method, error) {
	methodName := strings.TrimSpace(name)
	if methodName == "" {
		return nil, fmt.Errorf("htn method name required")
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("htn method %q requires at least one task", methodName)
	}
	subtasks := make([]runtime.SubtaskSpec, 0, len(tasks))
	var previous string
	for i, task := range tasks {
		text := strings.TrimSpace(task.Text)
		if text == "" {
			return nil, fmt.Errorf("htn task %d requires text", i+1)
		}
		executor := strings.TrimSpace(task.Capability)
		if executor == "" {
			executor = runtime.ExecutorReact
		}
		name := fmt.Sprintf("t%d", i+1)
		spec := runtime.SubtaskSpec{
			Name:        name,
			Type:        authoredTaskType,
			Instruction: text,
			Executor:    executor,
		}
		// A linear dependency chain guarantees declaration-order execution: the
		// plan executor runs ready steps serially when exactly one is ready at
		// a time (a parallel-ready batch would use the primitive dispatcher's
		// branch path and scramble order).
		if previous != "" {
			spec.DependsOn = []string{previous}
		}
		subtasks = append(subtasks, spec)
		previous = name
	}
	method := &runtime.Method{
		Name:     methodName,
		TaskType: authoredTaskType,
		Subtasks: subtasks,
	}
	if err := method.Validate(); err != nil {
		return nil, err
	}
	return method, nil
}

// Initialize satisfies agentgraph.WorkflowExecutor. It wires configuration and ensures the
// method library is populated.
func (a *HTNAgent) Initialize(cfg *execution.Config) error {
	a.Config = cfg
	if a.Methods == nil {
		a.Methods = runtime.NewMethodLibrary()
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
func (a *HTNAgent) BuildGraph(ctx context.Context, task *execution.Task) (*agentgraph.Graph, error) {
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
func (a *HTNAgent) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if env == nil {
		env = contextdata.NewEnvelope("htn", "session")
	}

	// Classify task type if not already set (rule-based, never an LLM call).
	resolvedTask := task
	if task != nil && task.Type == "" {
		resolvedTask = &execution.Task{
			ID:          task.ID,
			Type:        string(runtime.ClassifyTask(task)),
			Instruction: task.Instruction,
			Context:     task.Context,
			Metadata:    task.Metadata,
		}
	}

	// Execute streaming trigger before method decomposition.
	if a.StreamTrigger != nil {
		ctx = contextstream.WithTrigger(ctx, a.StreamTrigger)
	}
	if err := a.executeStreamingTrigger(ctx, resolvedTask, env); err != nil {
		return nil, fmt.Errorf("htn: streaming trigger failed: %w", err)
	}

	// Authored methods bypass the method library entirely (D4): the ingested
	// decomposition is exactly the decomposition, with zero LLM calls and a
	// resolved method that validates against the same runtime contract.
	if a.authoredMethod != nil {
		resolved := runtime.ResolveMethod(*a.authoredMethod)
		if err := resolved.Validate(); err != nil {
			a.planFailed(ctx, resolvedTask, err)
			return nil, fmt.Errorf("htn: authored method invalid: %w", err)
		}
		return a.executeResolvedMethod(ctx, resolvedTask, &resolved, env, true)
	}

	// Find matching method from the library.
	method := a.Methods.Find(resolvedTask)
	if method == nil {
		// No method — delegate directly to the primitive executor.
		return a.delegateToPrimitive(ctx, resolvedTask, env)
	}
	resolvedMethod := runtime.ResolveMethod(*method)
	return a.executeResolvedMethod(ctx, resolvedTask, &resolvedMethod, env, false)
}

// executeResolvedMethod is the shared post-decomposition execution: preflight,
// the plan-start lifecycle, the PlanExecutor over the primitive dispatcher, and
// the execution-completed lifecycle. Authored methods additionally surface the
// result contract (method, tasks_completed, tasks_total). Completed-step
// resume flows through plan.completed_steps unchanged.
func (a *HTNAgent) executeResolvedMethod(ctx context.Context, task *execution.Task, resolved *runtime.ResolvedMethod, env *contextdata.Envelope, authored bool) (*execution.Result, error) {
	compiledPlan, err := runtime.DecomposeResolved(task, resolved)
	if err != nil {
		a.planFailed(ctx, task, err)
		return nil, fmt.Errorf("htn: decomposition failed: %w", err)
	}

	// Run preflight to check required capabilities.
	preflightReport, preflightErr := runtime.PlanPreflight(compiledPlan, a.Tools)
	if preflightErr != nil {
		a.planFailed(ctx, task, preflightErr)
		return nil, fmt.Errorf("htn: %w", preflightErr)
	}
	_ = preflightReport
	a.planStarted(ctx, task, compiledPlan)

	// Execute via plan_executor.
	stepIndexes := make(map[string]int, len(compiledPlan.Steps))
	for idx, step := range compiledPlan.Steps {
		stepIndexes[step.ID] = idx
	}
	executor := &pl.PlanExecutor{
		Options: pl.PlanExecutionOptions{
			BuildStepTask:    a.buildPlanStepTask,
			MergeBranches:    runtime.MergeHTNBranches,
			CompletedStepIDs: runtime.CompletedStepsFromEnvelope,
			BeforeStep: func(step pl.PlanStep, _ *execution.Task, _ *contextdata.Envelope) {
				a.stepStarted(ctx, step)
			},
			Recover: func(ctx context.Context, step pl.PlanStep, stepTask *execution.Task, s *contextdata.Envelope, err error) (*pl.StepRecovery, error) {
				diagnosis := fmt.Sprintf("retrying step %q after failure: %v", step.ID, err)
				notes := []string{fmt.Sprintf("step %q failed with: %v", step.ID, err)}
				s.SetWorkingValueWithClass(runtime.ContextKeyLastRecoveryDiag, diagnosis, contextdata.MemoryClassTask)
				s.SetWorkingValueWithClass(runtime.ContextKeyLastFailureStep, step.ID, contextdata.MemoryClassTask)
				if err != nil {
					s.SetWorkingValueWithClass(runtime.ContextKeyLastFailureError, err.Error(), contextdata.MemoryClassTask)
				}
				s.SetWorkingValueWithClass(runtime.ContextKeyLastRecoveryNotes, notes, contextdata.MemoryClassTask)
				return &pl.StepRecovery{Diagnosis: diagnosis, Notes: notes}, nil
			},
			AfterStep: func(step pl.PlanStep, s *contextdata.Envelope, result *execution.Result) {
				a.afterStep(ctx, step, s, result, nil, stepIndexes, nil, "", "", task)
			},
		},
	}

	primitiveAgent := runtime.NewPrimitiveDispatcher(a.Tools, a.primitiveAgent())
	result, err := executor.Execute(ctx, primitiveAgent, task, compiledPlan, env)
	if err != nil {
		a.executionCompleted(ctx, task, false, 0, len(compiledPlan.Steps))
		return nil, fmt.Errorf("htn: plan execution failed: %w", err)
	}

	completed := env.StringSliceFromContext("plan.completed_steps")
	if completed == nil {
		completed = []string{}
	}
	a.executionCompleted(ctx, task, result != nil && result.Success, len(completed), len(compiledPlan.Steps))
	if !authored {
		return result, nil
	}

	// Authored result contract: method, tasks_completed, tasks_total (D4).
	fields := execution.ResultFields(result.Data)
	if fields == nil {
		fields = map[string]any{}
	}
	fields["method"] = resolved.Method.Name
	fields["tasks_completed"] = len(completed)
	fields["tasks_total"] = len(compiledPlan.Steps)
	env.SetWorkingValueWithClass("htn.method", resolved.Method.Name, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("htn.tasks_completed", len(completed), contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("htn.tasks_total", len(compiledPlan.Steps), contextdata.MemoryClassTask)
	result.Data = execution.NewToolResultPayload(fields)
	return result, nil
}

func (a *HTNAgent) buildPlanStepTask(parentTask *execution.Task, compiledPlan *pl.Plan, step pl.PlanStep, env *contextdata.Envelope) *execution.Task {
	stepTask := &execution.Task{
		ID:          parentTask.ID,
		Type:        parentTask.Type,
		Instruction: parentTask.Instruction,
		Context:     map[string]any{},
		Metadata:    parentTask.Metadata,
	}
	// Pass parent state to the step task for shared context access.
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
	result *execution.Result,
	checkpointStore any,
	stepIndexes map[string]int,
	wfStore any,
	workflowID, runID string,
	task *execution.Task,
) {
	a.stepCompleted(ctx, step)
	completed := runtime.CompletedStepsFromEnvelope(env)
	if !containsStepID(completed, step.ID) {
		completed = append(completed, step.ID)
	}
	env.SetWorkingValueWithClass("plan.completed_steps", completed, contextdata.MemoryClassTask)
	// Agent-specific execution state loading
	// execution := runtime.LoadExecutionState(env)
	// execution.CompletedSteps = append([]string(nil), completed...)
	// runtime.PublishExecutionState(env, execution)
	// Checkpoint saving is handled by the repository-backed persistence layer.
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
func (a *HTNAgent) delegateToPrimitive(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
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

func (n *noopAgent) Initialize(_ *execution.Config) error { return nil }
func (n *noopAgent) Capabilities() []string               { return nil }
func (n *noopAgent) BuildGraph(ctx context.Context, _ *execution.Task) (*agentgraph.Graph, error) {
	g := agentgraph.NewGraph()
	done := agentgraph.NewTerminalNode("noop_done")
	_ = g.AddNode(done)
	_ = g.SetStart("noop_done")
	return g, nil
}
func (n *noopAgent) Execute(_ context.Context, _ *execution.Task, _ *contextdata.Envelope) (*execution.Result, error) {
	return &execution.Result{Success: true, Data: execution.NewToolResultPayload(map[string]any{})}, nil
}

// streamMode returns the streaming mode, defaulting to blocking.
func (a *HTNAgent) streamMode() contextstream.Mode {
	if a.StreamMode != "" {
		return a.StreamMode
	}
	return contextstream.ModeBlocking
}

// streamQuery returns the query for streaming, defaulting to task instruction.
func (a *HTNAgent) streamQuery(task *execution.Task) string {
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
func (a *HTNAgent) streamTriggerNode(task *execution.Task) agentgraph.Node {
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
func (a *HTNAgent) executeStreamingTrigger(ctx context.Context, task *execution.Task, env *contextdata.Envelope) error {
	if contextstream.TriggerFromContext(ctx) == nil {
		return nil
	}
	node := a.streamTriggerNode(task)
	if node == nil {
		return nil
	}
	// Execute the stream node directly
	_, err := node.Execute(ctx, env)
	return err
}
