package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/agents/internal/workflowutil"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextmgr"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworksearch "codeburg.org/lexbit/relurpify/framework/search"
)

// RewooAgent executes a strict plan/execute/synthesize workflow.
type RewooAgent struct {
	Model             core.LanguageModel
	Tools             *capability.Registry
	Memory            memory.MemoryStore
	Config            *core.Config
	Options           RewooOptions
	ContextPolicy     *contextmgr.ContextPolicy
	PermissionManager *authorization.PermissionManager
	IndexManager      *ast.IndexManager
	SearchEngine      *frameworksearch.SearchEngine
	Telemetry         core.Telemetry

	initialised bool
}

// debugf logs debug messages if Config.DebugAgent is enabled.
func (a *RewooAgent) debugf(format string, args ...interface{}) {
	if a == nil || a.Config == nil || !a.Config.DebugAgent {
		return
	}
	fmt.Printf("[rewoo] "+format+"\n", args...)
}

func (a *RewooAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityExplain,
	}
}

func (a *RewooAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("rewoo_done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	if err := g.SetStart(done.ID()); err != nil {
		return nil, err
	}
	return g, nil
}

func (a *RewooAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if state == nil {
		state = core.NewContext()
	}
	surfaces := workflowutil.ResolveRuntimeSurfaces(a.Memory)
	var workflowID, runID string
	if surfaces.Workflow != nil {
		var err error
		workflowID, runID, err = workflowutil.EnsureWorkflowRun(ctx, surfaces.Workflow, task, state, "rewoo")
		if err != nil {
			return nil, fmt.Errorf("rewoo: workflow init failed: %w", err)
		}
	}

	options := a.options()
	var executionCatalog *capability.ExecutionCapabilityCatalogSnapshot
	if a.Tools != nil {
		executionCatalog = a.Tools.CaptureExecutionCatalogSnapshot()
	}

	// Set up context management
	state.SetExecutionPhase(string(PhasePlan))
	var sharedContext *core.SharedContext
	if a.ContextPolicy != nil {
		// Initialize context manager once at the start
		if err := a.ContextPolicy.InitialLoad(task); err != nil {
			a.debugf("context initial load failed: %v", err)
			// Non-fatal: continue without progressive loading
		}
		sharedContext = core.NewSharedContext(state, a.ContextPolicy.Budget, a.ContextPolicy.Summarizer)
	}

	if surfaces.Workflow != nil {
		if retrievalPayload, err := workflowutil.Hydrate(ctx, surfaces.Workflow, workflowID, workflowutil.RetrievalQuery{
			Primary:   taskInstruction(task),
			TaskText:  taskInstruction(task),
			StepFiles: workflowutil.TaskPaths(task),
		}, 4, 500); err != nil {
			return nil, fmt.Errorf("rewoo: retrieval hydrate failed: %w", err)
		} else if len(retrievalPayload) > 0 {
			workflowutil.ApplyState(state, "rewoo.workflow_retrieval", retrievalPayload)
			state.Set("rewoo.retrieval_applied", true)
			// Note: task augmentation for retrieval happens at the graph level
			// The graph node will apply retrieval context to planning
		}
	}

	// Phase 7: Create checkpoint store if workflow store is available
	var checkpointStore *RewooCheckpointStore
	if surfaces.Workflow != nil {
		checkpointStore = NewRewooCheckpointStore(surfaces.Workflow, a.debugf)
	}

	// Build the static graph once at the start
	// (Phase 6: Graph-based execution integration)
	// (Phase 7: With checkpoint nodes if checkpoint store is available)
	g, err := buildStaticGraphWithCheckpoints(
		a.Model,
		a.Tools,
		task,
		executionModelToolSpecs(a.Tools, executionCatalog),
		a.ContextPolicy,
		sharedContext,
		state,
		options,
		a.PermissionManager,
		checkpointStore,
		a.debugf,
	)
	if err != nil {
		if surfaces.Workflow != nil && runID != "" {
			_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusFailed, timePtr(time.Now().UTC()))
		}
		return nil, fmt.Errorf("rewoo: build graph failed: %w", err)
	}

	// Execute the graph (handles planning, execution, replan routing, and synthesis)
	// The graph manages all phase transitions and replan logic through conditional edges
	state.SetExecutionPhase(string(PhasePlan))
	if _, err := g.Execute(ctx, state); err != nil {
		if surfaces.Workflow != nil && runID != "" {
			_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusFailed, timePtr(time.Now().UTC()))
		}
		return nil, fmt.Errorf("rewoo: graph execution failed: %w", err)
	}

	// Extract results from state after graph completion
	var plan *RewooPlan
	if planVal, ok := state.Get("rewoo.plan"); ok {
		if p, ok := planVal.(*RewooPlan); ok {
			plan = p
		}
	}

	var results []RewooStepResult
	if resultsVal, ok := state.Get("rewoo.tool_results"); ok {
		if r, ok := resultsVal.([]RewooStepResult); ok {
			results = r
		}
	}

	synthesis := ""
	if synthVal, ok := state.Get("rewoo.synthesis"); ok {
		if s, ok := synthVal.(string); ok {
			synthesis = s
		}
	}

	// Persist final plan and results (for graph-based execution)
	if plan != nil {
		a.persistPlan(ctx, surfaces, workflowID, runID, plan, 0)
	}
	if len(results) > 0 {
		a.persistStepResults(ctx, surfaces, workflowID, task, plan, results, 0)
		if toolResultsRef := a.persistToolResultsArtifact(ctx, surfaces, workflowID, runID, results); toolResultsRef != nil {
			state.Set("rewoo.tool_results_ref", *toolResultsRef)
			state.Set("rewoo.tool_results", compactRewooToolResultsState(results))
		}
		state.Set("rewoo.tool_results_summary", summarizeRewooStepResults(results))
	}
	if synthesis != "" {
		if synthRef := a.persistSynthesis(ctx, surfaces, workflowID, runID, task, synthesis, results); synthRef != nil {
			state.Set("rewoo.synthesis_ref", *synthRef)
		}
	}
	if surfaces.Workflow != nil && runID != "" {
		_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusCompleted, timePtr(time.Now().UTC()))
	}

	stepsOK := 0
	for _, result := range results {
		if result.Success {
			stepsOK++
		}
	}
	return &core.Result{
		Success: true,
		Data: map[string]any{
			"synthesis": synthesis,
			"steps_run": len(results),
			"steps_ok":  stepsOK,
		},
	}, nil
}

