package execution

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeoretrieval "codeburg.org/lexbit/relurpify/archaeo/retrieval"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

type Service struct {
	GuidanceBroker *guidance.GuidanceBroker
	DeferralPolicy guidance.DeferralPolicy
	WorkflowStore  memory.WorkflowStateStore
	Retrieval      archaeoretrieval.Store
	MutationPolicy MutationPolicy
	Now            func() time.Time
}

type MutationPolicy struct {
	ContinueOnStalePlan bool
}

type MutationEvaluation struct {
	WorkflowID        string
	HandoffRef        string
	ActiveStepID      string
	RelevantMutations []archaeodomain.MutationEvent
	HighestImpact     archaeodomain.MutationImpact
	Disposition       archaeodomain.ExecutionDisposition
	Blocking          bool
	RequireReplan     bool
	ContinueOnStale   bool
}

type BlastRadiusAssessment struct {
	Expected int
	Actual   int
	Affected []string
}

type GateAssessment struct {
	MissingSymbols      []string
	ActiveAnchors       map[string]bool
	DriftedAnchors      map[string]struct{}
	DriftedDependencies []string
	Confidence          float64
	ConfidenceThreshold float64
	BlastRadius         *BlastRadiusAssessment
	Evidence            retrieval.MixedEvidenceResult
	HasEvidence         bool
	AvailableSymbolMap  map[string]bool
}

type PreflightOutcome struct {
	ConfidenceUpdated  bool
	InvalidatedStepIDs []string
	ShouldInvalidate   bool
	Result             *core.Result
	Err                error
	MutationEvaluation *MutationEvaluation
	MutationCheckpoint *archaeodomain.MutationCheckpointSummary
}

func (s Service) GuidanceTimeoutBehavior(kind guidance.GuidanceKind, blastRadius int) guidance.GuidanceTimeoutBehavior {
	policy := s.DeferralPolicy
	if policy.MaxBlastRadiusForDefer == 0 && len(policy.DeferrableKinds) == 0 {
		policy = guidance.DefaultDeferralPolicy()
	}
	if kind == guidance.GuidanceRecovery {
		return guidance.GuidanceTimeoutFail
	}
	if blastRadius > policy.MaxBlastRadiusForDefer {
		return guidance.GuidanceTimeoutFail
	}
	for _, allowed := range policy.DeferrableKinds {
		if allowed == kind {
			return guidance.GuidanceTimeoutDefer
		}
	}
	return guidance.GuidanceTimeoutFail
}

func (s Service) RequestGuidance(ctx context.Context, req guidance.GuidanceRequest, fallbackChoice string) guidance.GuidanceDecision {
	if s.GuidanceBroker == nil {
		log.Printf("euclo: guidance broker unavailable for %s; proceeding with %s", req.Kind, fallbackChoice)
		return guidance.GuidanceDecision{
			RequestID: req.ID,
			ChoiceID:  fallbackChoice,
			DecidedBy: "no-broker",
			DecidedAt: s.now(),
		}
	}
	decision, err := s.GuidanceBroker.Request(ctx, req)
	if err != nil || decision == nil {
		log.Printf("euclo: guidance request failed for %s: %v", req.Kind, err)
		return guidance.GuidanceDecision{
			RequestID: req.ID,
			ChoiceID:  fallbackChoice,
			DecidedBy: "guidance-error",
			DecidedAt: s.now(),
		}
	}
	return *decision
}

func (s Service) ApplyGuidanceDecision(plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, decision guidance.GuidanceDecision, reason string) (*core.Result, error, bool) {
	if plan == nil || step == nil {
		return nil, nil, false
	}
	now := s.now()
	switch decision.ChoiceID {
	case "skip":
		step.Status = frameworkplan.PlanStepSkipped
		step.History = append(step.History, frameworkplan.StepAttempt{
			AttemptedAt:   now,
			Outcome:       "skipped",
			FailureReason: reason,
		})
		step.UpdatedAt = now
		return &core.Result{
			Success: true,
			Data: map[string]any{
				"plan_step_status": "skipped",
				"guidance_decision": map[string]any{
					"choice_id":  decision.ChoiceID,
					"decided_by": decision.DecidedBy,
				},
			},
		}, nil, true
	case "replan":
		replanErr := fmt.Errorf("replan requested for step %s", step.ID)
		step.Status = frameworkplan.PlanStepFailed
		step.History = append(step.History, frameworkplan.StepAttempt{
			AttemptedAt:   now,
			Outcome:       "failed",
			FailureReason: replanErr.Error(),
		})
		step.UpdatedAt = now
		return &core.Result{
			Success: false,
			Error:   replanErr,
			Data: map[string]any{
				"plan_step_status": "failed",
				"guidance_decision": map[string]any{
					"choice_id":  decision.ChoiceID,
					"decided_by": decision.DecidedBy,
				},
			},
		}, replanErr, true
	default:
		return nil, nil, false
	}
}

