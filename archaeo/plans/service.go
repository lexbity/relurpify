package plans

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkkeylock "codeburg.org/lexbit/relurpify/framework/keylock"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

type ActiveContext struct {
	WorkflowID string
	Plan       *frameworkplan.LivingPlan
	Step       *frameworkplan.PlanStep
}

type Service struct {
	Store         frameworkplan.PlanStore
	WorkflowStore memory.WorkflowStateStore
	Now           func() time.Time
}

var planMutationLocks frameworkkeylock.Locker

func (s Service) ApplyInvalidation(plan *frameworkplan.LivingPlan, event frameworkplan.InvalidationEvent, excludeStepID string) []string {
	if plan == nil {
		return nil
	}
	invalidated := frameworkplan.PropagateInvalidation(plan, event)
	if excludeStepID == "" {
		return invalidated
	}
	out := make([]string, 0, len(invalidated))
	for _, stepID := range invalidated {
		if stepID == excludeStepID {
			continue
		}
		out = append(out, stepID)
	}
	return out
}

func (s Service) ApplySymbolInvalidations(plan *frameworkplan.LivingPlan, stepID string, symbolIDs []string) []string {
	if plan == nil || len(symbolIDs) == 0 {
		return nil
	}
	now := s.now()
	changed := make([]string, 0)
	for _, symbolID := range symbolIDs {
		changed = append(changed, s.ApplyInvalidation(plan, frameworkplan.InvalidationEvent{
			Kind:   frameworkplan.InvalidationSymbolChanged,
			Target: symbolID,
			At:     now,
		}, stepID)...)
	}
	return uniqueStrings(changed)
}

func (s Service) ApplyAnchorInvalidations(plan *frameworkplan.LivingPlan, stepID string, anchorIDs []string) []string {
	if plan == nil || len(anchorIDs) == 0 {
		return nil
	}
	now := s.now()
	changed := make([]string, 0)
	for _, anchorID := range anchorIDs {
		changed = append(changed, s.ApplyInvalidation(plan, frameworkplan.InvalidationEvent{
			Kind:   frameworkplan.InvalidationAnchorDrifted,
			Target: anchorID,
			At:     now,
		}, stepID)...)
	}
	return uniqueStrings(changed)
}

func (s Service) ApplyScopeInvalidations(plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) []string {
	if plan == nil || step == nil || len(step.Scope) == 0 {
		return nil
	}
	changed := s.ApplySymbolInvalidations(plan, step.ID, step.Scope)
	if len(changed) == 0 {
		return nil
	}
	plan.UpdatedAt = s.now()
	return changed
}

func (s Service) LoadActiveContext(ctx context.Context, workflowID string, task *core.Task) (*ActiveContext, error) {
	if s.Store == nil || workflowID == "" {
		return nil, nil
	}
	plan, err := s.loadActivePlan(ctx, workflowID)
	if err != nil || plan == nil {
		return nil, err
	}
	return &ActiveContext{
		WorkflowID: workflowID,
		Plan:       plan,
		Step:       ActiveStep(task, plan),
	}, nil
}

func (s Service) loadActivePlan(ctx context.Context, workflowID string) (*frameworkplan.LivingPlan, error) {
	if s.Store == nil {
		return nil, nil
	}
	if active, err := s.LoadActiveVersion(ctx, workflowID); err != nil {
		return nil, err
	} else if active != nil {
		plan, err := s.Store.LoadPlan(ctx, active.Plan.ID)
		if err != nil || plan == nil {
			return &active.Plan, err
		}
		plan.Version = active.Version
		return plan, nil
	}
	return s.Store.LoadPlanByWorkflow(ctx, workflowID)
}

func (s Service) workflowStore() memory.WorkflowStateStore {
	if s.WorkflowStore == nil {
		return nil
	}
	value := reflect.ValueOf(s.WorkflowStore)
	switch value.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface:
		if value.IsNil() {
			return nil
		}
	}
	return s.WorkflowStore
}

func (s Service) PersistStep(ctx context.Context, plan *frameworkplan.LivingPlan, stepID string) error {
	if s.Store == nil || plan == nil || stepID == "" {
		return nil
	}
	step := plan.Steps[stepID]
	if step == nil {
		return nil
	}
	return s.Store.UpdateStep(ctx, plan.ID, stepID, step)
}