func executionModelToolSpecs(registry *capability.Registry, snapshot *capability.ExecutionCapabilityCatalogSnapshot) []core.LLMToolSpec {
	if snapshot != nil {
		return snapshot.ModelCallableLLMToolSpecs()
	}
	if registry == nil {
		return nil
	}
	return registry.ModelCallableLLMToolSpecs()
}

func compactRewooToolResultsState(results []RewooStepResult) map[string]any {
	value := map[string]any{
		"step_count": len(results),
	}
	if len(results) == 0 {
		return value
	}
	steps := make([]map[string]any, 0, len(results))
	stepsOK := 0
	for _, result := range results {
		if result.Success {
			stepsOK++
		}
		steps = append(steps, map[string]any{
			"step_id": result.StepID,
			"tool":    result.Tool,
			"success": result.Success,
			"error":   result.Error,
		})
	}
	value["steps_ok"] = stepsOK
	value["steps"] = steps
	value["last_step"] = steps[len(steps)-1]
	return value
}

func (a *RewooAgent) options() RewooOptions {
	opts := a.Options
	if opts.MaxReplanAttempts < 0 {
		opts.MaxReplanAttempts = 0
	}
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 20
	}
	if opts.OnFailure == "" {
		opts.OnFailure = StepOnFailureSkip
	}
	return opts
}

type noopAgent struct{}

func (n *noopAgent) Initialize(_ *core.Config) error { return nil }
func (n *noopAgent) Capabilities() []core.Capability { return nil }
func (n *noopAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("noop_done")
	_ = g.AddNode(done)
	_ = g.SetStart(done.ID())
	return g, nil
}
func (n *noopAgent) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	return &core.Result{Success: true, Data: map[string]any{}}, nil
}

func (a *RewooAgent) persistPlan(ctx context.Context, surfaces workflowutil.RuntimeSurfaces, workflowID, runID string, plan *RewooPlan, attempt int) {
	if plan == nil {
		return
	}
	now := time.Now().UTC()
	if surfaces.Runtime != nil {
		payload, _ := json.Marshal(plan)
		_ = surfaces.Runtime.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
			RecordID:   fmt.Sprintf("rewoo_plan_%d", now.UnixNano()),
			Scope:      memory.MemoryScopeProject,
			Kind:       memory.DeclarativeMemoryKindDecision,
			Title:      "ReWOO plan",
			Content:    string(payload),
			Summary:    plan.Goal,
			WorkflowID: workflowID,
			CreatedAt:  now,
			UpdatedAt:  now,
			Tags:       []string{"agent:rewoo", fmt.Sprintf("attempt:%d", attempt)},
		})
	}
	if surfaces.Workflow != nil && workflowID != "" {
		payload, _ := json.Marshal(plan)
		_ = surfaces.Workflow.PutKnowledge(ctx, memory.KnowledgeRecord{
			RecordID:   fmt.Sprintf("rewoo_plan_%d", now.UnixNano()),
			WorkflowID: workflowID,
			Kind:       memory.KnowledgeKindDecision,
			Title:      "ReWOO execution plan",
			Content:    string(payload),
			Status:     "accepted",
			Metadata:   map[string]any{"run_id": runID, "attempt": attempt},
			CreatedAt:  now,
		})
	}
}

