package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents/internal/workflowutil"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
)

// RewooAgent executes a strict plan/execute/synthesize workflow.
type RewooAgent struct {
	Model   core.LanguageModel
	Tools   *capability.Registry
	Memory  memory.MemoryStore
	Config  *core.Config
	Options RewooOptions

	initialised bool
}

func (a *RewooAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	a.initialised = true
	return nil
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
	planner := &rewooPlannerNode{Model: a.Model}
	planningTask := task
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
			planningTask = workflowutil.ApplyTaskRetrieval(task, retrievalPayload)
		}
	}

	var (
		plan      *RewooPlan
		results   []RewooStepResult
		synthesis string
		err       error
	)
	for attempt := 0; attempt <= options.MaxReplanAttempts; attempt++ {
		plan, err = planner.Plan(ctx, planningTask, a.Tools.ModelCallableLLMToolSpecs())
		if err != nil {
			if surfaces.Workflow != nil && runID != "" {
				_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusFailed, timePtr(time.Now().UTC()))
			}
			return nil, fmt.Errorf("rewoo: planning: %w", err)
		}
		state.Set("rewoo.plan", plan)
		a.persistPlan(ctx, surfaces, workflowID, runID, plan, attempt)

		results, err = (&rewooExecutor{
			Registry:  a.Tools,
			OnFailure: options.OnFailure,
			MaxSteps:  options.MaxSteps,
		}).Execute(ctx, plan, state)
		state.Set("rewoo.tool_results", results)
		a.persistStepResults(ctx, surfaces, workflowID, task, plan, results, attempt)
		if err != nil {
			if err == rewooErrReplanRequired && attempt < options.MaxReplanAttempts {
				replanContext := buildReplanContext(plan, results, err)
				state.Set("rewoo.replan_context", replanContext)
				planningTask = cloneTaskWithContext(planningTask)
				if planningTask.Context == nil {
					planningTask.Context = map[string]any{}
				}
				planningTask.Context["rewoo_replan_context"] = replanContext
				a.persistReplanSignal(ctx, surfaces, workflowID, runID, replanContext, attempt)
				continue
			}
			if surfaces.Workflow != nil && runID != "" {
				_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusFailed, timePtr(time.Now().UTC()))
			}
			return nil, fmt.Errorf("rewoo: execute: %w", err)
		}

		synthesis, err = synthesize(ctx, a.Model, planningTask, results)
		if err != nil {
			if surfaces.Workflow != nil && runID != "" {
				_ = surfaces.Workflow.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusFailed, timePtr(time.Now().UTC()))
			}
			return nil, fmt.Errorf("rewoo: synthesis: %w", err)
		}
		state.Set("rewoo.synthesis", synthesis)
		a.persistSynthesis(ctx, surfaces, workflowID, runID, task, synthesis, results)
		break
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

func (a *RewooAgent) persistSynthesis(ctx context.Context, surfaces workflowutil.RuntimeSurfaces, workflowID, runID string, task *core.Task, synthesis string, results []RewooStepResult) {
	if strings.TrimSpace(synthesis) == "" {
		return
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
	}
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