func (s Service) AnchorGateState(ctx context.Context, task *core.Task) (map[string]bool, map[string]struct{}, error) {
	active := make(map[string]bool)
	drifted := make(map[string]struct{})
	if s.Retrieval == nil {
		return active, drifted, nil
	}
	corpusScope := CorpusScopeForTask(task)
	driftedRecords, err := s.Retrieval.DriftedAnchors(ctx, corpusScope)
	if err != nil {
		return nil, nil, err
	}
	for _, record := range driftedRecords {
		drifted[record.AnchorID] = struct{}{}
	}
	activeRecords, err := s.Retrieval.ActiveAnchors(ctx, corpusScope)
	if err != nil {
		return nil, nil, err
	}
	for _, record := range activeRecords {
		if record.SupersededBy != nil {
			continue
		}
		if _, blocked := drifted[record.AnchorID]; blocked {
			continue
		}
		active[record.AnchorID] = true
	}
	return active, drifted, nil
}

func (s Service) AssessPlanStep(ctx context.Context, task *core.Task, state *core.Context, step *frameworkplan.PlanStep, graph *graphdb.Engine) (GateAssessment, error) {
	var assessment GateAssessment
	if step == nil {
		return assessment, nil
	}
	assessment.MissingSymbols = MissingPlanSymbols(step, graph)
	activeAnchors, driftedAnchors, err := s.AnchorGateState(ctx, task)
	if err != nil {
		return assessment, err
	}
	assessment.ActiveAnchors = activeAnchors
	assessment.DriftedAnchors = driftedAnchors
	assessment.DriftedDependencies = intersectStrings(step.AnchorDependencies, driftedAnchors)
	degradation := frameworkplan.DefaultConfidenceDegradation()
	assessment.Confidence = frameworkplan.RecalculateConfidence(step, assessment.DriftedDependencies, assessment.MissingSymbols, degradation)
	assessment.ConfidenceThreshold = degradation.Threshold
	assessment.Evidence, assessment.HasEvidence = MixedEvidenceForStep(state, step)
	assessment.AvailableSymbolMap = AvailableSymbolMap(step, graph)
	if ShouldCheckBlastRadius(step, graph != nil) {
		impact := graph.ImpactSet(step.Scope, nil, 3)
		expected := len(step.Scope)
		actual := len(impact.Affected)
		if expected > 0 && actual > BlastRadiusExpansionThreshold(expected) {
			assessment.BlastRadius = &BlastRadiusAssessment{
				Expected: expected,
				Actual:   actual,
				Affected: TruncateStrings(impact.Affected, 20),
			}
		}
	}
	return assessment, nil
}