func (a *RewooAgent) persistStepResults(ctx context.Context, surfaces workflowutil.RuntimeSurfaces, workflowID string, task *core.Task, plan *RewooPlan, results []RewooStepResult, attempt int) {
	if len(results) == 0 {
		return
	}
	stepDescriptions := make(map[string]string, len(plan.Steps))
	for _, step := range plan.Steps {
		stepDescriptions[step.ID] = step.Description
	}
	now := time.Now().UTC()
	for idx, result := range results {
		summary := result.Error
		if summary == "" {
			summary = strings.TrimSpace(fmt.Sprint(result.Output))
		}
		if summary == "" {
			summary = "step completed"
		}
		if surfaces.Runtime != nil {
			kind := memory.DeclarativeMemoryKindFact
			if !result.Success {
				kind = memory.DeclarativeMemoryKindConstraint
			}
			_ = surfaces.Runtime.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
				RecordID:   fmt.Sprintf("rewoo_step_%d_%d", now.UnixNano(), idx),
				Scope:      memory.MemoryScopeProject,
				Kind:       kind,
				Title:      firstNonEmpty(stepDescriptions[result.StepID], result.StepID),
				Content:    summary,
				Summary:    summary,
				WorkflowID: workflowID,
				TaskID:     taskInstructionID(task),
				Verified:   result.Success,
				CreatedAt:  now,
				UpdatedAt:  now,
				Tags:       []string{"agent:rewoo", "step:" + result.StepID},
				Metadata: map[string]any{
					"tool":    result.Tool,
					"attempt": attempt,
					"success": result.Success,
				},
			})
		}
		if surfaces.Workflow != nil && workflowID != "" {
			kind := memory.KnowledgeKindFact
			title := "ReWOO step result"
			status := "accepted"
			if !result.Success {
				kind = memory.KnowledgeKindIssue
				title = "ReWOO step failure"
				status = "open"
			}
			_ = surfaces.Workflow.PutKnowledge(ctx, memory.KnowledgeRecord{
				RecordID:   fmt.Sprintf("rewoo_knowledge_%d_%d", now.UnixNano(), idx),
				WorkflowID: workflowID,
				StepID:     result.StepID,
				Kind:       kind,
				Title:      title,
				Content:    summary,
				Status:     status,
				Metadata: map[string]any{
					"tool":    result.Tool,
					"attempt": attempt,
				},
				CreatedAt: now,
			})
		}
	}
}

func (a *RewooAgent) persistReplanSignal(ctx context.Context, surfaces workflowutil.RuntimeSurfaces, workflowID, runID, replanContext string, attempt int) {
	if surfaces.Workflow == nil || workflowID == "" {
		return
	}
	_ = surfaces.Workflow.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    fmt.Sprintf("rewoo_replan_%d", time.Now().UnixNano()),
		WorkflowID: workflowID,
		RunID:      runID,
		EventType:  "replan_required",
		Message:    replanContext,
		Metadata:   map[string]any{"attempt": attempt},
		CreatedAt:  time.Now().UTC(),
	})
}