func (s Service) PersistAllSteps(ctx context.Context, plan *frameworkplan.LivingPlan) error {
	if s.Store == nil || plan == nil {
		return nil
	}
	for stepID := range plan.Steps {
		if err := s.PersistStep(ctx, plan, stepID); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) PersistPreflightBlocked(ctx context.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, reason string, shouldInvalidate bool, invalidatedStepIDs []string) (*core.Result, error) {
	if plan == nil || step == nil {
		return nil, nil
	}
	s.RecordBlockedStep(plan, step, reason, shouldInvalidate)
	if err := s.PersistStep(ctx, plan, step.ID); err != nil {
		return nil, err
	}
	if len(invalidatedStepIDs) > 0 {
		if err := s.PersistAllSteps(ctx, plan); err != nil {
			return nil, err
		}
	}
	return &core.Result{
		Success: false,
		Error:   fmt.Errorf("%s", reason),
		Data:    map[string]any{"plan_step_status": "blocked"},
	}, fmt.Errorf("%s", reason)
}

func (s Service) PersistPreflightShortCircuit(ctx context.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, result *core.Result, err error) (*core.Result, error) {
	if plan == nil || step == nil {
		return result, err
	}
	if persistErr := s.PersistStep(ctx, plan, step.ID); persistErr != nil {
		if err == nil {
			err = persistErr
		}
	}
	return result, err
}

func (s Service) PersistPreflightConfidenceUpdate(ctx context.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) error {
	if plan == nil || step == nil {
		return nil
	}
	step.UpdatedAt = s.now()
	plan.UpdatedAt = s.now()
	return s.PersistStep(ctx, plan, step.ID)
}

func (s Service) RecordStepOutcome(plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, outcome, failureReason, gitCheckpoint string) {
	if plan == nil || step == nil {
		return
	}
	now := s.now()
	attempt := frameworkplan.StepAttempt{
		AttemptedAt:   now,
		Outcome:       outcome,
		FailureReason: failureReason,
		GitCheckpoint: gitCheckpoint,
	}
	step.History = append(step.History, attempt)
	switch outcome {
	case "completed":
		step.Status = frameworkplan.PlanStepCompleted
	case "failed":
		step.Status = frameworkplan.PlanStepFailed
	case "skipped":
		step.Status = frameworkplan.PlanStepSkipped
	case "blocked":
		step.Status = frameworkplan.PlanStepInvalidated
	}
	step.UpdatedAt = now
	plan.UpdatedAt = now
}

func (s Service) RecordBlockedStep(plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, reason string, invalidate bool) {
	if plan == nil || step == nil {
		return
	}
	now := s.now()
	step.History = append(step.History, frameworkplan.StepAttempt{
		AttemptedAt:   now,
		Outcome:       "blocked",
		FailureReason: reason,
	})
	if invalidate {
		step.Status = frameworkplan.PlanStepInvalidated
	}
	step.UpdatedAt = now
	plan.UpdatedAt = now
}

func ActiveStep(task *core.Task, plan *frameworkplan.LivingPlan) *frameworkplan.PlanStep {
	if task == nil || task.Context == nil || plan == nil {
		return nil
	}
	raw, ok := task.Context["current_step_id"]
	if !ok || raw == nil {
		return nil
	}
	stepID, _ := raw.(string)
	if stepID == "" {
		stepID = fmt.Sprint(raw)
	}
	if stepID == "" {
		return nil
	}
	return plan.Steps[stepID]
}

func (s Service) AnchorChunks(ctx context.Context, workflowID string, version int, rootChunkIDs []string, chunkStateRef string) (*frameworkplan.LivingPlan, error) {
	record, err := s.AnchorChunkVersion(ctx, workflowID, version, rootChunkIDs, chunkStateRef)
	if err != nil || record == nil {
		return nil, err
	}
	return &record.Plan, nil
}

func (s Service) AnchorChunkVersion(ctx context.Context, workflowID string, version int, rootChunkIDs []string, chunkStateRef string) (*archaeodomain.VersionedLivingPlan, error) {
	var (
		record *archaeodomain.VersionedLivingPlan
		err    error
	)
	err = planMutationLocks.With("workflow:"+strings.TrimSpace(workflowID), func() error {
		record, err = s.LoadVersion(ctx, workflowID, version)
		if err != nil || record == nil {
			if err != nil {
				return err
			}
			return fmt.Errorf("plan version %d not found", version)
		}
		record.RootChunkIDs = uniqueStrings(rootChunkIDs)
		record.ChunkStateRef = strings.TrimSpace(chunkStateRef)
		record.UpdatedAt = s.now()
		return s.saveVersion(ctx, record)
	})
	return record, err
}

func (s Service) ChunkSeedForVersion(ctx context.Context, workflowID string, version int) ([]string, error) {
	record, err := s.LoadVersion(ctx, workflowID, version)
	if err != nil || record == nil {
		return nil, err
	}
	return append([]string(nil), record.RootChunkIDs...), nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