func (s Service) EvaluateMutations(ctx context.Context, workflowID string, handoff *archaeodomain.ExecutionHandoff, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) (*MutationEvaluation, error) {
	if s.WorkflowStore == nil || workflowID == "" {
		return nil, nil
	}
	mutations, err := archaeoevents.ReadMutationEvents(ctx, s.WorkflowStore, workflowID)
	if err != nil || len(mutations) == 0 {
		return nil, err
	}
	result := &MutationEvaluation{
		WorkflowID:   workflowID,
		HandoffRef:   handoffRef(handoff),
		ActiveStepID: stepID(step),
		Disposition:  archaeodomain.DispositionContinue,
	}
	for _, mutation := range mutations {
		if !mutationRelevantToExecution(mutation, handoff, plan, step) {
			continue
		}
		adjustMutationForPolicy(&mutation, s.MutationPolicy)
		result.RelevantMutations = append(result.RelevantMutations, mutation)
		if mutationPriority(mutation.Disposition) > mutationPriority(result.Disposition) {
			result.Disposition = mutation.Disposition
			result.HighestImpact = mutation.Impact
		}
	}
	if len(result.RelevantMutations) == 0 {
		return nil, nil
	}
	switch result.Disposition {
	case archaeodomain.DispositionInvalidateStep, archaeodomain.DispositionPauseForLearning, archaeodomain.DispositionPauseForGuidance, archaeodomain.DispositionBlockExecution:
		result.Blocking = true
	case archaeodomain.DispositionRequireReplan:
		result.Blocking = true
		result.RequireReplan = true
	case archaeodomain.DispositionContinueOnStalePlan:
		result.ContinueOnStale = true
	}
	if result.HighestImpact == "" {
		result.HighestImpact = archaeodomain.ImpactInformational
	}
	return result, nil
}

func mutationRelevantToExecution(mutation archaeodomain.MutationEvent, handoff *archaeodomain.ExecutionHandoff, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) bool {
	if handoff != nil && !handoff.CreatedAt.IsZero() && mutation.CreatedAt.Before(handoff.CreatedAt) {
		return false
	}
	if step != nil && containsString(mutation.BlastRadius.AffectedStepIDs, step.ID) {
		return true
	}
	if step != nil && strings.TrimSpace(mutation.StepID) == strings.TrimSpace(step.ID) {
		return true
	}
	if mutation.Category == archaeodomain.MutationPlanStaleness {
		if plan != nil && strings.TrimSpace(mutation.PlanID) != "" && strings.TrimSpace(mutation.PlanID) == strings.TrimSpace(plan.ID) {
			return true
		}
		if handoff != nil && mutation.PlanVersion != nil && *mutation.PlanVersion == handoff.PlanVersion {
			return true
		}
		return plan != nil && mutation.PlanVersion != nil && plan.Version == *mutation.PlanVersion
	}
	if mutation.Blocking {
		return true
	}
	if plan != nil && strings.TrimSpace(mutation.PlanID) != "" && strings.TrimSpace(mutation.PlanID) == strings.TrimSpace(plan.ID) {
		return true
	}
	return false
}

func adjustMutationForPolicy(mutation *archaeodomain.MutationEvent, policy MutationPolicy) {
	if mutation == nil {
		return
	}
	if mutation.Category != archaeodomain.MutationPlanStaleness {
		return
	}
	if policy.ContinueOnStalePlan {
		if mutation.Disposition == archaeodomain.DispositionRequireReplan {
			mutation.Disposition = archaeodomain.DispositionContinueOnStalePlan
		}
		return
	}
	if mutation.Disposition == archaeodomain.DispositionContinueOnStalePlan {
		mutation.Disposition = archaeodomain.DispositionRequireReplan
	}
}

func mutationPriority(disposition archaeodomain.ExecutionDisposition) int {
	switch disposition {
	case archaeodomain.DispositionBlockExecution:
		return 6
	case archaeodomain.DispositionRequireReplan:
		return 5
	case archaeodomain.DispositionInvalidateStep:
		return 4
	case archaeodomain.DispositionPauseForLearning, archaeodomain.DispositionPauseForGuidance:
		return 3
	case archaeodomain.DispositionContinueOnStalePlan:
		return 2
	case archaeodomain.DispositionContinue:
		return 1
	default:
		return 0
	}
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func handoffRef(handoff *archaeodomain.ExecutionHandoff) string {
	if handoff == nil {
		return ""
	}
	return strings.TrimSpace(handoff.HandoffRef)
}

func ShouldCheckBlastRadius(step *frameworkplan.PlanStep, graphAvailable bool) bool {
	return graphAvailable && step != nil && len(step.Scope) > 0
}

func BlastRadiusExpansionThreshold(expected int) int {
	if expected <= 0 {
		return 0
	}
	return maxInt(expected*2, expected+5)
}

func TruncateStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[:limit]...)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func intersectStrings(values []string, allowed map[string]struct{}) []string {
	if len(values) == 0 || len(allowed) == 0 {
		return nil
	}
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
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