func (a *RewooAgent) persistSynthesis(ctx context.Context, surfaces workflowutil.RuntimeSurfaces, workflowID, runID string, task *core.Task, synthesis string, results []RewooStepResult) *core.ArtifactReference {
	if strings.TrimSpace(synthesis) == "" {
		return nil
	}
	now := time.Now().UTC()
	stepSummaries := make([]string, 0, len(results))
	for _, result := range results {
		status := "ok"
		if !result.Success {
			status = "failed"
		}
		summary := strings.TrimSpace(result.Error)
		if summary == "" {
			summary = strings.TrimSpace(fmt.Sprint(result.Output))
		}
		if summary == "" {
			summary = "step completed"
		}
		stepSummaries = append(stepSummaries, fmt.Sprintf("%s [%s]: %s", result.StepID, status, summary))
	}
	knowledgeContent := synthesis
	if len(stepSummaries) > 0 {
		knowledgeContent += "\n\nStep results:\n" + strings.Join(stepSummaries, "\n")
	}
	if surfaces.Runtime != nil {
		_ = surfaces.Runtime.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
			RecordID:   fmt.Sprintf("rewoo_synthesis_%d", now.UnixNano()),
			Scope:      memory.MemoryScopeProject,
			Kind:       memory.DeclarativeMemoryKindFact,
			Title:      "ReWOO synthesis",
			Content:    synthesis,
			Summary:    synthesis,
			WorkflowID: workflowID,
			TaskID:     taskInstructionID(task),
			Verified:   true,
			CreatedAt:  now,
			UpdatedAt:  now,
			Tags:       []string{"agent:rewoo", "artifact:synthesis"},
		})
	}
	if surfaces.Workflow != nil && workflowID != "" {
		_ = surfaces.Workflow.PutKnowledge(ctx, memory.KnowledgeRecord{
			RecordID:   fmt.Sprintf("rewoo_synthesis_%d", now.UnixNano()),
			WorkflowID: workflowID,
			Kind:       memory.KnowledgeKindFact,
			Title:      "ReWOO synthesis",
			Content:    knowledgeContent,
			Status:     "accepted",
			Metadata: map[string]any{
				"run_id":       runID,
				"step_results": results,
			},
			CreatedAt: now,
		})
		payload, _ := json.Marshal(map[string]any{
			"synthesis":    synthesis,
			"step_results": results,
		})
		artifact := memory.WorkflowArtifactRecord{
			ArtifactID:        fmt.Sprintf("rewoo_synthesis_%d", now.UnixNano()),
			WorkflowID:        workflowID,
			RunID:             runID,
			Kind:              "rewoo_synthesis",
			ContentType:       "application/json",
			StorageKind:       memory.ArtifactStorageInline,
			SummaryText:       synthesis,
			SummaryMetadata:   map[string]any{"agent": "rewoo", "run_id": runID},
			InlineRawText:     string(payload),
			RawSizeBytes:      int64(len(payload)),
			CompressionMethod: "none",
			CreatedAt:         now,
		}
		if err := surfaces.Workflow.UpsertWorkflowArtifact(ctx, artifact); err == nil {
			ref := workflowutil.WorkflowArtifactReference(artifact)
			return &ref
		}
	}
	return nil
}

func (a *RewooAgent) persistToolResultsArtifact(ctx context.Context, surfaces workflowutil.RuntimeSurfaces, workflowID, runID string, results []RewooStepResult) *core.ArtifactReference {
	if surfaces.Workflow == nil || strings.TrimSpace(workflowID) == "" || len(results) == 0 {
		return nil
	}
	now := time.Now().UTC()
	payload, _ := json.Marshal(results)
	artifact := memory.WorkflowArtifactRecord{
		ArtifactID:        fmt.Sprintf("rewoo_results_%d", now.UnixNano()),
		WorkflowID:        workflowID,
		RunID:             runID,
		Kind:              "rewoo_tool_results",
		ContentType:       "application/json",
		StorageKind:       memory.ArtifactStorageInline,
		SummaryText:       summarizeRewooStepResults(results),
		SummaryMetadata:   map[string]any{"agent": "rewoo", "run_id": runID, "result_count": len(results)},
		InlineRawText:     string(payload),
		RawSizeBytes:      int64(len(payload)),
		CompressionMethod: "none",
		CreatedAt:         now,
	}
	if err := surfaces.Workflow.UpsertWorkflowArtifact(ctx, artifact); err != nil {
		return nil
	}
	ref := workflowutil.WorkflowArtifactReference(artifact)
	return &ref
}

func summarizeRewooStepResults(results []RewooStepResult) string {
	if len(results) == 0 {
		return ""
	}
	parts := make([]string, 0, len(results))
	for _, result := range results {
		status := "ok"
		if !result.Success {
			status = "failed"
		}
		parts = append(parts, fmt.Sprintf("%s [%s]", result.StepID, status))
	}
	return strings.Join(parts, "; ")
}

func buildReplanContext(plan *RewooPlan, results []RewooStepResult, err error) string {
	failed := make([]string, 0, len(results))
	for _, result := range results {
		if result.Success {
			continue
		}
		failed = append(failed, fmt.Sprintf("%s (%s): %s", result.StepID, result.Tool, result.Error))
	}
	if len(failed) == 0 {
		failed = append(failed, err.Error())
	}
	goal := ""
	if plan != nil {
		goal = plan.Goal
	}
	return fmt.Sprintf("Goal: %s\nFailures:\n%s", goal, strings.Join(failed, "\n"))
}

func cloneTaskWithContext(task *core.Task) *core.Task {
	cloned := core.CloneTask(task)
	if cloned == nil {
		cloned = &core.Task{}
	}
	if cloned.Context == nil {
		cloned.Context = map[string]any{}
	}
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func taskInstructionID(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ID)
}

func timePtr(value time.Time) *time.Time {
	return &value
}
