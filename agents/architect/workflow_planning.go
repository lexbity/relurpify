package architect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents/internal/workflowutil"
	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
)

// WorkflowPlanningResult captures the durable outputs of the planning phase so
// architect execution can reuse them without rebuilding ad hoc state.
type WorkflowPlanningResult struct {
	PlanRecord        memory.WorkflowPlanRecord
	PlannerOutput     map[string]any
	PlanAdjustments   []string
	SelectedCandidate string
}

// WorkflowPlanningService runs the planning phase and persists workflow-scoped
// outputs such as the immutable plan and planner artifacts.
type WorkflowPlanningService struct {
	Model        core.LanguageModel
	Planner      *plannerpkg.PlannerAgent
	PlannerTools *capability.Registry
	Config       *core.Config
}

func (s *WorkflowPlanningService) PlanAndPersist(ctx context.Context, task *core.Task, state *core.Context, store *db.SQLiteWorkflowStateStore, workflowID, runID string) (*WorkflowPlanningResult, error) {
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	if store == nil {
		return nil, fmt.Errorf("workflow store required")
	}
	if s.Planner == nil {
		return nil, fmt.Errorf("planner required")
	}

	selectedCandidate, err := s.selectCandidate(ctx, task, state)
	if err != nil {
		return nil, err
	}

	plannerTask := core.CloneTask(task)
	if selectedCandidate != "" {
		plannerTask.Instruction = fmt.Sprintf("%s\n\nPreferred approach:\n%s", task.Instruction, selectedCandidate)
	}

	planState := core.NewContext()
	planState.Set("task.id", task.ID)
	planState.Set("task.type", string(task.Type))
	planState.Set("task.instruction", task.Instruction)
	if retrievalPayload, err := workflowutil.Hydrate(ctx, store, workflowID, workflowutil.RetrievalQuery{
		Primary:   task.Instruction,
		TaskText:  task.Instruction,
		StepFiles: workflowutil.TaskPaths(task),
	}, 4, 500); err != nil {
		return nil, err
	} else if len(retrievalPayload) > 0 {
		workflowutil.ApplyState(planState, "planner.workflow_retrieval", retrievalPayload)
		workflowutil.ApplyState(state, "planner.workflow_retrieval", retrievalPayload)
		plannerTask = workflowutil.ApplyTask(plannerTask, retrievalPayload)
	}

	planResult, err := s.Planner.Execute(ctx, plannerTask, planState)
	if err != nil {
		return nil, err
	}
	planVal, ok := planState.Get("planner.plan")
	if !ok {
		return nil, fmt.Errorf("planner did not populate a plan")
	}
	plan, ok := planVal.(core.Plan)
	if !ok {
		return nil, fmt.Errorf("planner produced invalid plan")
	}
	if err := graph.ValidatePlan(&plan); err != nil {
		return nil, err
	}

	record := memory.WorkflowPlanRecord{
		PlanID:     architectRecordID("plan"),
		WorkflowID: workflowID,
		RunID:      runID,
		Plan:       plan,
		IsActive:   true,
		CreatedAt:  time.Now().UTC(),
	}
	if err := store.SavePlan(ctx, record); err != nil {
		return nil, err
	}

	result := &WorkflowPlanningResult{
		PlanRecord:        record,
		SelectedCandidate: selectedCandidate,
	}
	if rawAdjustments, ok := planState.Get("planner.plan_adjustments"); ok {
		switch values := rawAdjustments.(type) {
		case []string:
			result.PlanAdjustments = append([]string{}, values...)
		case []any:
			for _, value := range values {
				if value == nil {
					continue
				}
				result.PlanAdjustments = append(result.PlanAdjustments, fmt.Sprint(value))
			}
		}
	}
	result.PlannerOutput = plannerOutputFromState(planState, planResult, plan, result.PlanAdjustments)

	s.applyPlanningState(state, result)
	if err := s.persistPlanningArtifacts(ctx, store, workflowID, runID, result); err != nil {
		return nil, err
	}
	if err := s.persistPlanningKnowledge(ctx, store, workflowID, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *WorkflowPlanningService) applyPlanningState(state *core.Context, result *WorkflowPlanningResult) {
	if state == nil || result == nil {
		return
	}
	state.Set("architect.plan", result.PlanRecord.Plan)
	state.Set("planner.plan", result.PlanRecord.Plan)
	state.Set("architect.completed_steps", []string{})
	state.Set("plan.completed_steps", []string{})
	state.Set("architect.last_step_summary", "")
	if result.SelectedCandidate != "" {
		state.Set("architect.selected_candidate", result.SelectedCandidate)
	}
	if result.PlannerOutput != nil {
		state.Set("architect.plan_result", result.PlannerOutput)
	}
	if len(result.PlanAdjustments) > 0 {
		state.Set("planner.plan_adjustments", append([]string{}, result.PlanAdjustments...))
	}
}

func (s *WorkflowPlanningService) persistPlanningArtifacts(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, runID string, result *WorkflowPlanningResult) error {
	now := time.Now().UTC()
	if result == nil {
		return nil
	}
	if result.PlannerOutput != nil {
		payload := mustJSONForArchitect(result.PlannerOutput)
		if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
			ArtifactID:        architectRecordID("planner_artifact"),
			WorkflowID:        workflowID,
			RunID:             runID,
			Kind:              "planner_output",
			ContentType:       "application/json",
			StorageKind:       memory.ArtifactStorageInline,
			SummaryText:       fmt.Sprintf("Planned %d steps.", len(result.PlanRecord.Plan.Steps)),
			SummaryMetadata:   map[string]any{"plan_id": result.PlanRecord.PlanID},
			InlineRawText:     payload,
			RawSizeBytes:      int64(len(payload)),
			CompressionMethod: "none",
			CreatedAt:         now,
		}); err != nil {
			return err
		}
	}
	if strings.TrimSpace(result.SelectedCandidate) != "" {
		payload := mustJSONForArchitect(map[string]any{
			"selected_candidate": result.SelectedCandidate,
		})
		if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
			ArtifactID:        architectRecordID("candidate_artifact"),
			WorkflowID:        workflowID,
			RunID:             runID,
			Kind:              "candidate_selection",
			ContentType:       "application/json",
			StorageKind:       memory.ArtifactStorageInline,
			SummaryText:       "Selected candidate approach for workflow planning.",
			InlineRawText:     payload,
			RawSizeBytes:      int64(len(payload)),
			CompressionMethod: "none",
			CreatedAt:         now,
		}); err != nil {
			return err
		}
	}
	return store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    architectRecordID("plan_created"),
		WorkflowID: workflowID,
		RunID:      runID,
		EventType:  "plan_created",
		Message:    fmt.Sprintf("Planned %d steps.", len(result.PlanRecord.Plan.Steps)),
		Metadata: map[string]any{
			"plan_id":       result.PlanRecord.PlanID,
			"adjustments":   append([]string{}, result.PlanAdjustments...),
			"has_candidate": strings.TrimSpace(result.SelectedCandidate) != "",
		},
		CreatedAt: now,
	})
}

