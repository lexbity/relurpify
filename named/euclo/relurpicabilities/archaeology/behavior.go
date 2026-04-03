package archaeology

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentblackboard "github.com/lexcodex/relurpify/agents/blackboard"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
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

type enrichedArchaeoInput struct {
	requestHistory      *execution.RequestHistoryView
	activePlan          *execution.ActivePlanView
	learningQueue       *execution.LearningQueueView
	tensions            []execution.TensionView
	tensionSummary      *execution.TensionSummaryView
	patternRefs         []string
	tensionIDs          []string
	requestRefs         []string
	learningRefs        []string
	anchorRefs          []string
	basedOnRevision     string
	semanticSnapshotRef string
	explorationID       string
}

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
	enriched := enrichArchaeoExecutionInput(ctx, in)
	ensureArchaeologyExecutionState(in.State, in.Work, enriched)
	if len(enriched.patternRefs) > 0 || len(enriched.tensionIDs) > 0 || len(enriched.learningRefs) > 0 {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "archaeology_explore_archaeo_context",
			Kind:       euclotypes.ArtifactKindExplore,
			Summary:    enriched.summary(),
			Payload:    enriched.payload(),
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	explorationArtifacts, err := executeExplorationPasses(ctx, withEnrichedSemanticInput(in, enriched))
	if err != nil {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, explorationArtifacts...)
	persistExplorationPatterns(ctx, in, explorationArtifacts)

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
	enriched := enrichArchaeoExecutionInput(ctx, in)
	ensureArchaeologyExecutionState(in.State, in.Work, enriched)

	evidencePayload := compileEvidencePayload(withEnrichedSemanticInput(in, enriched))
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

	shapeResult, _, shapeErr := execution.ExecuteRecipe(ctx, withEnrichedSemanticInput(in, enriched), execution.RecipeArchaeologyCompileShape, "archaeology-compile-shape",
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
	reviewPayload := archaeologyReviewPayload("euclo:archaeology.compile-plan", "", nil)
	if reviewErr == nil && reviewResult != nil && reviewResult.Success {
		reviewPayload = archaeologyReviewPayload("euclo:archaeology.compile-plan", execution.ResultSummary(reviewResult), reviewResult.Data)
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
	if persisted, err := persistCompiledPlan(ctx, in, payload, enriched); err == nil && persisted != nil {
		if in.State != nil {
			in.State.Set("euclo.active_plan_version", persisted)
			in.State.Set("euclo.living_plan", &persisted.Plan)
		}
		// GuidanceBroker notification: if the plan review surfaced open
		// questions, submit a non-blocking guidance request so the TUI/operator
		// layer can surface them for acknowledgement. Errors are swallowed —
		// guidance notification must not block plan activation.
		submitPlanReviewGuidance(in, persisted.Plan.ID, reviewResult)
		payload["plan_id"] = persisted.Plan.ID
		payload["plan_version"] = persisted.Version
		payload["workflow_id"] = persisted.WorkflowID
		persistPlanReviewComment(ctx, in, persisted.Plan.ID, reviewResult)
	} else if err != nil {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
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
	seededPlanPayload := planArtifactFromState(in.State)
	routines := execution.SupportingIDs(in.Work, "euclo:archaeology.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, in, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.AppendDiagnostic(in.State, "pipeline.plan", "archaeology implement-plan executing against a compiled plan")
	execution.SetBehaviorTrace(in.State, in.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)
	ensureArchaeologyExecutionState(in.State, in.Work, enrichArchaeoExecutionInput(ctx, in))
	// Blocking learning gate: if the convergence-guard routine found unresolved
	// blocking learning interactions, halt before any plan step executes. These
	// represent prior learning items (anchor drift, pattern proposals, tension
	// reviews) that must be resolved before execution is safe to proceed.
	if blockingIDs := blockingLearningIDsFromRoutineArtifacts(routineArtifacts); len(blockingIDs) > 0 {
		now := time.Now().UTC()
		issue := eucloruntime.DeferredExecutionIssue{
			IssueID:               fmt.Sprintf("blocking-learning-gate-%d", now.UnixNano()),
			WorkflowID:            in.Work.WorkflowID,
			RunID:                 in.Work.RunID,
			ExecutionID:           in.Work.ExecutionID,
			ActivePlanID:          activePlanID(in.Work),
			ActivePlanVersion:     activePlanVersion(in.Work),
			Kind:                  eucloruntime.DeferredIssueStaleAssumption,
			Severity:              eucloruntime.DeferredIssueSeverityHigh,
			Status:                eucloruntime.DeferredIssueStatusOpen,
			Title:                 fmt.Sprintf("Implement-plan blocked: %d unresolved learning interaction(s)", len(blockingIDs)),
			Summary:               fmt.Sprintf("Execution halted: %d blocking learning interaction(s) must be resolved before plan execution can proceed.", len(blockingIDs)),
			WhyNotResolvedInline:  "blocking learning interactions must be resolved by the operator before plan execution resumes",
			RecommendedReentry:    "archaeology",
			RecommendedNextAction: "review and resolve pending learning interactions, then retry plan execution",
			Evidence: eucloruntime.DeferredExecutionEvidence{
				RelevantPatternRefs: append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
				RelevantTensionRefs: append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
				CheckpointRefs:      append([]string(nil), blockingIDs...),
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		if in.State != nil {
			in.State.Set("euclo.deferred_execution_issues", []eucloruntime.DeferredExecutionIssue{issue})
			in.State.Set("euclo.deferred_issue_ids", []string{issue.IssueID})
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "archaeology_blocking_learning_gate",
			Kind:       euclotypes.ArtifactKindDeferredExecutionIssues,
			Summary:    issue.Summary,
			Payload:    []eucloruntime.DeferredExecutionIssue{issue},
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		gateErr := fmt.Errorf("archaeology implement-plan blocked by unresolved learning interactions: %v", blockingIDs)
		return &core.Result{Success: false, Error: gateErr, Data: map[string]any{
			"summary":   issue.Summary,
			"artifacts": artifacts,
		}}, gateErr
	}
	planPayload := planArtifactFromState(in.State)
	if len(compiledPlanSteps(planPayload)) == 0 {
		planPayload = seededPlanPayload
		if in.State != nil && planPayload != nil {
			in.State.Set("pipeline.plan", planPayload)
		}
	}
	if planPayload == nil {
		if active, err := loadBoundPlan(ctx, in); err == nil && active != nil {
			planPayload = versionedPlanPayload(active)
			if in.State != nil {
				in.State.Set("euclo.active_plan_version", active)
				in.State.Set("euclo.living_plan", &active.Plan)
				in.State.Set("pipeline.plan", planPayload)
			}
		}
	}
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

		blastRadius := queryStepBlastRadius(in, stringSlice(step["scope"]))
		if in.State != nil && blastRadius != nil {
			in.State.Set("euclo.current_step_blast_radius", blastRadius)
		}
		// Confidence gate: skip steps whose confidence has degraded below the
		// acceptable threshold. They are deferred as stale-assumption issues so
		// the operator can decide whether to accept the risk or re-plan.
		if planStep := livingPlanStepFromState(in.State, stepID); planStep != nil &&
			planStep.ConfidenceScore > 0 &&
			planStep.ConfidenceScore < frameworkplan.DefaultConfidenceDegradation().Threshold {
			now := time.Now().UTC()
			issue := eucloruntime.DeferredExecutionIssue{
				IssueID:               fmt.Sprintf("confidence-gate-%s-%d", stepID, now.UnixNano()),
				WorkflowID:            in.Work.WorkflowID,
				RunID:                 in.Work.RunID,
				ExecutionID:           in.Work.ExecutionID,
				ActivePlanID:          activePlanID(in.Work),
				ActivePlanVersion:     activePlanVersion(in.Work),
				StepID:                stepID,
				Kind:                  eucloruntime.DeferredIssueStaleAssumption,
				Severity:              eucloruntime.DeferredIssueSeverityHigh,
				Status:                eucloruntime.DeferredIssueStatusOpen,
				Title:                 fmt.Sprintf("Step %s skipped: confidence below threshold (%.2f)", stepID, planStep.ConfidenceScore),
				Summary:               fmt.Sprintf("Plan step %q has confidence %.2f below required threshold %.2f — execution deferred.", stepID, planStep.ConfidenceScore, frameworkplan.DefaultConfidenceDegradation().Threshold),
				WhyNotResolvedInline:  "confidence degradation requires operator acknowledgment before execution can continue",
				RecommendedReentry:    "archaeology",
				RecommendedNextAction: "review anchor dependencies, update plan step assumptions, and re-run compile-plan",
				Evidence: eucloruntime.DeferredExecutionEvidence{
					RelevantPatternRefs: append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
					RelevantTensionRefs: append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
				},
				CreatedAt: now,
				UpdatedAt: now,
			}
			if in.State != nil {
				in.State.Set("euclo.deferred_execution_issues", append(deferredIssuesFromState(in.State), issue))
				in.State.Set("euclo.deferred_issue_ids", []string{issue.IssueID})
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "archaeology_confidence_gate_" + stepID,
				Kind:       euclotypes.ArtifactKindDeferredExecutionIssues,
				Summary:    issue.Summary,
				Payload:    []eucloruntime.DeferredExecutionIssue{issue},
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
			execution.MergeStateArtifactsToContext(in.State, artifacts)
			gateErr := fmt.Errorf("archaeology implement-plan deferred at %s: confidence gate", stepID)
			return &core.Result{Success: false, Error: gateErr, Data: map[string]any{
				"summary":   issue.Summary,
				"artifacts": artifacts,
			}}, gateErr
		}
		implementResult, _, stepErr := execution.ExecuteRecipe(ctx, in, execution.RecipeArchaeologyImplementStep, "archaeology-implement-step-"+stepID,
			buildImplementStepInstruction(stepTitle, step, idx, len(steps), in, blastRadius),
		)
		if stepErr != nil || implementResult == nil || !implementResult.Success {
			failureReason := strings.TrimSpace(execution.ErrorMessage(stepErr, implementResult))
			if failureReason == "" {
				failureReason = "step execution did not complete successfully"
			}
			recordStepAttempt(ctx, in, stepID, "failed", failureReason, "")
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
			"blast_radius":     blastRadius,
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
		// Gap detection: check implementation against anchor commitments for this
		// step's symbol scope. Gaps become DeferredExecutionIssues but do not halt
		// execution unless the step is also missing a git checkpoint.
		gapStatus := "skipped"
		if gapArtifact, gapIssues := performStepGapDetection(ctx, in, stepID, stepTitle, step); gapArtifact != nil {
			artifacts = append(artifacts, *gapArtifact)
			gapStatus = stringValue(gapArtifact.Payload.(map[string]any)["gap_status"])
			for _, issue := range gapIssues {
				in.State.Set("euclo.deferred_execution_issues", append(
					deferredIssuesFromState(in.State), issue,
				))
			}
		}
		// Semantic git checkpoint: emit a commit with structured YAML metadata
		// binding this step to its plan, blast radius, and gap detection outcome.
		gitRef := emitSemanticGitCheckpoint(ctx, in, stepID, stepTitle, blastRadius, gapStatus)
		if gitRef != "" {
			checkpointRefs = append(checkpointRefs, gitRef)
		}
		recordStepAttempt(ctx, in, stepID, "completed", "", gitRef)
		// Confidence recalculation: if gap detection surfaced anchor drift,
		// degrade and persist the step's confidence score so downstream steps
		// can observe the updated signal.
		if gapStatus == "warning" || gapStatus == "critical" {
			if planStep := livingPlanStepFromState(in.State, stepID); planStep != nil {
				driftedAnchors := stringSlice(step["anchor_deps"])
				newScore := frameworkplan.RecalculateConfidence(planStep, driftedAnchors, nil, frameworkplan.DefaultConfidenceDegradation())
				planStep.ConfidenceScore = newScore
				planStep.UpdatedAt = time.Now().UTC()
				planID := activePlanID(in.Work)
				if in.ServiceBundle.PlanStore != nil && planID != "" {
					_ = in.ServiceBundle.PlanStore.UpdateStep(ctx, planID, stepID, planStep)
				}
			}
		}
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
	if raw, ok := state.Get("euclo.seeded_pipeline_plan"); ok {
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

func enrichArchaeoExecutionInput(ctx context.Context, in execution.ExecuteInput) enrichedArchaeoInput {
	var enriched enrichedArchaeoInput
	if in.ServiceBundle.Archaeo == nil || strings.TrimSpace(in.Work.WorkflowID) == "" {
		return enriched
	}
	workflowID := strings.TrimSpace(in.Work.WorkflowID)
	if history, err := in.ServiceBundle.Archaeo.RequestHistory(ctx, workflowID); err == nil && history != nil {
		enriched.requestHistory = history
		for _, request := range history.Requests {
			if strings.TrimSpace(request.ID) != "" {
				enriched.requestRefs = append(enriched.requestRefs, strings.TrimSpace(request.ID))
			}
		}
	}
	if activePlan, err := in.ServiceBundle.Archaeo.ActivePlan(ctx, workflowID); err == nil && activePlan != nil {
		enriched.activePlan = activePlan
		if activePlan.ActivePlan != nil {
			enriched.patternRefs = append(enriched.patternRefs, activePlan.ActivePlan.PatternRefs...)
			enriched.anchorRefs = append(enriched.anchorRefs, activePlan.ActivePlan.AnchorRefs...)
			enriched.tensionIDs = append(enriched.tensionIDs, activePlan.ActivePlan.TensionRefs...)
			enriched.basedOnRevision = strings.TrimSpace(activePlan.ActivePlan.BasedOnRevision)
			enriched.semanticSnapshotRef = strings.TrimSpace(activePlan.ActivePlan.SemanticSnapshotRef)
			enriched.explorationID = strings.TrimSpace(activePlan.ActivePlan.DerivedFromExploration)
		}
	}
	if queue, err := in.ServiceBundle.Archaeo.LearningQueue(ctx, workflowID); err == nil && queue != nil {
		enriched.learningQueue = queue
		for _, item := range queue.PendingLearning {
			if strings.TrimSpace(item.ID) != "" {
				enriched.learningRefs = append(enriched.learningRefs, strings.TrimSpace(item.ID))
			}
		}
	}
	if tensions, err := in.ServiceBundle.Archaeo.TensionsByWorkflow(ctx, workflowID); err == nil && len(tensions) > 0 {
		enriched.tensions = append([]execution.TensionView(nil), tensions...)
		for _, tension := range tensions {
			if strings.TrimSpace(tension.ID) != "" {
				enriched.tensionIDs = append(enriched.tensionIDs, strings.TrimSpace(tension.ID))
			}
			enriched.patternRefs = append(enriched.patternRefs, tension.PatternIDs...)
			enriched.anchorRefs = append(enriched.anchorRefs, tension.AnchorRefs...)
			if enriched.basedOnRevision == "" {
				enriched.basedOnRevision = strings.TrimSpace(tension.BasedOnRevision)
			}
		}
	}
	if summary, err := in.ServiceBundle.Archaeo.TensionSummaryByWorkflow(ctx, workflowID); err == nil && summary != nil {
		enriched.tensionSummary = summary
	}
	enriched.patternRefs = execution.UniqueStrings(enriched.patternRefs)
	enriched.tensionIDs = execution.UniqueStrings(enriched.tensionIDs)
	enriched.requestRefs = execution.UniqueStrings(enriched.requestRefs)
	enriched.learningRefs = execution.UniqueStrings(enriched.learningRefs)
	enriched.anchorRefs = execution.UniqueStrings(enriched.anchorRefs)
	return enriched
}

func ensureArchaeologyExecutionState(state *core.Context, work eucloruntime.UnitOfWork, enriched enrichedArchaeoInput) {
	if state == nil {
		return
	}
	explorationID := firstNonEmptyString(
		strings.TrimSpace(work.SemanticInputs.ExplorationID),
		strings.TrimSpace(enriched.explorationID),
		strings.TrimSpace(state.GetString("euclo.active_exploration_id")),
	)
	if explorationID == "" && strings.TrimSpace(work.WorkflowID) != "" {
		explorationID = strings.TrimSpace(work.WorkflowID) + ":exploration"
	}
	if explorationID == "" {
		explorationID = "euclo:exploration"
	}
	snapshotID := firstNonEmptyString(
		strings.TrimSpace(enriched.semanticSnapshotRef),
		strings.TrimSpace(state.GetString("euclo.active_exploration_snapshot_id")),
	)
	if snapshotID == "" {
		snapshotID = explorationID + ":snapshot"
	}
	state.Set("euclo.active_exploration_id", explorationID)
	state.Set("euclo.active_exploration_snapshot_id", snapshotID)
}

func (e enrichedArchaeoInput) summary() string {
	parts := []string{}
	if len(e.patternRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d pattern refs", len(e.patternRefs)))
	}
	if len(e.tensionIDs) > 0 {
		parts = append(parts, fmt.Sprintf("%d tensions", len(e.tensionIDs)))
	}
	if len(e.learningRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d pending learning items", len(e.learningRefs)))
	}
	if e.activePlan != nil && e.activePlan.ActivePlan != nil {
		parts = append(parts, fmt.Sprintf("active plan v%d", e.activePlan.ActivePlan.Version))
	}
	if len(parts) == 0 {
		return "no archaeology service context available"
	}
	return "archaeology service context: " + strings.Join(parts, ", ")
}

func (e enrichedArchaeoInput) payload() map[string]any {
	payload := map[string]any{
		"pattern_refs":      append([]string(nil), e.patternRefs...),
		"tension_refs":      append([]string(nil), e.tensionIDs...),
		"request_refs":      append([]string(nil), e.requestRefs...),
		"learning_refs":     append([]string(nil), e.learningRefs...),
		"anchor_refs":       append([]string(nil), e.anchorRefs...),
		"based_on_revision": e.basedOnRevision,
		"snapshot_ref":      e.semanticSnapshotRef,
		"exploration_id":    e.explorationID,
	}
	if e.requestHistory != nil {
		payload["request_history"] = e.requestHistory
	}
	if e.activePlan != nil {
		payload["active_plan"] = e.activePlan
	}
	if e.learningQueue != nil {
		payload["learning_queue"] = e.learningQueue
	}
	if len(e.tensions) > 0 {
		payload["tensions"] = append([]execution.TensionView(nil), e.tensions...)
	}
	if e.tensionSummary != nil {
		payload["tension_summary"] = e.tensionSummary
	}
	payload["summary"] = e.summary()
	return payload
}

func withEnrichedSemanticInput(in execution.ExecuteInput, enriched enrichedArchaeoInput) execution.ExecuteInput {
	out := in
	out.Work.SemanticInputs.PatternRefs = execution.UniqueStrings(append(append([]string(nil), out.Work.SemanticInputs.PatternRefs...), enriched.patternRefs...))
	out.Work.SemanticInputs.TensionRefs = execution.UniqueStrings(append(append([]string(nil), out.Work.SemanticInputs.TensionRefs...), enriched.tensionIDs...))
	out.Work.SemanticInputs.RequestProvenanceRefs = execution.UniqueStrings(append(append([]string(nil), out.Work.SemanticInputs.RequestProvenanceRefs...), enriched.requestRefs...))
	out.Work.SemanticInputs.LearningInteractionRefs = execution.UniqueStrings(append(append([]string(nil), out.Work.SemanticInputs.LearningInteractionRefs...), enriched.learningRefs...))
	if strings.TrimSpace(out.Work.SemanticInputs.ExplorationID) == "" {
		out.Work.SemanticInputs.ExplorationID = enriched.explorationID
	}
	return out
}

func persistCompiledPlan(ctx context.Context, in execution.ExecuteInput, payload map[string]any, enriched enrichedArchaeoInput) (*execution.VersionedPlanView, error) {
	if in.ServiceBundle.Archaeo == nil {
		return nil, nil
	}
	plan := livingPlanFromPayload(payload, in, enriched)
	if plan == nil {
		return nil, nil
	}
	derived := strings.TrimSpace(in.Work.SemanticInputs.ExplorationID)
	if derived == "" {
		derived = enriched.explorationID
	}
	drafted, err := in.ServiceBundle.Archaeo.DraftPlanVersion(ctx, plan, execution.DraftPlanInput{
		WorkflowID:             in.Work.WorkflowID,
		DerivedFromExploration: derived,
		BasedOnRevision:        firstNonEmptyString(enriched.basedOnRevision, stringValue(payload["based_on_revision"])),
		SemanticSnapshotRef:    firstNonEmptyString(enriched.semanticSnapshotRef, stringValue(payload["snapshot_ref"])),
		PatternRefs:            execution.UniqueStrings(append(append([]string(nil), in.Work.SemanticInputs.PatternRefs...), enriched.patternRefs...)),
		TensionRefs:            execution.UniqueStrings(append(append([]string(nil), in.Work.SemanticInputs.TensionRefs...), enriched.tensionIDs...)),
		AnchorRefs:             append([]string(nil), enriched.anchorRefs...),
		FormationResultRef:     stringValue(payload["formation_result_ref"]),
	})
	if err != nil || drafted == nil {
		return nil, err
	}
	return in.ServiceBundle.Archaeo.ActivatePlanVersion(ctx, drafted.WorkflowID, drafted.Version)
}

func loadBoundPlan(ctx context.Context, in execution.ExecuteInput) (*execution.VersionedPlanView, error) {
	if in.ServiceBundle.Archaeo == nil {
		if in.State == nil {
			return nil, nil
		}
		if raw, ok := in.State.Get("euclo.active_plan_version"); ok {
			switch typed := raw.(type) {
			case *execution.VersionedPlanView:
				return typed, nil
			case execution.VersionedPlanView:
				return &typed, nil
			}
		}
		return nil, nil
	}
	if in.Work.PlanBinding != nil && strings.TrimSpace(in.Work.PlanBinding.WorkflowID) != "" {
		return in.ServiceBundle.Archaeo.ActivePlanVersion(ctx, in.Work.PlanBinding.WorkflowID)
	}
	if strings.TrimSpace(in.Work.WorkflowID) != "" {
		return in.ServiceBundle.Archaeo.ActivePlanVersion(ctx, in.Work.WorkflowID)
	}
	return nil, nil
}

func versionedPlanPayload(active *execution.VersionedPlanView) map[string]any {
	if active == nil {
		return nil
	}
	steps := make([]map[string]any, 0, len(active.Plan.StepOrder))
	for _, stepID := range active.Plan.StepOrder {
		step := active.Plan.Steps[stepID]
		if step == nil {
			continue
		}
		record := map[string]any{
			"id":               step.ID,
			"description":      step.Description,
			"scope":            append([]string(nil), step.Scope...),
			"anchor_deps":      append([]string(nil), step.AnchorDependencies...),
			"confidence_score": step.ConfidenceScore,
			"depends_on":       append([]string(nil), step.DependsOn...),
			"status":           step.Status,
		}
		steps = append(steps, record)
	}
	return map[string]any{
		"plan_id":               active.PlanID,
		"plan_version":          active.Version,
		"title":                 active.Plan.Title,
		"workflow_id":           active.WorkflowID,
		"derived_exploration":   active.DerivedFromExploration,
		"based_on_revision":     active.BasedOnRevision,
		"semantic_snapshot_ref": active.SemanticSnapshotRef,
		"pattern_refs":          append([]string(nil), active.PatternRefs...),
		"tension_refs":          append([]string(nil), active.TensionRefs...),
		"steps":                 steps,
		"summary":               firstNonEmptyString(active.Plan.Title, fmt.Sprintf("plan v%d", active.Version)),
	}
}

func livingPlanFromPayload(payload map[string]any, in execution.ExecuteInput, enriched enrichedArchaeoInput) *frameworkplan.LivingPlan {
	if !compiledPlanReady(payload) {
		return nil
	}
	now := time.Now().UTC()
	planID := firstNonEmptyString(stringValue(payload["plan_id"]), activePlanID(in.Work))
	if planID == "" {
		planID = fmt.Sprintf("plan-%s-%d", firstNonEmptyString(in.Work.WorkflowID, "euclo"), now.Unix())
	}
	title := firstNonEmptyString(stringValue(payload["title"]), stringValue(payload["summary"]), "Euclo compiled plan")
	plan := &frameworkplan.LivingPlan{
		ID:         planID,
		WorkflowID: in.Work.WorkflowID,
		Title:      title,
		Steps:      map[string]*frameworkplan.PlanStep{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	for idx, item := range compiledPlanSteps(payload) {
		stepID := firstNonEmptyString(stringValue(item["id"]), fmt.Sprintf("step-%d", idx+1))
		description := firstNonEmptyString(stringValue(item["description"]), stringValue(item["title"]), fmt.Sprintf("step %d", idx+1))
		scope := stringSlice(item["scope"])
		if len(scope) == 0 {
			scope = stringSlice(item["symbol_scope"])
		}
		step := &frameworkplan.PlanStep{
			ID:                 stepID,
			Description:        description,
			Scope:              scope,
			AnchorDependencies: execution.UniqueStrings(append(stringSlice(item["anchor_dependencies"]), enriched.anchorRefs...)),
			ConfidenceScore:    floatValue(item["confidence_score"], 0.7),
			DependsOn:          stringSlice(item["depends_on"]),
			Status:             frameworkplan.PlanStepPending,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if gate := stringValue(item["evidence_gate"]); gate != "" {
			step.EvidenceGate = &frameworkplan.EvidenceGate{RequiredSymbols: []string{gate}}
		}
		plan.Steps[stepID] = step
		plan.StepOrder = append(plan.StepOrder, stepID)
	}
	if plan.Version <= 0 {
		plan.Version = 1
	}
	return plan
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

func buildImplementStepInstruction(stepTitle string, step map[string]any, index, total int, in execution.ExecuteInput, blastRadius map[string]any) string {
	parts := []string{
		fmt.Sprintf("Execute plan step %d/%d: %s.", index+1, total, stepTitle),
	}
	if desc := strings.TrimSpace(stringValue(step["description"])); desc != "" && desc != stepTitle {
		parts = append(parts, "Description: "+desc)
	}
	if expected := strings.TrimSpace(stringValue(step["expected"])); expected != "" {
		parts = append(parts, "Expected outcome: "+expected)
	}
	if blastRadius != nil {
		if count, ok := blastRadius["affected_count"].(int); ok && count > 0 {
			parts = append(parts, fmt.Sprintf("Blast radius: %d symbols affected — proceed with care.", count))
		}
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
	if raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func stringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(fmt.Sprint(item)); value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

// deferredIssuesFromState reads the current deferred issues slice from state.
func deferredIssuesFromState(state *core.Context) []eucloruntime.DeferredExecutionIssue {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.deferred_execution_issues")
	if !ok || raw == nil {
		return nil
	}
	if typed, ok := raw.([]eucloruntime.DeferredExecutionIssue); ok {
		return append([]eucloruntime.DeferredExecutionIssue(nil), typed...)
	}
	return nil
}

// performStepGapDetection runs the gap-detect recipe for the given plan step.
// It returns a verification artifact and any deferred issues raised for intent
// drift or anchor violations. Both return values are nil when the recipe
// produces no useful signal or the registry is unavailable.
func performStepGapDetection(ctx context.Context, in execution.ExecuteInput, stepID, stepTitle string, step map[string]any) (*euclotypes.Artifact, []eucloruntime.DeferredExecutionIssue) {
	scope := stringSlice(step["scope"])
	anchorDeps := stringSlice(step["anchor_deps"])
	instruction := fmt.Sprintf(
		"Gap detection for plan step %q (%s). Symbol scope: %v. Anchor dependencies: %v. "+
			"Examine the step implementation in state and report any intent drift, behavioral gaps, "+
			"or anchor violations. State gap_status as 'clean', 'warning', or 'critical'.",
		stepID, stepTitle, scope, anchorDeps,
	)
	result, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeArchaeologyImplementGapDetect,
		"archaeology-gap-detect-"+stepID, instruction)
	if err != nil || result == nil {
		return nil, nil
	}
	gapStatus := "clean"
	if result.Data != nil {
		if s := strings.TrimSpace(stringValue(result.Data["gap_status"])); s != "" {
			gapStatus = s
		}
	}
	now := time.Now().UTC()
	payload := map[string]any{
		"step_id":      stepID,
		"step_title":   stepTitle,
		"scope":        append([]string(nil), scope...),
		"anchor_deps":  append([]string(nil), anchorDeps...),
		"gap_status":   gapStatus,
		"gap_findings": result.Data,
		"summary":      execution.ResultSummary(result),
	}
	artifact := &euclotypes.Artifact{
		ID:         "archaeology_gap_detect_" + stepID,
		Kind:       euclotypes.ArtifactKindVerification,
		Summary:    fmt.Sprintf("gap detection %s for step %s", gapStatus, stepID),
		Payload:    payload,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	}
	var issues []eucloruntime.DeferredExecutionIssue
	if gapStatus == "warning" || gapStatus == "critical" {
		issue := eucloruntime.DeferredExecutionIssue{
			IssueID:               fmt.Sprintf("gap-detect-%s-%d", stepID, now.UnixNano()),
			WorkflowID:            in.Work.WorkflowID,
			RunID:                 in.Work.RunID,
			ExecutionID:           in.Work.ExecutionID,
			ActivePlanID:          activePlanID(in.Work),
			ActivePlanVersion:     activePlanVersion(in.Work),
			StepID:                stepID,
			Kind:                  eucloruntime.DeferredIssuePatternTension,
			Severity:              deferredIssueSeverityFromGap(gapStatus),
			Status:                eucloruntime.DeferredIssueStatusOpen,
			Title:                 fmt.Sprintf("Intent drift detected at step %s", stepID),
			Summary:               execution.ResultSummary(result),
			WhyNotResolvedInline:  "gap detection surfaced intent drift — surfaced as deferred issue for archaeology review",
			RecommendedReentry:    "archaeology",
			RecommendedNextAction: "review gap findings, update anchor dependencies or revise plan step scope",
			Evidence: eucloruntime.DeferredExecutionEvidence{
				RelevantPatternRefs: append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
				RelevantTensionRefs: append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		issues = append(issues, issue)
	}
	return artifact, issues
}

func deferredIssueSeverityFromGap(gapStatus string) eucloruntime.DeferredIssueSeverity {
	if gapStatus == "critical" {
		return eucloruntime.DeferredIssueSeverityHigh
	}
	return eucloruntime.DeferredIssueSeverityMedium
}

// emitSemanticGitCheckpoint writes a git commit carrying structured YAML
// metadata that binds the commit to its plan step, blast radius, and gap
// detection outcome. Returns the short commit hash on success, empty string
// on failure or missing tooling. Failures are swallowed — git checkpoint is
// best-effort and must not block plan execution.
func emitSemanticGitCheckpoint(ctx context.Context, in execution.ExecuteInput, stepID, stepTitle string, blastRadius map[string]any, gapStatus string) string {
	if in.Environment.Registry == nil || in.Task == nil || in.Task.Context == nil {
		return ""
	}
	workspace := strings.TrimSpace(fmt.Sprint(in.Task.Context["workspace"]))
	if workspace == "" {
		return ""
	}
	blastCount := 0
	if blastRadius != nil {
		blastCount, _ = blastRadius["affected_count"].(int)
	}
	planID := strings.TrimSpace(activePlanID(in.Work))
	planVersion := activePlanVersion(in.Work)
	msg := fmt.Sprintf("euclo: step %s — %s\n\n---\nstep_id: %s\nplan_id: %s\nplan_version: %d\nblast_radius: %d\ngap_status: %s\nworkflow_id: %s\n",
		stepID, stepTitle,
		stepID, planID, planVersion,
		blastCount, gapStatus,
		strings.TrimSpace(in.Work.WorkflowID),
	)
	var commitResult *core.ToolResult
	for _, capID := range []string{"tool:cli_git", "cli_git"} {
		candidate, err := in.Environment.Registry.InvokeCapability(ctx, core.NewContext(), capID, map[string]any{
			"args":              []string{"commit", "--allow-empty", "-m", msg},
			"working_directory": workspace,
		})
		if err == nil && candidate != nil && candidate.Success {
			commitResult = candidate
			break
		}
	}
	if commitResult == nil {
		if tool, ok := in.Environment.Registry.Get("cli_git"); ok && tool != nil {
			candidate, err := tool.Execute(ctx, core.NewContext(), map[string]any{
				"args":              []string{"commit", "--allow-empty", "-m", msg},
				"working_directory": workspace,
			})
			if err == nil && candidate != nil && candidate.Success {
				commitResult = candidate
			}
		}
	}
	if commitResult == nil {
		return ""
	}
	// Retrieve the new HEAD hash to use as the checkpoint reference.
	for _, capID := range []string{"tool:cli_git", "cli_git"} {
		candidate, err := in.Environment.Registry.InvokeCapability(ctx, core.NewContext(), capID, map[string]any{
			"args":              []string{"rev-parse", "--short", "HEAD"},
			"working_directory": workspace,
		})
		if err == nil && candidate != nil && candidate.Success {
			if hash := strings.TrimSpace(fmt.Sprint(candidate.Data["stdout"])); hash != "" {
				return "git:" + hash
			}
		}
	}
	return "git:committed"
}

// queryStepBlastRadius runs ImpactSet from the step's symbol scope against the
// GraphDB. Returns nil when GraphDB is unavailable or scope is empty.
func queryStepBlastRadius(in execution.ExecuteInput, scope []string) map[string]any {
	if in.ServiceBundle.GraphDB == nil || len(scope) == 0 {
		return nil
	}
	// Pass nil edgeKinds to traverse all edge kinds (GraphDB semantics: empty
	// allowed set matches everything).
	result := in.ServiceBundle.GraphDB.ImpactSet(scope, nil, 3)
	return map[string]any{
		"origin_ids":     append([]string(nil), result.OriginIDs...),
		"affected_ids":   append([]string(nil), result.Affected...),
		"affected_count": len(result.Affected),
	}
}

// persistExplorationPatterns writes newly discovered patterns from the
// exploration artifact set to PatternStore with status "proposed". Errors are
// swallowed — pattern persistence is best-effort and must not fail the
// exploration.
func persistExplorationPatterns(ctx context.Context, in execution.ExecuteInput, artifacts []euclotypes.Artifact) {
	if in.ServiceBundle.PatternStore == nil {
		return
	}
	corpusScope := strings.TrimSpace(in.Work.WorkflowID)
	for _, artifact := range artifacts {
		if artifact.Kind != euclotypes.ArtifactKindExplore {
			continue
		}
		payload, ok := artifact.Payload.(map[string]any)
		if !ok {
			continue
		}
		for _, rawPattern := range anySlice(payload["patterns"]) {
			p, ok := rawPattern.(map[string]any)
			if !ok {
				continue
			}
			title := firstNonEmptyString(
				strings.TrimSpace(stringValue(p["title"])),
				strings.TrimSpace(stringValue(p["name"])),
			)
			description := firstNonEmptyString(
				strings.TrimSpace(stringValue(p["description"])),
				strings.TrimSpace(stringValue(p["summary"])),
			)
			if title == "" || description == "" {
				continue
			}
			kind := patterns.PatternKindStructural
			if k := strings.TrimSpace(stringValue(p["kind"])); k != "" {
				kind = patterns.PatternKind(k)
			}
			now := time.Now().UTC()
			record := patterns.PatternRecord{
				ID:           fmt.Sprintf("pattern-%s-%d", strings.ReplaceAll(title, " ", "-"), now.UnixNano()),
				Kind:         kind,
				Title:        title,
				Description:  description,
				Status:       patterns.PatternStatusProposed,
				Confidence:   floatValue(p["confidence"], 0.5),
				Instances:    patternInstances(p),
				CorpusScope:  corpusScope,
				CorpusSource: in.Work.PrimaryRelurpicCapabilityID,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			if err := in.ServiceBundle.PatternStore.Save(ctx, record); err == nil && in.ServiceBundle.LearningBroker != nil {
				// Notify the LearningBroker so the TUI/operator layer can surface
				// this pattern proposal for review. Non-blocking: errors swallowed.
				interaction := archaeolearning.Interaction{
					ID:          fmt.Sprintf("broker-pattern-%s-%d", record.ID, now.UnixNano()),
					WorkflowID:  corpusScope,
					Kind:        archaeolearning.InteractionPatternProposal,
					SubjectType: archaeolearning.SubjectPattern,
					SubjectID:   record.ID,
					Title:       "New pattern proposal: " + title,
					Description: description,
					Status:      archaeolearning.StatusPending,
					Blocking:    false,
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				_ = in.ServiceBundle.LearningBroker.SubmitAsync(interaction)
			}
		}
	}
}

// persistPlanReviewComment writes the compile-plan review findings to
// CommentStore as an open-question comment tied to the plan. Errors are
// swallowed — comment persistence must not fail plan activation.
func persistPlanReviewComment(ctx context.Context, in execution.ExecuteInput, planID string, reviewResult *core.Result) {
	if in.ServiceBundle.CommentStore == nil || strings.TrimSpace(planID) == "" {
		return
	}
	body := strings.TrimSpace(execution.ResultSummary(reviewResult))
	if body == "" {
		return
	}
	now := time.Now().UTC()
	record := patterns.CommentRecord{
		CommentID:   fmt.Sprintf("plan-review-%s-%d", planID, now.UnixNano()),
		PatternID:   planID,
		IntentType:  patterns.CommentOpenQuestion,
		Body:        body,
		AuthorKind:  patterns.AuthorKindAgent,
		TrustClass:  patterns.TrustClassBuiltinTrusted,
		CorpusScope: strings.TrimSpace(in.Work.WorkflowID),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_ = in.ServiceBundle.CommentStore.Save(ctx, record)
}

// livingPlanStepFromState returns the PlanStep for stepID from the living plan
// stored in state under "euclo.living_plan". Returns nil when the state or
// plan is unavailable, or the step is not present.
func livingPlanStepFromState(state *core.Context, stepID string) *frameworkplan.PlanStep {
	if state == nil || strings.TrimSpace(stepID) == "" {
		return nil
	}
	raw, ok := state.Get("euclo.living_plan")
	if !ok || raw == nil {
		return nil
	}
	var plan *frameworkplan.LivingPlan
	switch typed := raw.(type) {
	case *frameworkplan.LivingPlan:
		plan = typed
	case frameworkplan.LivingPlan:
		plan = &typed
	default:
		return nil
	}
	if plan == nil || plan.Steps == nil {
		return nil
	}
	return plan.Steps[strings.TrimSpace(stepID)]
}

// recordStepAttempt appends a StepAttempt to the step's History and persists
// the updated PlanStep via PlanStore. It also advances the step status to
// completed or failed. Errors are swallowed — attempt recording is
// best-effort and must not block plan execution.
func recordStepAttempt(ctx context.Context, in execution.ExecuteInput, stepID, outcome, failureReason, gitCheckpoint string) {
	planID := activePlanID(in.Work)
	if in.ServiceBundle.PlanStore == nil || planID == "" || strings.TrimSpace(stepID) == "" {
		return
	}
	planStep := livingPlanStepFromState(in.State, stepID)
	if planStep == nil {
		planStep = &frameworkplan.PlanStep{
			ID:        stepID,
			CreatedAt: time.Now().UTC(),
		}
	}
	attempt := frameworkplan.StepAttempt{
		AttemptedAt:   time.Now().UTC(),
		Outcome:       outcome,
		FailureReason: failureReason,
		GitCheckpoint: gitCheckpoint,
	}
	planStep.History = append(planStep.History, attempt)
	planStep.UpdatedAt = time.Now().UTC()
	switch outcome {
	case "completed":
		planStep.Status = frameworkplan.PlanStepCompleted
	case "failed":
		planStep.Status = frameworkplan.PlanStepFailed
	}
	_ = in.ServiceBundle.PlanStore.UpdateStep(ctx, planID, stepID, planStep)
}

// blockingLearningIDsFromRoutineArtifacts extracts blocking learning
// interaction IDs from the convergence-guard routine artifact payload. Returns
// nil when no blocking items are present.
func blockingLearningIDsFromRoutineArtifacts(artifacts []euclotypes.Artifact) []string {
	for _, artifact := range artifacts {
		if artifact.ProducerID != ConvergenceGuard {
			continue
		}
		payload, ok := artifact.Payload.(map[string]any)
		if !ok {
			continue
		}
		queue, ok := payload["learning_queue"].(map[string]any)
		if !ok {
			continue
		}
		blocking := stringSlice(queue["blocking"])
		if len(blocking) > 0 {
			return blocking
		}
	}
	return nil
}

// submitPlanReviewGuidance posts a non-blocking GuidanceAmbiguity request to
// the GuidanceBroker when the compile-plan review surfaced open questions.
// Errors are swallowed — guidance notification must not block plan activation.
func submitPlanReviewGuidance(in execution.ExecuteInput, planID string, reviewResult *core.Result) {
	if in.ServiceBundle.GuidanceBroker == nil {
		return
	}
	if reviewResult == nil || !reviewResult.Success {
		return
	}
	openQuestions := stringSlice(reviewResult.Data["open_questions"])
	if len(openQuestions) == 0 {
		return
	}
	summary := strings.TrimSpace(execution.ResultSummary(reviewResult))
	if summary == "" {
		summary = fmt.Sprintf("Plan %s compilation review found %d open question(s)", planID, len(openQuestions))
	}
	req := guidance.GuidanceRequest{
		ID:          fmt.Sprintf("plan-review-%s-%d", planID, time.Now().UnixNano()),
		Kind:        guidance.GuidanceAmbiguity,
		Title:       "Plan compilation: open questions require acknowledgement",
		Description: summary,
		Choices: []guidance.GuidanceChoice{
			{ID: "proceed", Label: "Proceed with plan as-is", IsDefault: true},
			{ID: "refine", Label: "Refine plan before execution"},
			{ID: "defer", Label: "Defer execution"},
		},
		TimeoutBehavior: guidance.GuidanceTimeoutDefer,
		Context: map[string]any{
			"plan_id":        planID,
			"workflow_id":    in.Work.WorkflowID,
			"open_questions": openQuestions,
		},
	}
	_, _ = in.ServiceBundle.GuidanceBroker.SubmitAsync(req)
}

func patternInstances(payload map[string]any) []patterns.PatternInstance {
	files := stringSlice(payload["files"])
	if len(files) == 0 {
		files = stringSlice(payload["instances"])
	}
	out := make([]patterns.PatternInstance, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		out = append(out, patterns.PatternInstance{FilePath: file})
	}
	return out
}

func floatValue(raw any, fallback float64) float64 {
	switch typed := raw.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		if value, err := typed.Float64(); err == nil {
			return value
		}
	case string:
		if typed = strings.TrimSpace(typed); typed != "" {
			if value, err := json.Number(typed).Float64(); err == nil {
				return value
			}
		}
	}
	return fallback
}

func archaeologyReviewPayload(source, summary string, reviewData any) map[string]any {
	return map[string]any{
		"review_source": source,
		"summary":       summary,
		"review":        reviewData,
		"findings": []map[string]any{{
			"severity":         "info",
			"description":      firstNonEmptyString(summary, "archaeology review completed"),
			"rationale":        "archaeology review summarized plan completeness and execution readiness",
			"category":         "planning",
			"confidence":       0.5,
			"impacted_files":   []string{},
			"impacted_symbols": []string{},
			"review_source":    source,
			"traceability": map[string]any{
				"source": "reflection_review",
			},
		}},
	}
}
