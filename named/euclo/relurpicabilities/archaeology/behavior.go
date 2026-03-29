package archaeology

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentblackboard "github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclobb "github.com/lexcodex/relurpify/named/euclo/execution/blackboard"
	rewooexec "github.com/lexcodex/relurpify/named/euclo/execution/rewoo"
	localbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/local"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type exploreBehavior struct{}
type compilePlanBehavior struct{}
type implementPlanBehavior struct{}

func NewExploreBehavior() execution.Behavior       { return exploreBehavior{} }
func NewCompilePlanBehavior() execution.Behavior   { return compilePlanBehavior{} }
func NewImplementPlanBehavior() execution.Behavior { return implementPlanBehavior{} }

func (exploreBehavior) ID() string { return Explore }

func (exploreBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:archaeology.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, in, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.AppendDiagnostic(in.State, "euclo.plan_candidates", "archaeology exploration behavior executed with archaeology-backed semantic inputs")
	execution.SetBehaviorTrace(in.State, in.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	explorationArtifacts, err := executeExplorationPasses(ctx, in)
	if err != nil {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, explorationArtifacts...)

	alternativeArtifacts, err := executeDesignAlternativesIfEligible(ctx, in)
	if err != nil {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, alternativeArtifacts...)
	if len(artifacts) > 0 {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
	}
	return execution.SuccessResult("archaeology explore completed successfully", artifacts)
}

func (compilePlanBehavior) ID() string { return CompilePlan }

func (compilePlanBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:archaeology.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, in, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.SetBehaviorTrace(in.State, in.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	evidencePayload := compileEvidencePayload(in)
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "archaeology_compile_evidence",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    strings.TrimSpace(fmt.Sprint(evidencePayload["summary"])),
		Payload:    evidencePayload,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	reconcileResult, _, reconcileErr := execution.ExecuteRecipe(ctx, in, execution.RecipeArchaeologyCompileReconcile, "archaeology-compile-reconcile",
		"Reconcile surfaced patterns, prospectives, tensions, and candidate directions into one coherent compile basis for: "+execution.CapabilityTaskInstruction(in.Task))
	if reconcileErr == nil && reconcileResult != nil && reconcileResult.Success {
		reconcilePayload := map[string]any{
			"source":   "euclo:archaeology.compile-plan",
			"evidence": evidencePayload,
			"result":   reconcileResult.Data,
			"summary":  execution.ResultSummary(reconcileResult),
		}
		if in.State != nil {
			in.State.Set("euclo.plan_candidates", reconcilePayload)
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "archaeology_compile_reconcile",
			Kind:       euclotypes.ArtifactKindPlanCandidates,
			Summary:    execution.ResultSummary(reconcileResult),
			Payload:    reconcilePayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	shapeResult, _, shapeErr := execution.ExecuteRecipe(ctx, in, execution.RecipeArchaeologyCompileShape, "archaeology-compile-shape",
		"Shape a full executable implementation plan from the compiled exploration evidence for: "+execution.CapabilityTaskInstruction(in.Task))
	if shapeErr == nil && shapeResult != nil && shapeResult.Success {
		draftPayload := map[string]any{
			"source":   "euclo:archaeology.compile-plan",
			"evidence": evidencePayload,
			"plan":     shapeResult.Data,
			"summary":  execution.ResultSummary(shapeResult),
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "archaeology_compile_plan_draft",
			Kind:       euclotypes.ArtifactKindPlanCandidates,
			Summary:    execution.ResultSummary(shapeResult),
			Payload:    draftPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, in, execution.RecipeArchaeologyCompileReview, "archaeology-compile-review",
		"Review the compiled plan draft for completeness, coherence, execution readiness, and missing constraints.")
	reviewPayload := map[string]any{
		"source": "euclo:archaeology.compile-plan",
	}
	if reviewErr == nil && reviewResult != nil && reviewResult.Success {
		reviewPayload["review"] = reviewResult.Data
		reviewPayload["summary"] = execution.ResultSummary(reviewResult)
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "archaeology_compile_plan_review",
			Kind:       euclotypes.ArtifactKindReviewFindings,
			Summary:    execution.ResultSummary(reviewResult),
			Payload:    reviewPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	payload := planArtifactFromState(in.State)
	if payload == nil && shapeResult != nil && shapeResult.Success {
		payload = shapeResult.Data
	}
	if !compiledPlanReady(payload) {
		issue := buildCompilePlanDeferredIssue(in, evidencePayload, reconcileResult, shapeResult, reviewResult)
		if in.State != nil {
			in.State.Set("euclo.deferred_execution_issues", []eucloruntime.DeferredExecutionIssue{issue})
			in.State.Set("euclo.deferred_issue_ids", []string{issue.IssueID})
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "archaeology_compile_plan_deferred",
			Kind:       euclotypes.ArtifactKindDeferredExecutionIssues,
			Summary:    issue.Summary,
			Payload:    []eucloruntime.DeferredExecutionIssue{issue},
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		err := fmt.Errorf("archaeology compile-plan deferred: executable plan not produced")
		return &core.Result{Success: false, Error: err, Data: map[string]any{
			"summary":   issue.Summary,
			"artifacts": artifacts,
		}}, err
	}

	if in.State != nil {
		in.State.Set("pipeline.plan", payload)
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "archaeology_compile_plan",
		Kind:       euclotypes.ArtifactKindPlan,
		Summary:    compilePlanSummary(shapeResult, payload),
		Payload:    payload,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("archaeology compile-plan completed successfully", artifacts)
}

func (implementPlanBehavior) ID() string { return ImplementPlan }

func (implementPlanBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:archaeology.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, in, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.AppendDiagnostic(in.State, "pipeline.plan", "archaeology implement-plan executing against a compiled plan")
	execution.SetBehaviorTrace(in.State, in.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)
	planPayload := planArtifactFromState(in.State)
	steps := compiledPlanSteps(planPayload)
	if len(steps) == 0 {
		err := fmt.Errorf("archaeology implement-plan requires executable plan steps")
		return &core.Result{Success: false, Error: err}, err
	}
	if result, handled, execErr := executeImplementPlanViaRewoo(ctx, in, planPayload, steps, artifacts); handled {
		return result, execErr
	}

	checkpointRefs := make([]string, 0, len(steps))
	completedSteps := make([]string, 0, len(steps))
	for idx, step := range steps {
		stepID := strings.TrimSpace(stringValue(step["id"]))
		if stepID == "" {
			stepID = fmt.Sprintf("plan-step-%d", idx+1)
		}
		stepTitle := firstNonEmptyString(
			stringValue(step["title"]),
			stringValue(step["description"]),
			fmt.Sprintf("plan step %d", idx+1),
		)
		if in.State != nil {
			in.State.Set("euclo.current_plan_step_id", stepID)
			in.State.Set("euclo.execution_status", map[string]any{
				"status":          "executing",
				"active_plan_id":  activePlanID(in.Work),
				"active_step_id":  stepID,
				"completed_steps": append([]string(nil), completedSteps...),
				"total_steps":     len(steps),
			})
		}

		implementResult, _, stepErr := execution.ExecuteRecipe(ctx, in, execution.RecipeArchaeologyImplementStep, "archaeology-implement-step-"+stepID,
			buildImplementStepInstruction(stepTitle, step, idx, len(steps), in),
		)
		if stepErr != nil || implementResult == nil || !implementResult.Success {
			issue := buildImplementPlanDeferredIssue(in, stepID, stepTitle, completedSteps, checkpointRefs, stepErr, implementResult)
			if in.State != nil {
				in.State.Set("euclo.deferred_execution_issues", []eucloruntime.DeferredExecutionIssue{issue})
				in.State.Set("euclo.deferred_issue_ids", []string{issue.IssueID})
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "archaeology_implement_plan_deferred_" + stepID,
				Kind:       euclotypes.ArtifactKindDeferredExecutionIssues,
				Summary:    issue.Summary,
				Payload:    []eucloruntime.DeferredExecutionIssue{issue},
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
			execution.MergeStateArtifactsToContext(in.State, artifacts)
			err := fmt.Errorf("archaeology implement-plan deferred at %s", stepID)
			return &core.Result{Success: false, Error: err, Data: map[string]any{
				"summary":   issue.Summary,
				"artifacts": artifacts,
			}}, err
		}

		stepPayload := map[string]any{
			"step_id":          stepID,
			"step_index":       idx + 1,
			"step_total":       len(steps),
			"title":            stepTitle,
			"description":      stringValue(step["description"]),
			"implementation":   implementResult.Data,
			"completed_before": append([]string(nil), completedSteps...),
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "archaeology_implement_step_" + stepID,
			Kind:       euclotypes.ArtifactKindEditExecution,
			Summary:    execution.ResultSummary(implementResult),
			Payload:    stepPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})

		reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, in, execution.RecipeArchaeologyImplementCheckpoint, "archaeology-implement-checkpoint-"+stepID,
			buildCheckpointInstruction(stepTitle, stepID, idx, len(steps)))
		checkpointID := "checkpoint_" + stepID
		checkpointPayload := map[string]any{
			"checkpoint_id": checkpointID,
			"step_id":       stepID,
			"step_index":    idx + 1,
			"step_total":    len(steps),
			"status":        "pass",
			"summary":       "checkpoint completed",
		}
		if reviewErr == nil && reviewResult != nil && reviewResult.Success {
			checkpointPayload["summary"] = execution.ResultSummary(reviewResult)
			checkpointPayload["review"] = reviewResult.Data
		}
		checkpointRefs = append(checkpointRefs, checkpointID)
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         checkpointID,
			Kind:       euclotypes.ArtifactKindVerification,
			Summary:    stringValue(checkpointPayload["summary"]),
			Payload:    checkpointPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
		completedSteps = append(completedSteps, stepID)
	}

	finalPayload := map[string]any{
		"plan_id":         activePlanID(in.Work),
		"plan_version":    activePlanVersion(in.Work),
		"completed_steps": completedSteps,
		"checkpoint_refs": checkpointRefs,
		"step_count":      len(steps),
		"summary":         fmt.Sprintf("implemented %d plan steps", len(completedSteps)),
	}
	if in.State != nil {
		in.State.Set("pipeline.verify", map[string]any{
			"status":          "pass",
			"summary":         finalPayload["summary"],
			"checkpoint_refs": checkpointRefs,
			"checks": []any{map[string]any{
				"name":   "plan_step_checkpoints",
				"status": "pass",
			}},
		})
		in.State.Set("euclo.execution_status", map[string]any{
			"status":          "completed",
			"active_plan_id":  activePlanID(in.Work),
			"completed_steps": completedSteps,
			"total_steps":     len(steps),
			"checkpoint_refs": checkpointRefs,
		})
	}
	artifacts = append(artifacts,
		euclotypes.Artifact{
			ID:         "archaeology_implement_plan_status",
			Kind:       euclotypes.ArtifactKindExecutionStatus,
			Summary:    stringValue(finalPayload["summary"]),
			Payload:    finalPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		},
		euclotypes.Artifact{
			ID:         "archaeology_implement_plan_verification",
			Kind:       euclotypes.ArtifactKindVerification,
			Summary:    stringValue(finalPayload["summary"]),
			Payload:    map[string]any{"status": "pass", "summary": finalPayload["summary"], "checkpoint_refs": checkpointRefs},
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		},
	)
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("archaeology implement-plan completed successfully", artifacts)
}

func executeImplementPlanViaRewoo(ctx context.Context, in execution.ExecuteInput, planPayload map[string]any, steps []map[string]any, baseArtifacts []euclotypes.Artifact) (*core.Result, bool, error) {
	if len(steps) < 2 {
		return nil, false, nil
	}
	task := core.CloneTask(in.Task)
	if task == nil {
		task = &core.Task{}
	}
	if task.Context == nil {
		task.Context = map[string]any{}
	}
	task.Type = core.TaskTypeCodeModification
	task.Context["compiled_plan"] = planPayload
	task.Context["plan_steps"] = steps
	task.Context["plan_id"] = activePlanID(in.Work)
	task.Context["plan_version"] = activePlanVersion(in.Work)
	task.Instruction = "Execute the compiled implementation plan step by step using the provided compiled_plan and plan_steps context. Preserve plan order, execute concretely, and synthesize final progress."

	result, err := rewooexec.Execute(ctx, in.Environment, task, in.State)
	if err != nil || result == nil || !result.Success {
		if in.State != nil {
			execution.AppendDiagnostic(in.State, "pipeline.plan", "rewoo plan execution degraded; falling back to manual step execution")
		}
		return nil, false, nil
	}
	execution.AddSpecializedCapabilityTrace(in.State, "euclo.execution.rewoo")

	var artifacts []euclotypes.Artifact
	artifacts = append(artifacts, baseArtifacts...)
	stepResults := rewooStepResultsFromState(in.State)
	checkpointRefs := make([]string, 0, len(stepResults))
	completedSteps := make([]string, 0, len(stepResults))
	for idx, step := range stepResults {
		stepID := firstNonEmptyString(stringValue(step["step_id"]), fmt.Sprintf("rewoo-step-%d", idx+1))
		checkpointID := "checkpoint_" + stepID
		checkpointRefs = append(checkpointRefs, checkpointID)
		completedSteps = append(completedSteps, stepID)
		artifacts = append(artifacts,
			euclotypes.Artifact{
				ID:         "archaeology_rewoo_step_" + stepID,
				Kind:       euclotypes.ArtifactKindEditExecution,
				Summary:    firstNonEmptyString(stringValue(step["tool"]), "rewoo step executed"),
				Payload:    step,
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			},
			euclotypes.Artifact{
				ID:         checkpointID,
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    firstNonEmptyString(stringValue(step["tool"]), "rewoo checkpoint"),
				Payload:    map[string]any{"checkpoint_id": checkpointID, "step_id": stepID, "result": step},
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			},
		)
	}
	summary := "implemented compiled plan via rewoo"
	if raw, ok := in.State.Get("rewoo.synthesis"); ok {
		if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
			summary = strings.TrimSpace(text)
		}
	}
	finalPayload := map[string]any{
		"plan_id":         activePlanID(in.Work),
		"plan_version":    activePlanVersion(in.Work),
		"completed_steps": completedSteps,
		"checkpoint_refs": checkpointRefs,
		"step_count":      len(stepResults),
		"summary":         summary,
		"source":          "rewoo",
	}
	if in.State != nil {
		in.State.Set("pipeline.verify", map[string]any{
			"status":          "pass",
			"summary":         summary,
			"checkpoint_refs": checkpointRefs,
			"checks":          []any{map[string]any{"name": "rewoo_plan_execution", "status": "pass"}},
		})
		in.State.Set("euclo.execution_status", map[string]any{
			"status":          "completed",
			"active_plan_id":  activePlanID(in.Work),
			"completed_steps": completedSteps,
			"total_steps":     len(steps),
			"checkpoint_refs": checkpointRefs,
			"source":          "rewoo",
		})
	}
	artifacts = append(artifacts,
		euclotypes.Artifact{
			ID:         "archaeology_implement_plan_status",
			Kind:       euclotypes.ArtifactKindExecutionStatus,
			Summary:    summary,
			Payload:    finalPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		},
		euclotypes.Artifact{
			ID:         "archaeology_implement_plan_verification",
			Kind:       euclotypes.ArtifactKindVerification,
			Summary:    summary,
			Payload:    map[string]any{"status": "pass", "summary": summary, "checkpoint_refs": checkpointRefs, "source": "rewoo"},
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		},
	)
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	success, _ := execution.SuccessResult("archaeology implement-plan completed via rewoo", artifacts)
	return success, true, nil
}

func rewooStepResultsFromState(state *core.Context) []map[string]any {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("rewoo.tool_results")
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if record, ok := item.(map[string]any); ok {
				out = append(out, record)
			}
		}
		return out
	case map[string]any:
		if steps, ok := typed["steps"].([]map[string]any); ok {
			return append([]map[string]any(nil), steps...)
		}
		if steps, ok := typed["steps"].([]any); ok {
			out := make([]map[string]any, 0, len(steps))
			for _, item := range steps {
				if record, ok := item.(map[string]any); ok {
					out = append(out, record)
				}
			}
			return out
		}
	}
	return nil
}

func planArtifactFromState(state *core.Context) map[string]any {
	if state == nil {
		return nil
	}
	if raw, ok := state.Get("pipeline.plan"); ok {
		if typed, ok := raw.(map[string]any); ok && len(typed) > 0 {
			return typed
		}
	}
	if raw, ok := state.Get("propose.items"); ok && raw != nil {
		return map[string]any{"items": raw}
	}
	return nil
}

func executeExplorationPasses(ctx context.Context, in execution.ExecuteInput) ([]euclotypes.Artifact, error) {
	envelope := archaeologyExecutionEnvelope(in)
	if in.State != nil {
		in.State.Set("euclo.blackboard_seed_facts", map[string]any{
			"archaeology:task":           execution.CapabilityTaskInstruction(in.Task),
			"archaeology:pattern_refs":   append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
			"archaeology:tension_refs":   append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
			"archaeology:prospective":    append([]string(nil), in.Work.SemanticInputs.ProspectiveRefs...),
			"archaeology:convergence":    append([]string(nil), in.Work.SemanticInputs.ConvergenceRefs...),
			"archaeology:provenance":     append([]string(nil), in.Work.SemanticInputs.RequestProvenanceRefs...),
			"archaeology:exploration_id": strings.TrimSpace(in.Work.SemanticInputs.ExplorationID),
		})
	}
	bbResult, err := euclobb.Execute(ctx, envelope, archaeologyKnowledgeSources(), 6, func(bb *agentblackboard.Blackboard) bool {
		return boardHasFact(bb, "archaeology:convergence_assessment")
	})
	if err != nil {
		return nil, err
	}

	explorePayload := buildExplorePayloadFromBoard(bbResult.Board)
	artifacts := []euclotypes.Artifact{{
		ID:         "archaeology_explore",
		Kind:       euclotypes.ArtifactKindExplore,
		Summary:    strings.TrimSpace(fmt.Sprint(explorePayload["summary"])),
		Payload:    explorePayload,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	}}
	if in.State != nil {
		in.State.Set("pipeline.explore", explorePayload)
	}

	planResult, _, planErr := execution.ExecuteRecipe(ctx, in, execution.RecipeArchaeologyExploreShape, "archaeology-explore-shape",
		"Shape the exploration findings into candidate engineering directions for: "+execution.CapabilityTaskInstruction(in.Task))
	if planErr == nil && planResult != nil && planResult.Success {
		planPayload := map[string]any{
			"source":      "euclo:archaeology.explore",
			"exploration": explorePayload,
			"candidates":  planResult.Data,
			"summary":     execution.ResultSummary(planResult),
		}
		if in.State != nil {
			in.State.Set("euclo.plan_candidates", planPayload)
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "archaeology_explore_candidates",
			Kind:       euclotypes.ArtifactKindPlanCandidates,
			Summary:    execution.ResultSummary(planResult),
			Payload:    planPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, in, execution.RecipeArchaeologyExploreReview, "archaeology-explore-review",
		"Review the exploration findings for coherence, convergence, and missing constraints.")
	if reviewErr == nil && reviewResult != nil && reviewResult.Success {
		reviewPayload := map[string]any{
			"source":      "euclo:archaeology.explore",
			"exploration": explorePayload,
			"review":      reviewResult.Data,
			"summary":     execution.ResultSummary(reviewResult),
		}
		if in.State != nil {
			in.State.Set("pipeline.analyze", reviewPayload)
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "archaeology_explore_review",
			Kind:       euclotypes.ArtifactKindAnalyze,
			Summary:    execution.ResultSummary(reviewResult),
			Payload:    reviewPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	return artifacts, nil
}

func executeDesignAlternativesIfEligible(ctx context.Context, in execution.ExecuteInput) ([]euclotypes.Artifact, error) {
	capability := localbehavior.NewDesignAlternativesCapability(in.Environment)
	if capability == nil {
		return nil, nil
	}
	artifactState := euclotypes.ArtifactStateFromContext(in.State)
	snapshot := eucloruntime.SnapshotCapabilities(in.Environment.Registry)
	if !capability.Eligible(artifactState, snapshot).Eligible {
		return nil, nil
	}
	result := capability.Execute(ctx, archaeologyExecutionEnvelope(in))
	execution.AddSpecializedCapabilityTrace(in.State, capability.Descriptor().ID)
	if result.Status == euclotypes.ExecutionStatusFailed {
		msg := strings.TrimSpace(result.Summary)
		if msg == "" && result.FailureInfo != nil {
			msg = strings.TrimSpace(result.FailureInfo.Message)
		}
		if msg == "" {
			msg = "design alternatives exploration failed"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return result.Artifacts, nil
}

func archaeologyExecutionEnvelope(in execution.ExecuteInput) euclotypes.ExecutionEnvelope {
	return euclotypes.ExecutionEnvelope{
		Task:        in.Task,
		Mode:        in.Mode,
		Profile:     in.Profile,
		Registry:    in.Environment.Registry,
		State:       in.State,
		Memory:      in.Environment.Memory,
		Environment: in.Environment,
		Telemetry:   in.Telemetry,
		WorkflowID:  in.Work.WorkflowID,
		RunID:       in.Work.RunID,
	}
}

func archaeologyKnowledgeSources() []agentblackboard.KnowledgeSource {
	return []agentblackboard.KnowledgeSource{
		euclobb.NewAnalysisKnowledgeSource("Pattern Mapper", "not archaeology:patterns exists", []string{"file_read"},
			`Surface architectural and implementation patterns relevant to the exploration request.
Goal: {{goal}}
Context: {{entries}}
Return JSON with:
- facts: [{"key":"archaeology:patterns","value":[{"name":"...","summary":"...","files":["..."],"relevance":0.0}]}]
- summary: short string`),
		euclobb.NewSynthesisKnowledgeSource("Prospective Explorer", "archaeology:patterns exists", []string{"archaeology:patterns", "archaeology:task", "archaeology:provenance"},
			`Identify plausible engineering directions from the surfaced patterns.
Inputs: {{input_entries}}
Return JSON with:
- facts: [{"key":"archaeology:prospectives","value":[{"title":"...","summary":"...","tradeoffs":["..."],"confidence":0.0}]}]
- summary: short string`),
		euclobb.NewSynthesisKnowledgeSource("Coherence Reviewer", "archaeology:prospectives exists", []string{"archaeology:patterns", "archaeology:prospectives", "archaeology:tension_refs"},
			`Review whether the prospective directions fit the discovered patterns and tensions coherently.
Inputs: {{input_entries}}
Return JSON with:
- facts: [{"key":"archaeology:coherence_assessment","value":{"status":"coherent","notes":["..."],"risks":["..."]}}]
- summary: short string`),
		euclobb.NewSynthesisKnowledgeSource("Convergence Analyst", "archaeology:coherence_assessment exists", []string{"archaeology:prospectives", "archaeology:coherence_assessment", "archaeology:convergence"},
			`Assess whether the exploration is converging on a workable engineering direction.
Inputs: {{input_entries}}
Return JSON with:
- facts: [{"key":"archaeology:convergence_assessment","value":{"status":"ready","recommended_direction":"...","open_questions":["..."]}}]
- summary: short string`),
	}
}

func buildExplorePayloadFromBoard(board *agentblackboard.Blackboard) map[string]any {
	patterns, _ := boardFact(board, "archaeology:patterns")
	prospectives, _ := boardFact(board, "archaeology:prospectives")
	coherence, _ := boardFact(board, "archaeology:coherence_assessment")
	convergence, _ := boardFact(board, "archaeology:convergence_assessment")
	summary := "archaeology exploration completed"
	if record, ok := convergence.(map[string]any); ok {
		if text, ok := record["recommended_direction"].(string); ok && strings.TrimSpace(text) != "" {
			summary = "archaeology exploration converged on: " + strings.TrimSpace(text)
		}
	}
	return map[string]any{
		"patterns":               defaultAny(patterns, []any{}),
		"prospectives":           defaultAny(prospectives, []any{}),
		"coherence_assessment":   defaultAny(coherence, map[string]any{}),
		"convergence_assessment": defaultAny(convergence, map[string]any{}),
		"summary":                summary,
	}
}

func boardHasFact(board *agentblackboard.Blackboard, key string) bool {
	_, ok := boardFact(board, key)
	return ok
}

func boardFact(board *agentblackboard.Blackboard, key string) (any, bool) {
	if board == nil {
		return nil, false
	}
	key = strings.TrimSpace(key)
	for i := len(board.Facts) - 1; i >= 0; i-- {
		if board.Facts[i].Key == key {
			return decodeJSONOrString(board.Facts[i].Value), true
		}
	}
	return nil, false
}

func decodeJSONOrString(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		return payload
	}
	return raw
}

func defaultAny(value any, fallback any) any {
	if value == nil {
		return fallback
	}
	return value
}

func compileEvidencePayload(in execution.ExecuteInput) map[string]any {
	payload := map[string]any{
		"task":              execution.CapabilityTaskInstruction(in.Task),
		"pattern_refs":      append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
		"tension_refs":      append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
		"prospective_refs":  append([]string(nil), in.Work.SemanticInputs.ProspectiveRefs...),
		"convergence_refs":  append([]string(nil), in.Work.SemanticInputs.ConvergenceRefs...),
		"pattern_proposals": append([]eucloruntime.PatternProposalSummary(nil), in.Work.SemanticInputs.PatternProposals...),
		"coherence":         append([]eucloruntime.CoherenceSuggestion(nil), in.Work.SemanticInputs.CoherenceSuggestions...),
		"summary":           "compile-plan synthesized archaeology evidence into a plan-ready input bundle",
	}
	if in.State != nil {
		if raw, ok := in.State.Get("pipeline.explore"); ok && raw != nil {
			payload["exploration"] = raw
		}
		if raw, ok := in.State.Get("euclo.plan_candidates"); ok && raw != nil {
			payload["candidate_directions"] = raw
		}
	}
	return payload
}

func compiledPlanReady(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	if steps := anySlice(payload["steps"]); len(steps) > 0 {
		return true
	}
	if items := anySlice(payload["items"]); len(items) > 0 {
		return true
	}
	if nested, ok := payload["plan"].(map[string]any); ok {
		return compiledPlanReady(nested)
	}
	return false
}

func anySlice(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func compilePlanSummary(shapeResult *core.Result, payload map[string]any) string {
	if shapeResult != nil && strings.TrimSpace(execution.ResultSummary(shapeResult)) != "" {
		return execution.ResultSummary(shapeResult)
	}
	if payload != nil {
		if summary, ok := payload["summary"].(string); ok && strings.TrimSpace(summary) != "" {
			return strings.TrimSpace(summary)
		}
	}
	return "compiled executable plan produced"
}

func buildCompilePlanDeferredIssue(in execution.ExecuteInput, evidencePayload map[string]any, reconcileResult, shapeResult, reviewResult *core.Result) eucloruntime.DeferredExecutionIssue {
	now := time.Now().UTC()
	stepID := ""
	if in.Work.PlanBinding != nil {
		stepID = strings.TrimSpace(in.Work.PlanBinding.ActiveStepID)
	}
	return eucloruntime.DeferredExecutionIssue{
		IssueID:               fmt.Sprintf("compile-plan-deferred-%d", now.UnixNano()),
		WorkflowID:            in.Work.WorkflowID,
		RunID:                 in.Work.RunID,
		ExecutionID:           in.Work.ExecutionID,
		ActivePlanID:          activePlanID(in.Work),
		ActivePlanVersion:     activePlanVersion(in.Work),
		StepID:                stepID,
		Kind:                  eucloruntime.DeferredIssueAmbiguity,
		Severity:              eucloruntime.DeferredIssueSeverityMedium,
		Status:                eucloruntime.DeferredIssueStatusOpen,
		Title:                 "Compile-plan did not produce an executable plan",
		Summary:               "Archaeology compile-plan finished its compile passes without a materially executable plan artifact.",
		WhyNotResolvedInline:  "compile-plan must either emit a full executable plan or defer for later review",
		RecommendedReentry:    "archaeology",
		RecommendedNextAction: "review the exploration evidence, reconcile unresolved constraints, and rerun compile-plan",
		Evidence: eucloruntime.DeferredExecutionEvidence{
			RelevantPatternRefs:    append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
			RelevantTensionRefs:    append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
			RelevantProvenanceRefs: append([]string(nil), in.Work.SemanticInputs.ProvenanceRefs...),
			RelevantRequestRefs:    append([]string(nil), in.Work.SemanticInputs.RequestProvenanceRefs...),
			ShortReasoningSummary:  compileDeferredReasoning(evidencePayload, reconcileResult, shapeResult, reviewResult),
		},
		ArchaeoRefs: map[string][]string{
			"pattern_refs":     append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
			"tension_refs":     append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
			"prospective_refs": append([]string(nil), in.Work.SemanticInputs.ProspectiveRefs...),
			"convergence_refs": append([]string(nil), in.Work.SemanticInputs.ConvergenceRefs...),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func compileDeferredReasoning(evidencePayload map[string]any, reconcileResult, shapeResult, reviewResult *core.Result) string {
	parts := []string{}
	if summary, ok := evidencePayload["summary"].(string); ok && strings.TrimSpace(summary) != "" {
		parts = append(parts, strings.TrimSpace(summary))
	}
	if reconcileResult != nil && strings.TrimSpace(execution.ResultSummary(reconcileResult)) != "" {
		parts = append(parts, "reconcile="+execution.ResultSummary(reconcileResult))
	}
	if shapeResult != nil && strings.TrimSpace(execution.ResultSummary(shapeResult)) != "" {
		parts = append(parts, "shape="+execution.ResultSummary(shapeResult))
	}
	if reviewResult != nil && strings.TrimSpace(execution.ResultSummary(reviewResult)) != "" {
		parts = append(parts, "review="+execution.ResultSummary(reviewResult))
	}
	if len(parts) == 0 {
		return "compile-plan ended without a materially executable plan artifact"
	}
	return strings.Join(parts, " | ")
}

func activePlanID(work eucloruntime.UnitOfWork) string {
	if work.PlanBinding == nil {
		return ""
	}
	return strings.TrimSpace(work.PlanBinding.PlanID)
}

func activePlanVersion(work eucloruntime.UnitOfWork) int {
	if work.PlanBinding == nil {
		return 0
	}
	return work.PlanBinding.PlanVersion
}

func compiledPlanSteps(plan map[string]any) []map[string]any {
	if plan == nil {
		return nil
	}
	if steps := mapSlice(plan["steps"]); len(steps) > 0 {
		return steps
	}
	if nested, ok := plan["plan"].(map[string]any); ok {
		if steps := mapSlice(nested["steps"]); len(steps) > 0 {
			return steps
		}
	}
	if items := mapSlice(plan["items"]); len(items) > 0 {
		return items
	}
	return nil
}

func mapSlice(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
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

func buildImplementStepInstruction(stepTitle string, step map[string]any, index, total int, in execution.ExecuteInput) string {
	parts := []string{
		fmt.Sprintf("Execute plan step %d/%d: %s.", index+1, total, stepTitle),
	}
	if desc := strings.TrimSpace(stringValue(step["description"])); desc != "" && desc != stepTitle {
		parts = append(parts, "Description: "+desc)
	}
	if expected := strings.TrimSpace(stringValue(step["expected"])); expected != "" {
		parts = append(parts, "Expected outcome: "+expected)
	}
	parts = append(parts, "Overall plan objective: "+execution.CapabilityTaskInstruction(in.Task))
	return strings.Join(parts, " ")
}

func buildCheckpointInstruction(stepTitle, stepID string, index, total int) string {
	return fmt.Sprintf("Review checkpoint %d/%d for plan step %s (%s). Confirm the implementation is coherent and note any unresolved risks.", index+1, total, stepID, stepTitle)
}

func buildImplementPlanDeferredIssue(in execution.ExecuteInput, stepID, stepTitle string, completedSteps, checkpointRefs []string, stepErr error, result *core.Result) eucloruntime.DeferredExecutionIssue {
	now := time.Now().UTC()
	summary := fmt.Sprintf("Plan execution halted at step %s (%s).", stepID, stepTitle)
	details := strings.TrimSpace(execution.ErrorMessage(stepErr, result))
	if details == "" {
		details = "step execution did not complete successfully"
	}
	return eucloruntime.DeferredExecutionIssue{
		IssueID:               fmt.Sprintf("implement-plan-deferred-%d", now.UnixNano()),
		WorkflowID:            in.Work.WorkflowID,
		RunID:                 in.Work.RunID,
		ExecutionID:           in.Work.ExecutionID,
		ActivePlanID:          activePlanID(in.Work),
		ActivePlanVersion:     activePlanVersion(in.Work),
		StepID:                stepID,
		RelatedStepIDs:        append([]string(nil), completedSteps...),
		Kind:                  eucloruntime.DeferredIssueNonfatalFailure,
		Severity:              eucloruntime.DeferredIssueSeverityHigh,
		Status:                eucloruntime.DeferredIssueStatusOpen,
		Title:                 "Plan execution paused at a failing step",
		Summary:               summary,
		WhyNotResolvedInline:  "plan-bound execution stopped at a step boundary to preserve single-plan continuity",
		RecommendedReentry:    "archaeology",
		RecommendedNextAction: "inspect the failing step, review checkpoint evidence, and resume plan execution after resolving the blocker",
		Evidence: eucloruntime.DeferredExecutionEvidence{
			RelevantPatternRefs:    append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
			RelevantTensionRefs:    append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
			RelevantProvenanceRefs: append([]string(nil), in.Work.SemanticInputs.ProvenanceRefs...),
			RelevantRequestRefs:    append([]string(nil), in.Work.SemanticInputs.RequestProvenanceRefs...),
			CheckpointRefs:         append([]string(nil), checkpointRefs...),
			ShortReasoningSummary:  details,
		},
		ArchaeoRefs: map[string][]string{
			"pattern_refs":     append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
			"tension_refs":     append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
			"prospective_refs": append([]string(nil), in.Work.SemanticInputs.ProspectiveRefs...),
			"convergence_refs": append([]string(nil), in.Work.SemanticInputs.ConvergenceRefs...),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func stringValue(raw any) string {
	return strings.TrimSpace(fmt.Sprint(raw))
}