func (s *WorkflowPlanningService) persistPlanningKnowledge(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID string, result *WorkflowPlanningResult) error {
	if result == nil || strings.TrimSpace(result.SelectedCandidate) == "" {
		return nil
	}
	return store.PutKnowledge(ctx, memory.KnowledgeRecord{
		RecordID:   architectRecordID("decision"),
		WorkflowID: workflowID,
		Kind:       memory.KnowledgeKindDecision,
		Title:      "Selected candidate approach",
		Content:    result.SelectedCandidate,
		Status:     "accepted",
		Metadata:   map[string]any{"source": "candidate_selection"},
		CreatedAt:  time.Now().UTC(),
	})
}

func (s *WorkflowPlanningService) selectCandidate(ctx context.Context, task *core.Task, state *core.Context) (string, error) {
	if !shouldRunCandidateSelection(task) || s.Model == nil || task == nil {
		return "", nil
	}
	if state != nil {
		if existing := state.GetString("architect.selected_candidate"); existing != "" {
			return existing, nil
		}
	}
	planningHints := planningHintsForConfig(s.Config, s.PlannerTools)
	candidatesPrompt := fmt.Sprintf(`Generate exactly 3 concise candidate implementation approaches for this task.
Return JSON as {"candidates":[{"id":"a","approach":"...","tradeoffs":"..."}]}.
%s
Task: %s`, planningHints, task.Instruction)
	resp, err := s.Model.Generate(ctx, candidatesPrompt, &core.LLMOptions{
		Model:       s.Config.Model,
		Temperature: 0.2,
		MaxTokens:   400,
	})
	if err != nil {
		return "", err
	}
	var generated struct {
		Candidates []struct {
			ID        string `json:"id"`
			Approach  string `json:"approach"`
			Tradeoffs string `json:"tradeoffs"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(reactpkg.ExtractJSON(resp.Text)), &generated); err != nil || len(generated.Candidates) == 0 {
		return "", nil
	}
	if state != nil {
		state.Set("architect.candidates", generated.Candidates)
	}

	var options []string
	for _, candidate := range generated.Candidates {
		options = append(options, fmt.Sprintf("%s: %s (tradeoffs: %s)", candidate.ID, candidate.Approach, candidate.Tradeoffs))
	}
	selectionPrompt := fmt.Sprintf(`Pick the single best candidate for this task.
Return JSON as {"id":"...","reason":"..."}.
Task: %s
Candidates:
%s`, task.Instruction, strings.Join(options, "\n"))
	selectionResp, err := s.Model.Generate(ctx, selectionPrompt, &core.LLMOptions{
		Model:       s.Config.Model,
		Temperature: 0,
		MaxTokens:   200,
	})
	if err != nil {
		return "", err
	}
	var selection struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(reactpkg.ExtractJSON(selectionResp.Text)), &selection); err != nil || selection.ID == "" {
		return "", nil
	}
	for _, candidate := range generated.Candidates {
		if candidate.ID == selection.ID {
			chosen := fmt.Sprintf("%s\nReason: %s", candidate.Approach, selection.Reason)
			if state != nil {
				state.Set("architect.selected_candidate", chosen)
			}
			return chosen, nil
		}
	}
	return "", nil
}

func planningHintsForConfig(cfg *core.Config, tools *capability.Registry) string {
	if cfg == nil || cfg.AgentSpec == nil {
		return ""
	}
	policy := frameworkskills.ResolveEffectiveSkillPolicy(nil, cfg.AgentSpec, tools).Policy
	return frameworkskills.RenderPlanningPolicy(policy, frameworkskills.PlanningRenderOptions{
		VerificationRequirement: "Include verification in the chosen approach.",
	})
}

func plannerOutputFromState(state *core.Context, result *core.Result, plan core.Plan, adjustments []string) map[string]any {
	out := map[string]any{
		"plan":        plan,
		"plan_steps":  plan.Steps,
		"files":       plan.Files,
		"adjustments": append([]string{}, adjustments...),
	}
	if result != nil && result.Data != nil {
		for key, value := range result.Data {
			out[key] = value
		}
	}
	if state != nil {
		if summary := state.GetString("planner.summary"); strings.TrimSpace(summary) != "" {
			out["summary"] = summary
		}
		if rawResults, ok := state.Get("planner.results"); ok {
			out["results"] = rawResults
		}
		if skipped, ok := state.Get("planner.skipped_tools"); ok {
			out["skipped_tools"] = skipped
		}
	}
	return out
}
